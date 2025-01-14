package coretesting

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/graph"
	"github.com/stretchr/testify/assert"
)

type (
	StringDep = graph.Edge[string]

	ResourcesExpectation struct {
		Nodes []string
		Deps  []StringDep

		// AssertSubset assert the dag contains all the `.Nodes` and `.Deps`. If false,
		// checks full equality.
		AssertSubset bool
	}
)

func (expect ResourcesExpectation) Assert(t *testing.T, dag *construct.ResourceGraph) {
	got := ResoucesFromDAG(dag)

	if expect.AssertSubset {
		assert.Subset(t, got.Nodes, expect.Nodes)
		assert.Subset(t, got.Deps, expect.Deps)
	} else {
		expect.ElementsMatchPretty(t, expect.Nodes, got.Nodes)
		expect.ElementsMatchPretty(t, expect.Deps, got.Deps)
	}
}

func (expect ResourcesExpectation) AssertConstructs(t *testing.T, graph *construct.ConstructGraph) {
	got := ConstructsFromGraph(graph)

	if expect.AssertSubset {
		assert.Subset(t, got.Nodes, expect.Nodes)
		assert.Subset(t, got.Deps, expect.Deps)
	} else {
		expect.ElementsMatchPretty(t, expect.Nodes, got.Nodes)
		expect.ElementsMatchPretty(t, expect.Deps, got.Deps)
	}
}

// ElementsMatchPretty invokes [assert.ElementsMatch], but first does a string-based check based on the elements;
// string representation. This means in the common case that unequal strings are enough to demonstrate inequality, we'll
// get a nicer diff.
func (expect ResourcesExpectation) ElementsMatchPretty(t *testing.T, expected any, actual any) {
	toStr := func(obj any) string {
		objVal := reflect.ValueOf(obj)
		if objVal.Type().Kind() != reflect.Slice && objVal.Type().Kind() != reflect.Array {
			return ""
		}
		arrLen := objVal.Len()
		var res []string
		for i := 0; i < arrLen; i++ {
			res = append(res, fmt.Sprintf(`%+v`, objVal.Index(i).Interface()))
		}
		sort.Strings(res)
		return strings.Join(res, "\n")
	}

	expectedStr := toStr(expected)
	actualStr := toStr(actual)
	if !assert.Equal(t, expectedStr, actualStr) {
		return
	}

	assert.ElementsMatch(t, expected, actual)
}

func ResoucesFromDAG(dag *construct.ResourceGraph) ResourcesExpectation {
	var nodes []string
	for _, r := range dag.ListResources() {
		nodes = append(nodes, r.Id().String())
	}
	var deps []StringDep
	for _, e := range dag.ListDependencies() {
		deps = append(deps, StringDep{Source: e.Source.Id().String(), Destination: e.Destination.Id().String()})
	}

	return ResourcesExpectation{
		Nodes: nodes,
		Deps:  deps,
	}
}

func ConstructsFromGraph(graph *construct.ConstructGraph) ResourcesExpectation {
	var nodes []string
	for _, r := range construct.ListConstructs[construct.BaseConstruct](graph) {
		nodes = append(nodes, r.Id().String())
	}
	var deps []StringDep
	for _, e := range graph.ListDependencies() {
		deps = append(deps, StringDep{Source: e.Source.Id().String(), Destination: e.Destination.Id().String()})
	}

	return ResourcesExpectation{
		Nodes: nodes,
		Deps:  deps,
	}
}

// GoString is useful in combination with `ResoucesFromDAG` to generate or update unit tests. Make sure to read over
// the results before using to make sure it is correct.
// For example:
//
//	fmt.Print(coretesting.ResoucesFromDAG(dag).GoString())
func (expect ResourcesExpectation) GoString() string {
	buf := new(strings.Builder)
	buf.WriteString("coretesting.ResourcesExpectation{\n")

	nodes := make([]string, len(expect.Nodes))
	copy(nodes, expect.Nodes)
	sort.Strings(nodes)
	buf.WriteString("	Nodes: []string{\n")
	for _, n := range nodes {
		fmt.Fprintf(buf, "		%s,\n", strconv.Quote(n))
	}
	buf.WriteString("	},\n")

	edges := make([]StringDep, len(expect.Deps))
	copy(edges, expect.Deps)
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Source == b.Source {
			return a.Destination < b.Destination
		}
		return a.Source < b.Source
	})
	buf.WriteString("	Deps: []coretesting.StringDep{\n")
	for _, e := range edges {
		fmt.Fprintf(buf, "		{Source: %s, Destination: %s},\n", strconv.Quote(e.Source), strconv.Quote(e.Destination))
	}
	buf.WriteString("	},\n")

	buf.WriteString("}\n")

	return buf.String()
}
