package knowledgebase

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/graph"
	"go.uber.org/zap"
)

type (
	// DynamicValueContext is used to scope the DAG into the template functions
	DynamicValueContext struct {
		DAG *construct.ResourceGraph

		resultJson bool
	}

	// DynamicValueData provides the resource or edge to the templates as
	// `{{ .Self }}` for resources
	// `{{ .Source }}` and `{{ .Destination }}` for edges
	DynamicValueData struct {
		Resource construct.ResourceId
		Edge     graph.Edge[construct.ResourceId]
	}
)

func (ctx *DynamicValueContext) Parse(tmpl string) (*template.Template, error) {
	t, err := template.New("config").Funcs(template.FuncMap{
		"upstream":      ctx.Upstream,
		"allUpstream":   ctx.AllUpstream,
		"downstream":    ctx.Downstream,
		"allDownstream": ctx.AllDownstream,
		"shortestPath":  ctx.ShortestPath,
		"fieldValue":    ctx.FieldValue,
		"fieldRef":      ctx.FieldRef,

		"toJson": ctx.toJson,

		"split":    strings.Split,
		"join":     strings.Join,
		"basename": filepath.Base,

		"filterIds":            filterIds,
		"filterMatch":          filterMatch,
		"mapString":            mapString,
		"zipToMap":             zipToMap,
		"keysToMapWithDefault": keysToMapWithDefault,
		"replace":              replaceRegex,

		"add": add,
		"sub": sub,
	}).Parse(tmpl)
	return t, err
}

// ExecuteDecode executes the template `tmpl` using `data` and decodes the value into `value`
func (ctx DynamicValueContext) ExecuteDecode(tmpl string, data DynamicValueData, value interface{}) error {
	t, err := ctx.Parse(tmpl)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	if err := t.Execute(buf, data); err != nil {
		return err
	}

	if ctx.resultJson {
		dec := json.NewDecoder(buf)
		return dec.Decode(value)
	}

	// trim the spaces so you don't have to sprinkle the templates with `{{-` and `-}}` (the `-` trims spaces)
	bstr := strings.TrimSpace(buf.String())

	switch value := value.(type) {
	case *string:
		*value = bstr
		return nil

	case *[]byte:
		*value = []byte(bstr)
		return nil

	case *bool:
		b, err := strconv.ParseBool(bstr)
		if err != nil {
			return err
		}
		*value = b
		return nil

	case encoding.TextUnmarshaler:
		// notably, this handles `construct.ResourceId` and `construct.IaCValue`
		return value.UnmarshalText([]byte(bstr))
	}

	resultStr := reflect.ValueOf(buf.String())
	valueRefl := reflect.ValueOf(value).Elem()
	if resultStr.Type().AssignableTo(valueRefl.Type()) {
		// this covers alias types like `type MyString string`
		valueRefl.Set(resultStr)
		return nil
	}

	return fmt.Errorf("cannot decode template result '%s' into %T", buf, value)
}

func (ctx *DynamicValueContext) ResolveConfig(config Configuration, data DynamicValueData) (Configuration, error) {
	res := ctx.DAG.GetResource(data.Resource)

	field := reflect.ValueOf(res).Elem().FieldByName(config.Field)
	if !field.IsValid() {
		return config, fmt.Errorf("field %s not found on resource %s", config.Field, data.Resource)
	}

	valueRefl := reflect.New(field.Type())
	value := valueRefl.Interface()
	err := ctx.ExecuteDecode(config.ValueTemplate, data, value)
	if err != nil {
		return config, err
	}

	config.Value = valueRefl.Elem().Interface()
	config.ValueTemplate = ""
	return config, nil
}

func (data DynamicValueData) Self() (construct.ResourceId, error) {
	if data.Resource.IsZero() {
		return construct.ResourceId{}, fmt.Errorf("no .Self is set")
	}
	return data.Resource, nil
}

func (data DynamicValueData) Source() (construct.ResourceId, error) {
	if data.Edge.Source.IsZero() {
		return construct.ResourceId{}, fmt.Errorf("no .Source is set")
	}
	return data.Edge.Source, nil
}

func (data DynamicValueData) Destination() (construct.ResourceId, error) {
	if data.Edge.Destination.IsZero() {
		return construct.ResourceId{}, fmt.Errorf("no .Destination is set")
	}
	return data.Edge.Destination, nil
}

// Log is primarily used for debugging templates and shouldn't actually appear in any.
// Allows for outputting any intermediate values (such as `$integration := downstream "aws:api_integration" .Self`)
func (data DynamicValueData) Log(level string, message string, args ...interface{}) string {
	l := zap.L()
	if !data.Resource.IsZero() {
		l = l.With(zap.String("resource", data.Resource.String()))
	}
	if !data.Edge.Source.IsZero() {
		l = l.With(zap.String("edge", data.Edge.Source.String()+" -> "+data.Edge.Destination.String()))
	}
	switch strings.ToLower(level) {
	case "debug":
		l.Sugar().Debugf(message, args...)
	case "info":
		l.Sugar().Infof(message, args...)
	case "warn":
		l.Sugar().Warnf(message, args...)
	case "error":
		l.Sugar().Errorf(message, args...)
	default:
		l.Sugar().Warnf(message, args...)
	}
	return ""
}

func argToRID(arg any) (construct.ResourceId, error) {
	switch arg := arg.(type) {
	case construct.ResourceId:
		return arg, nil

	case construct.Resource:
		return arg.Id(), nil

	case string:
		var resId construct.ResourceId
		err := resId.UnmarshalText([]byte(arg))
		return resId, err
	}

	return construct.ResourceId{}, fmt.Errorf("invalid argument type %T", arg)
}

// Upstream returns the first resource that matches `selector` which is upstream of `resource`
func (ctx *DynamicValueContext) Upstream(selector any, resource construct.ResourceId) (construct.ResourceId, error) {
	selId, err := argToRID(selector)
	if err != nil {
		return construct.ResourceId{}, err
	}

	upstream := ctx.DAG.GetAllUpstreamResources(ctx.DAG.GetResource(resource))
	for _, up := range upstream {
		if selId.Matches(up.Id()) {
			return up.Id(), nil
		}
	}
	return construct.ResourceId{},
		fmt.Errorf("no upstream resource of '%s' found matching selector '%s'", resource, selId)
}

// AllUpstream is like Upstream but returns all transitive upstream resources.
// nolint: lll
func (ctx *DynamicValueContext) AllUpstream(selector any, resource construct.ResourceId) ([]construct.ResourceId, error) {
	selId, err := argToRID(selector)
	if err != nil {
		return nil, err
	}

	var matches []construct.ResourceId
	upstream := ctx.DAG.GetAllUpstreamResources(ctx.DAG.GetResource(resource))
	for _, up := range upstream {
		if selId.Matches(up.Id()) {
			matches = append(matches, up.Id())
		}
	}
	return matches, nil
}

// Downstream returns the first resource that matches `selector` which is downstream of `resource`
// nolint: lll
func (ctx *DynamicValueContext) Downstream(selector any, resource construct.ResourceId) (construct.ResourceId, error) {
	selId, err := argToRID(selector)
	if err != nil {
		return construct.ResourceId{}, err
	}

	downstream := ctx.DAG.GetAllDownstreamResources(ctx.DAG.GetResource(resource))
	for _, down := range downstream {
		if selId.Matches(down.Id()) {
			return down.Id(), nil
		}
	}
	return construct.ResourceId{},
		fmt.Errorf("no downstream resource of '%s' found matching selector '%s'", resource, selId)
}

// AllDownstream is like Downstream but returns all transitive downstream resources.
// nolint: lll
func (ctx *DynamicValueContext) AllDownstream(selector any, resource construct.ResourceId) ([]construct.ResourceId, error) {
	selId, err := argToRID(selector)
	if err != nil {
		return nil, err
	}

	var matches []construct.ResourceId
	downstream := ctx.DAG.GetAllDownstreamResources(ctx.DAG.GetResource(resource))
	for _, down := range downstream {
		if selId.Matches(down.Id()) {
			matches = append(matches, down.Id())
		}
	}
	return matches, nil
}

// ShortestPath returns all the resource IDs on the shortest path from source to destination
func (ctx *DynamicValueContext) ShortestPath(source, destination any) ([]construct.ResourceId, error) {
	srcId, err := argToRID(source)
	if err != nil {
		return nil, err
	}
	dstId, err := argToRID(destination)
	if err != nil {
		return nil, err
	}
	path, err := ctx.DAG.ShortestPath(srcId, dstId)
	if err != nil {
		return nil, err
	}
	pathIds := make([]construct.ResourceId, len(path))
	for i, p := range path {
		pathIds[i] = p.Id()
	}
	return pathIds, nil
}

// FieldValue returns the value of `field` on `resource` in json
func (ctx *DynamicValueContext) FieldValue(field string, resource any) (any, error) {
	resId, err := argToRID(resource)
	if err != nil {
		return "", err
	}

	r := ctx.DAG.GetResource(resId)
	if r == nil {
		return nil, fmt.Errorf("resource '%s' not found", resId)
	}

	fieldValue := reflect.ValueOf(r).Elem().FieldByName(field)
	if !fieldValue.IsValid() {
		return nil, fmt.Errorf("field '%s' not found on resource '%s'", field, resId)
	}
	return fieldValue.Interface(), nil
}

// FieldRef returns a reference to `field` on `resource` (as an IaCValue)
func (ctx *DynamicValueContext) FieldRef(field string, resource any) (construct.IaCValue, error) {
	resId, err := argToRID(resource)
	if err != nil {
		return construct.IaCValue{}, err
	}

	return construct.IaCValue{
		ResourceId: resId,
		Property:   field,
	}, nil
}

// toJson is used to return complex values that do not have TextUnmarshaler implemented
func (ctx *DynamicValueContext) toJson(value any) (string, error) {
	j, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	ctx.resultJson = true
	return string(j), nil
}

// filterMatch returns a json array by filtering the values array with the regex pattern
func filterMatch(pattern string, values []string) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	matches := make([]string, 0, len(values))
	for _, v := range values {
		if ok := re.MatchString(v); ok {
			matches = append(matches, v)
		}
	}
	return matches, nil
}

func filterIds(selector any, ids []construct.ResourceId) ([]construct.ResourceId, error) {
	selId, err := argToRID(selector)
	if err != nil {
		return nil, err
	}
	matches := make([]construct.ResourceId, 0, len(ids))
	for _, r := range ids {
		if selId.Matches(r) {
			matches = append(matches, r)
		}
	}
	return matches, nil
}

// mapstring takes in a regex pattern and replacement as well as a json array of strings
// roughly `unmarshal value | sed s/pattern/replace/g | marshal`
func mapString(pattern, replace string, values []string) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	nv := make([]string, len(values))
	for i, v := range values {
		nv[i] = re.ReplaceAllString(v, replace)
	}
	return nv, nil
}

// zipToMap returns a json map by zipping the keys and values arrays
// Example: zipToMap(['a', 'b'], [1, 2]) => {"a": 1, "b": 2}
func zipToMap(keys []string, valuesArg any) (map[string]any, error) {
	// Have to use reflection here because technically, []string is not assignable to []any
	// thanks Go.
	valuesRefl := reflect.ValueOf(valuesArg)
	if valuesRefl.Kind() != reflect.Slice && valuesRefl.Kind() != reflect.Array {
		return nil, fmt.Errorf("values is not a slice or array")
	}
	if len(keys) != valuesRefl.Len() {
		return nil, fmt.Errorf("key length (%d) != value length (%d)", len(keys), valuesRefl.Len())
	}

	m := make(map[string]any)
	for i, k := range keys {
		m[k] = valuesRefl.Index(i).Interface()
	}
	return m, nil
}

// keysToMapWithDefault returns a json map by mapping the keys array to the static defaultValue
// Example keysToMapWithDefault(0, ['a', 'b']) => {"a": 0, "b": 0}
func keysToMapWithDefault(defaultValue any, keys []string) (map[string]any, error) {
	m := make(map[string]any)
	for _, k := range keys {
		m[k] = defaultValue
	}
	return m, nil
}

func add(args ...int) int {
	total := 0
	for _, a := range args {
		total += a
	}
	return total
}

func sub(args ...int) int {
	if len(args) == 0 {
		return 0
	}
	total := args[0]
	for _, a := range args[1:] {
		total -= a
	}
	return total
}

func replaceRegex(pattern, replace, value string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	s := re.ReplaceAllString(value, replace)
	return s, nil
}
