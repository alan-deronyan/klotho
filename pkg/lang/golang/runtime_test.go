package golang

import (
	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
)

type NoopRuntime struct{}

func (n NoopRuntime) AddExecRuntimeFiles(unit *types.ExecutionUnit, constructGraph *construct.ConstructGraph) error {
	return nil
}
func (n NoopRuntime) GetFsImports() []Import {
	return []Import{
		{Package: "gocloud.dev/blob"},
		{Alias: "_", Package: "gocloud.dev/blob/s3blob"},
	}
}
func (n NoopRuntime) GetSecretsImports() []Import {
	return []Import{
		{Package: "gocloud.dev/runtimevar"},
		{Alias: "_", Package: "gocloud.dev/runtimevar/awssecretsmanager"},
	}
}

func (n NoopRuntime) SetConfigType(id string, isSecret bool) {
}

func (n NoopRuntime) ActOnExposeListener(unit *types.ExecutionUnit, f *types.SourceFile, listener *HttpListener, routerName string) error {
	return nil
}
