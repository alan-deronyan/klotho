package engine2

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/dot"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
)

func dotAttributes(kb knowledgebase.TemplateKB, r *construct.Resource) map[string]string {
	a := make(map[string]string)
	a["label"] = r.ID.String()
	a["shape"] = "box"
	tmpl, _ := kb.GetResourceTemplate(r.ID)
	if tmpl != nil && len(tmpl.Classification.Is) > 0 {
		a["label"] += fmt.Sprintf("\n%v", tmpl.Classification.Is)
	}
	return a
}

func dotEdgeAttributes(kb knowledgebase.TemplateKB, g construct.Graph, e construct.ResourceEdge) map[string]string {
	a := make(map[string]string)
	_ = e.Source.WalkProperties(func(path construct.PropertyPath, nerr error) error {
		v := path.Get()
		if v == e.Target.ID {
			a["label"] = path.String()
			return construct.StopWalk
		}
		return nil
	})
	if e.Properties.Weight > 0 {
		if a["label"] == "" {
			a["label"] = fmt.Sprintf("%d", e.Properties.Weight)
		} else {
			a["label"] = fmt.Sprintf("%s\n%d", a["label"], e.Properties.Weight)
		}
	}
	sideEffect, err := knowledgebase.IsOperationalResourceSideEffect(g, kb, e.Source.ID, e.Target.ID)
	if err == nil && sideEffect {
		a["color"] = "green"
	}
	return a
}

func GraphToDOT(kb knowledgebase.TemplateKB, g construct.Graph, out io.Writer) error {
	ids, err := construct.TopologicalSort(g)
	if err != nil {
		return err
	}
	nodes, err := construct.ResolveIds(g, ids)
	if err != nil {
		return err
	}
	var errs error
	printf := func(s string, args ...any) {
		_, err := fmt.Fprintf(out, s, args...)
		errs = errors.Join(errs, err)
	}
	printf(`digraph {
  rankdir = TB
`)
	for _, n := range nodes {
		printf("  %q%s\n", n.ID, dot.AttributesToString(dotAttributes(kb, n)))
	}

	topoIndex := func(id construct.ResourceId) int {
		for i, id2 := range ids {
			if id2 == id {
				return i
			}
		}
		return -1
	}
	edges, err := g.Edges()
	if err != nil {
		return err
	}
	sort.Slice(edges, func(i, j int) bool {
		ti, tj := topoIndex(edges[i].Source), topoIndex(edges[j].Source)
		if ti != tj {
			return ti < tj
		}
		ti, tj = topoIndex(edges[i].Target), topoIndex(edges[j].Target)
		return ti < tj
	})
	for _, e := range edges {
		edge, err := g.Edge(e.Source, e.Target)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		printf("  %q -> %q%s\n", e.Source, e.Target, dot.AttributesToString(dotEdgeAttributes(kb, g, edge)))
	}
	printf("}\n")
	return errs
}

func GraphToSVG(kb knowledgebase.TemplateKB, g construct.Graph, prefix string) error {
	if debugDir := os.Getenv("KLOTHO_DEBUG_DIR"); debugDir != "" {
		prefix = filepath.Join(debugDir, prefix)
	}
	f, err := os.Create(prefix + ".gv")
	if err != nil {
		return err
	}
	defer f.Close()

	dotContent := new(bytes.Buffer)
	err = GraphToDOT(kb, g, io.MultiWriter(f, dotContent))
	if err != nil {
		return fmt.Errorf("could not render graph to file %s: %v", prefix+".gv", err)
	}

	svgContent, err := dot.ExecPan(bytes.NewReader(dotContent.Bytes()))
	if err != nil {
		return fmt.Errorf("could not run 'dot' for %s: %v", prefix+".gv", err)
	}

	svgFile, err := os.Create(prefix + ".gv.svg")
	if err != nil {
		return fmt.Errorf("could not create file %s: %v", prefix+".gv.svg", err)
	}
	defer svgFile.Close()
	_, err = fmt.Fprint(svgFile, svgContent)
	return err
}
