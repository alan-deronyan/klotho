package visualizer

import (
	"io"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/ioutil"
)

type (
	TypesChecker struct {
		DAG *construct.ResourceGraph
	}
)

func (tc TypesChecker) WriteTo(w io.Writer) (n int64, err error) {
	wh := ioutil.NewWriteToHelper(w, &n, &err)

	for _, res := range tc.DAG.ListResources() {
		wh.Writef("%s\n", TypeFor(res, tc.DAG))
	}
	return
}
