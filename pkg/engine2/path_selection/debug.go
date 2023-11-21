package path_selection

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/dot"
	"github.com/klothoplatform/klotho/pkg/set"
	"go.uber.org/zap"
)

// seenFiles is used to keep track of which files have been added to by this execution
// so that it can tell when to append (when already seen by this execution) or truncate
// (to reset between executions)
var seenFiles = make(set.Set[string])

func writeGraph(input ExpansionInput, working, result construct.Graph) {
	err := os.Mkdir("selection", 0755)
	if err != nil && !errors.Is(err, os.ErrExist) {
		zap.S().Warnf("Could not create folder for selection diagram: %v", err)
		return
	}

	fprefix := fmt.Sprintf("selection/%s-%s", input.Dep.Source.ID, input.Dep.Target.ID)
	f, err := os.OpenFile(fprefix+".gv", os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		zap.S().Errorf("could not create file %s: %v", fprefix, err)
		return
	}
	defer f.Close()

	if !seenFiles.Contains(f.Name()) {
		seenFiles.Add(f.Name())
		err := f.Truncate(0)
		if err != nil {
			zap.S().Errorf("could not truncate file %s: %v", f.Name(), err)
			return
		}
	}

	dotContent := new(bytes.Buffer)
	_, err = io.Copy(dotContent, f)
	if err != nil {
		zap.S().Errorf("could not read file %s: %v", f.Name(), err)
		return
	}

	if dotContent.Len() > 0 {
		content := strings.TrimSpace(dotContent.String())
		content = strings.TrimSuffix(content, "}")
		dotContent.Reset()
		dotContent.WriteString(content)
	} else {
		fmt.Fprintf(dotContent, `digraph {
  label = "%s → %s"
  rankdir = LR
  labelloc = t
  graph [ranksep = 2]
`, input.Dep.Source.ID, input.Dep.Target.ID)
	}

	err = graphToDOTCluster(input.Classification, working, result, dotContent)
	if err != nil {
		zap.S().Errorf("could not render graph for %s: %v", fprefix, err)
		return
	}

	fmt.Fprintln(dotContent, "}")

	content := dotContent.String()

	_, err = f.Seek(0, 0)
	if err == nil {
		_, err = io.Copy(f, strings.NewReader(content))
	}
	if err != nil {
		zap.S().Errorf("could not write file %s: %v", f.Name(), err)
		return
	}

	svgContent, err := dot.ExecPan(strings.NewReader(content))
	if err != nil {
		zap.S().Errorf("could not render graph to file %s: %v", fprefix, err)
		return
	}

	svgFile, err := os.Create(fprefix + ".gv.svg")
	if err != nil {
		zap.S().Errorf("could not create file %s.gv.svg: %v", fprefix, err)
		return
	}
	defer svgFile.Close()
	fmt.Fprint(svgFile, svgContent)
}