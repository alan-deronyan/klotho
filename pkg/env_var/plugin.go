package envvar

import (
	"errors"
	"fmt"
	"strings"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/config"

	"github.com/klothoplatform/klotho/pkg/annotation"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/logging"
	"github.com/klothoplatform/klotho/pkg/multierr"
	"go.uber.org/zap"
)

type (
	EnvVarInjection struct {
		Config *config.Application
	}
)

var (
	SupportedKindValues = map[string][]string{
		"orm":           {"connection_string"},
		"redis_node":    {"host", "port"},
		"redis_cluster": {"host", "port"},
	}
)

func (p EnvVarInjection) Name() string { return "EnvVarInjection" }

func (p EnvVarInjection) Transform(input *types.InputFiles, fileDeps *types.FileDependencies, constructGraph *construct.ConstructGraph) error {
	var errs multierr.Error

	units := construct.GetConstructsOfType[*types.ExecutionUnit](constructGraph)
	for _, unit := range units {
		for _, f := range unit.Files() {
			log := zap.L().With(logging.FileField(f)).Sugar()
			ast, ok := f.(*types.SourceFile)
			if !ok {
				log.Debug("Skipping non-source file")
				continue
			}

			for _, annot := range ast.Annotations() {
				cap := annot.Capability
				if cap.Name == annotation.PersistCapability {
					if cap.ID == "" {
						errs.Append(types.NewCompilerError(ast, annot, errors.New("'id' is required")))
					}
					directiveResult, err := ParseDirectiveToEnvVars(cap)
					if err != nil {
						errs.Append(err)
						continue
					}
					if directiveResult.kind == "" {
						continue
					}
					err = handlePersist(directiveResult, cap, unit, constructGraph)
					if err != nil {
						errs.Append(err)
						continue
					}
				}
			}
		}

	}
	return errs.ErrOrNil()
}

func validateValue(kind string, value string) bool {
	for _, v := range SupportedKindValues[kind] {
		if v == value {
			return true
		}
	}
	return false
}

type EnvironmentVariableDirectiveResult struct {
	kind      string
	variables types.EnvironmentVariables
}

func ParseDirectiveToEnvVars(cap *annotation.Capability) (EnvironmentVariableDirectiveResult, error) {
	overallKind := ""
	envVars := cap.Directives.Object(types.EnvironmentVariableDirective)
	foundVars := types.EnvironmentVariables{}
	if envVars == nil {
		return EnvironmentVariableDirectiveResult{}, nil
	}
	for name, v := range envVars {

		v, ok := v.(string)
		if !ok {
			return EnvironmentVariableDirectiveResult{}, errors.New("environment variable directive must have values as strings")
		}
		valueSplit := strings.Split(v, ".")
		if len(valueSplit) != 2 {
			return EnvironmentVariableDirectiveResult{}, errors.New("invalid environment variable directive value")
		}

		kind := valueSplit[0]
		value := valueSplit[1]

		_, ok = SupportedKindValues[kind]
		if !ok {
			return EnvironmentVariableDirectiveResult{}, errors.New("invalid value for 'kind' of environment variable value")
		}

		if !validateValue(kind, value) {
			return EnvironmentVariableDirectiveResult{}, fmt.Errorf("value, %s, is not valid for kind, %s", value, kind)
		}

		if overallKind == "" {
			overallKind = kind
		} else if overallKind != kind {
			return EnvironmentVariableDirectiveResult{}, errors.New("cannot have multiple resource kinds in environment variables for single annotation")
		}

		foundVariable := types.NewEnvironmentVariable(name, nil, value)

		foundVars.Add(foundVariable)
	}

	return EnvironmentVariableDirectiveResult{kind: overallKind, variables: foundVars}, nil
}

func handlePersist(directiveResult EnvironmentVariableDirectiveResult, cap *annotation.Capability, unit *types.ExecutionUnit, constructGraph *construct.ConstructGraph) error {

	var resource construct.Construct
	switch directiveResult.kind {
	case "orm":
		resource = &types.Orm{Name: cap.ID}
	case "redis_cluster":
		resource = &types.RedisCluster{Name: cap.ID}
	case "redis_node":
		resource = &types.RedisNode{Name: cap.ID}
	default:
		return fmt.Errorf("unsupported 'kind', %s", directiveResult.kind)
	}

	constructGraph.AddConstruct(resource)
	constructGraph.AddDependency(unit.Id(), resource.Id())
	variables := types.EnvironmentVariables{}
	for _, variable := range directiveResult.variables {
		variable.Construct = resource
		variables = append(variables, variable)
	}
	unit.EnvironmentVariables.AddAll(variables)
	return nil
}
