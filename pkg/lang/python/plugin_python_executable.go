package python

import (
	"github.com/klothoplatform/klotho/pkg/annotation"
	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	klotho_errors "github.com/klothoplatform/klotho/pkg/errors"
	execunit "github.com/klothoplatform/klotho/pkg/exec_unit"
	"go.uber.org/zap"
)

var upstreamDependencyResolver = execunit.SourceFilesResolver{
	UnitFileDependencyResolver: UnitFileDependencyResolver,
	UpstreamAnnotations:        []string{annotation.ExposeCapability},
}

type PythonExecutable struct {
}

func (l PythonExecutable) Name() string {
	return "python_executable"
}

func (l PythonExecutable) Transform(input *types.InputFiles, fileDeps *types.FileDependencies, constructGraph *construct.ConstructGraph) error {
	inputFiles := input.Files()

	defaultRequirementsTxt, _ := inputFiles["requirements.txt"].(*RequirementsTxt)
	for _, unit := range construct.GetConstructsOfType[*types.ExecutionUnit](constructGraph) {
		if unit.Executable.Type != "" {
			zap.L().Sugar().Debugf("Skipping exececution unit '%s': executable type is already set to '%s'", unit.Name, unit.Executable.Type)
			continue
		}

		requirementsTxt := defaultRequirementsTxt
		requirementsTxtPath := types.CheckForProjectFile(input, unit, "requirements.txt")
		if requirementsTxtPath != "" {
			requirementsTxt, _ = inputFiles[requirementsTxtPath].(*RequirementsTxt)
		}
		if requirementsTxt == nil {
			zap.L().Sugar().Debugf("requirements.txt not found in execution_unit: %s", unit.Name)
			return nil
		}

		unit.AddResource(requirementsTxt.Clone())
		unit.Executable.Type = types.ExecutableTypePython

		for _, file := range unit.FilesOfLang(py) {
			for _, annot := range file.Annotations() {
				cap := annot.Capability
				if cap.Name == annotation.ExecutionUnitCapability && cap.ID == unit.Name {
					unit.AddEntrypoint(file)
				}
			}
		}

		if len(unit.Executable.Entrypoints) == 0 {
			resolveDefaultEntrypoint(unit)
		}

		err := refreshSourceFiles(unit)
		if err != nil {
			return err
		}
		refreshUpstreamEntrypoints(unit)
	}
	return nil
}

func refreshUpstreamEntrypoints(unit *types.ExecutionUnit) {
	for f := range unit.Executable.SourceFiles {
		if file, ok := unit.Get(f).(*types.SourceFile); ok && file.IsAnnotatedWith(annotation.ExposeCapability) {
			zap.L().Sugar().Debugf("Adding execution unit entrypoint: [@klotho::expose] -> [%s] -> %s", unit.Name, f)
			unit.AddEntrypoint(file)
		}
	}
}

func refreshSourceFiles(unit *types.ExecutionUnit) error {
	sourceFiles, err := upstreamDependencyResolver.Resolve(unit)
	if err != nil {
		return klotho_errors.WrapErrf(err, "file dependency resolution failed for execution unit: %s", unit.Name)
	}
	for k, v := range sourceFiles {
		unit.Executable.SourceFiles[k] = v
	}
	return err
}

func resolveDefaultEntrypoint(unit *types.ExecutionUnit) {
	for _, fallbackPath := range []string{"main.py", "app/main.py", "app.py", "app/app.py"} {
		if entrypoint := unit.Get(fallbackPath); entrypoint != nil {
			zap.L().Sugar().Debugf("Adding execution unit entrypoint: [default] -> [%s] -> %s", unit.Name, entrypoint.Path())
			unit.AddEntrypoint(entrypoint)
		}
	}
}
