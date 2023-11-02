package iac3

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/iancoleman/strcase"
	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/set"
)

type templateInputArgs map[string]any

func (tc *TemplatesCompiler) RenderResource(out io.Writer, rid construct.ResourceId) error {
	resTmpl, err := tc.templates.ResourceTemplate(rid)
	if err != nil {
		return err
	}
	r, err := tc.graph.Vertex(rid)
	if err != nil {
		return err
	}
	inputs, err := tc.getInputArgs(r, resTmpl)
	if err != nil {
		return err
	}

	if resTmpl.OutputType != "void" {
		_, err = fmt.Fprintf(out, "const %s = ", tc.vars[rid])
		if err != nil {
			return err
		}
	}
	err = resTmpl.Template.Execute(out, inputs)
	if err != nil {
		return fmt.Errorf("could not render resource %s: %w", rid, err)
	}

	return nil
}

func (tc *TemplatesCompiler) convertArg(arg any, templateArg *Arg) (any, error) {

	switch arg := arg.(type) {
	case construct.ResourceId:
		return tc.vars[arg], nil

	case construct.PropertyRef:
		return tc.PropertyRefValue(arg)

	case string:
		// use templateString to quote the string value
		return templateString(arg), nil

	case bool, int, float64:
		// safe to use as-is
		return arg, nil

	case nil:
		// don't add to inputs
		return nil, nil

	default:
		switch val := reflect.ValueOf(arg); val.Kind() {
		case reflect.Slice, reflect.Array:
			list := &TsList{l: make([]any, 0, val.Len())}
			for i := 0; i < val.Len(); i++ {
				if !val.Index(i).IsValid() || val.Index(i).IsNil() {
					continue
				}
				output, err := tc.convertArg(val.Index(i).Interface(), templateArg)
				if err != nil {
					return "", err
				}
				list.Append(output)
			}
			return list, nil
		case reflect.Map:
			TsMap := &TsMap{m: make(map[string]any)}
			for _, key := range val.MapKeys() {
				if !val.MapIndex(key).IsValid() || val.MapIndex(key).IsNil() {
					continue
				}
				keyStr, found := key.Interface().(string)
				if !found {
					return "", fmt.Errorf("map key is not a string")
				}
				keyResult := strcase.ToLowerCamel(keyStr)
				if templateArg != nil && templateArg.Wrapper == string(CamelCaseWrapper) {
					keyResult = strcase.ToCamel(keyStr)
				} else if templateArg != nil && templateArg.Wrapper == string(ModelCaseWrapper) {
					keyResult = keyStr
				}

				output, err := tc.convertArg(val.MapIndex(key).Interface(), templateArg)
				if err != nil {
					return "", err
				}
				TsMap.SetKey(keyResult, output)
			}
			return TsMap, nil
		case reflect.Struct:
			if hashset, ok := val.Interface().(set.HashedSet[string, any]); ok {
				return tc.convertArg(hashset.ToSlice(), templateArg)
			}
			fallthrough
		default:
			return jsonValue{Raw: arg}, nil
		}
	}
}

func (tc *TemplatesCompiler) getInputArgs(r *construct.Resource, template *ResourceTemplate) (templateInputArgs, error) {
	var errs error
	inputs := make(map[string]any, len(r.Properties)+len(globalVariables)+2) // +2 for Name and dependsOn

	for name, value := range r.Properties {
		templateArg := template.Args[name]
		var argValue any
		var err error
		if templateArg.Wrapper == string(TemplateWrapper) {
			argValue, err = tc.useNestedTemplate(template, value, templateArg)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("could not use nested template for arg %q: %w", name, err))
				continue
			}
		} else {
			argValue, err = tc.convertArg(value, &templateArg)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("could not convert arg %q: %w", name, err))
				continue
			}
		}

		if argValue != nil {
			inputs[name] = argValue
		}
	}
	if errs != nil {
		return templateInputArgs{}, errs
	}

	downstream, err := construct.DirectDownstreamDependencies(tc.graph, r.ID)
	if err != nil {
		return templateInputArgs{}, err
	}
	var dependsOn []string
	var applied appliedOutputs
	for _, dep := range downstream {
		switch dep.QualifiedTypeName() {
		case "aws:region", "aws:availability_zone", "aws:account_id":
			continue

		case "kubernetes:helm_chart":
			ao := tc.NewAppliedOutput(construct.PropertyRef{
				Resource: dep,
				// ready: pulumi.Output<pulumi.CustomResource[]>
				Property: "ready",
			}, "")
			applied = append(applied, ao)
			dependsOn = append(dependsOn, "..."+ao.Name)

		case "kubernetes:manifest", "kubernetes:kustomize_directory":
			ao := tc.NewAppliedOutput(construct.PropertyRef{
				Resource: dep,
				// resources: pulumi.Output<{
				// 		[key: string]: pulumi.CustomResource;
				// }>
				Property: "resources",
			}, "")
			applied = append(applied, ao)
			dependsOn = append(dependsOn, fmt.Sprintf("...Object.values(%s)", ao.Name))

		default:
			dependsOn = append(dependsOn, tc.vars[dep])
		}
	}
	sort.Strings(dependsOn)
	if len(applied) > 0 {
		buf := getBuffer()
		defer releaseBuffer(buf)
		err = applied.Render(buf, func(w io.Writer) error {
			return json.NewEncoder(w).Encode(dependsOn)
		})
		if err != nil {
			return templateInputArgs{}, err
		}
		inputs["dependsOn"] = buf.String()
	} else {
		inputs["dependsOn"] = "[" + strings.Join(dependsOn, ", ") + "]"
	}

	inputs["Name"] = templateString(r.ID.Name)

	for g := range globalVariables {
		inputs[g] = g
	}

	return inputs, nil
}

func (tc *TemplatesCompiler) useNestedTemplate(resTmpl *ResourceTemplate, val any, arg Arg) (string, error) {

	var contents []byte
	var err error

	nestedTemplatePath := path.Join(resTmpl.Path, strcase.ToSnake(arg.Name)+".ts.tmpl")

	f, err := tc.templates.fs.Open(nestedTemplatePath)
	if err != nil {
		return "", fmt.Errorf("could not find template for %s: %w", nestedTemplatePath, err)
	}
	contents, err = io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("could not read template for %s: %w", nestedTemplatePath, err)
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("no contents in template for %s: %w", nestedTemplatePath, err)
	}

	tmpl, err := template.New(nestedTemplatePath).Funcs(template.FuncMap{
		// "parseVal":       tc.parseVal,
		"modelCase":      tc.modelCase,
		"lowerCamelCase": tc.lowerCamelCase,
		"camelCase":      tc.camelCase,
		"getVar": func(id construct.ResourceId) string {
			return tc.vars[id]
		},
	}).Parse(string(contents))
	if err != nil {
		return "", fmt.Errorf("could not parse template for %s: %w", nestedTemplatePath, err)
	}
	result := getBuffer()
	err = tmpl.Execute(result, val)
	if err != nil {
		return "", fmt.Errorf("could not execute template for %s: %w", nestedTemplatePath, err)
	}
	return result.String(), nil
}

// func (tc *TemplatesCompiler) parseVal(val any) (any, error) {
// 	return tc.convertArg(val, nil)
// }

func (tc *TemplatesCompiler) modelCase(val any) (any, error) {
	return tc.convertArg(val, &Arg{Wrapper: string(ModelCaseWrapper)})
}

func (tc *TemplatesCompiler) lowerCamelCase(val any) (any, error) {
	return tc.convertArg(val, &Arg{Wrapper: string(LowerCamelCaseWrapper)})
}

func (tc *TemplatesCompiler) camelCase(val any) (any, error) {
	return tc.convertArg(val, &Arg{Wrapper: string(CamelCaseWrapper)})
}
