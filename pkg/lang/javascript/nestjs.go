package javascript

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/config"
	"github.com/klothoplatform/klotho/pkg/filter"
	"github.com/klothoplatform/klotho/pkg/filter/predicate"

	"github.com/klothoplatform/klotho/pkg/annotation"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/logging"
	"github.com/klothoplatform/klotho/pkg/multierr"
	"github.com/klothoplatform/klotho/pkg/query"
	"github.com/pkg/errors"
	sitter "github.com/smacker/go-tree-sitter"
	"go.uber.org/zap"
)

type NestJsHandler struct {
	output nestJsOutput
	log    *zap.Logger
	Config *config.Application
}

type nestJsOutput struct {
	factories   []nestFactoryResult
	controllers []query.Reference
	modules     []query.Reference
	routes      []query.Reference
}

func (p NestJsHandler) Name() string { return "NestJs" }

func (p NestJsHandler) Transform(input *types.InputFiles, fileDeps *types.FileDependencies, constructGraph *construct.ConstructGraph) error {
	var errs multierr.Error
	for _, unit := range construct.GetConstructsOfType[*types.ExecutionUnit](constructGraph) {
		err := p.transformSingle(constructGraph, unit)
		errs.Append(err)
	}
	return errs.ErrOrNil()
}

func (p *NestJsHandler) transformSingle(constructGraph *construct.ConstructGraph, unit *types.ExecutionUnit) error {

	execUnitInfo := execUnitExposeInfo{Unit: unit, RoutesByGateway: make(map[gatewaySpec][]gatewayRouteDefinition)}
	p.log = zap.L().With(zap.String("unit", unit.Name))

	var errs multierr.Error

	for _, f := range unit.Files() {
		js, ok := Language.ID.CastFile(f)
		if !ok {
			continue
		}
		newF, err := p.handleFile(js, unit)
		if err != nil {
			errs.Append(err)
			continue
		}
		if newF != nil {
			unit.Add(newF)
		}
	}
	for _, f := range unit.Files() {
		js, ok := Language.ID.CastFile(f)
		if !ok {
			continue
		}
		p.queryResources(js)
	}
	err := p.assignRoutesToGateway(&execUnitInfo)
	errs.Append(err)

	handleGatewayRoutes(&execUnitInfo, constructGraph, p.log)
	return errs.ErrOrNil()
}

func (p *NestJsHandler) handleFile(f *types.SourceFile, unit *types.ExecutionUnit) (*types.SourceFile, error) {
	annots := f.Annotations()

	fileContent := string(f.Program())
	for _, annot := range annots {
		log := zap.L().With(logging.AnnotationField(annot), logging.FileField(f))
		cap := annot.Capability
		if annot.IsDetached() || cap.Name != annotation.ExposeCapability {
			continue
		}

		if cap.ID == "" {
			return nil, types.NewCompilerError(f, annot, errors.New("'id' is required"))
		}

		// target can be public or private for now
		// currently private is unimplemented, so
		// we fail unless it's set to public
		// TODO: we should also link to documentation when
		// it's available
		target, ok := cap.Directives.String("target")
		if !ok {
			target = "private"
		}
		if target != "public" {
			return nil, types.NewCompilerError(f, annot, errors.New("expose capability must specify target = \"public\""))

		}

		listen := findListener(annot)

		if listen.Expression == nil {
			log.Debug("No listener found")
			continue
		}

		appName, err := findApp(listen)
		if err != nil {
			return nil, types.NewCompilerError(f, annot, errors.New("Couldn't find expose app creation"))
		}

		actedOn, newfileContent := p.actOnAnnotation(f, &listen, fileContent, appName, p.Config.GetResourceType(unit), annot)
		if actedOn {
			fileContent = newfileContent
			err := f.Reparse([]byte(fileContent))
			if err != nil {
				return f, errors.Wrap(err, "error reparsing after substitutions")
			}
		}
	}

	return f, nil
}

func (h *NestJsHandler) assignRoutesToGateway(info *execUnitExposeInfo) error {
	var errs multierr.Error

	controllers := h.findControllers(info.Unit.Name)
	modules := h.findModules(controllers)

	for _, factory := range h.output.factories {
		found, ok := modules[factory.moduleImportName]
		if !ok {
			continue
		}
		relPath, err := filepath.Rel(filepath.Dir(factory.f.Path()), found.f.Path())
		if err != nil {
			errs.Append(err)
		}
		if FileToLocalModule(relPath) == factory.moduleImportPath {
			for _, c := range found.controllers {
				gwSpec := gatewaySpec{
					FilePath:   factory.f.Path(),
					AppVarName: factory.appName,
					gatewayId:  factory.id,
				}
				if len(c.routes) == 0 {
					h.log.Sugar().Warnf("No routes found for controller '%s'", c.name)
				} else {
					h.log.Sugar().Infof("Found %d route(s) for controller '%s'", len(c.routes), c.name)
				}
				info.RoutesByGateway[gwSpec] = append(info.RoutesByGateway[gwSpec], c.routes...)
			}
		}
	}
	return errs.ErrOrNil()
}
func (h *NestJsHandler) actOnAnnotation(f *types.SourceFile, listen *exposeListenResult, fileContent string, appName string, unitType string, annot *types.Annotation) (actedOn bool, newfileContent string) {
	nestFactory := h.findNestFactory(f)
	newfileContent = fileContent
	actedOn = false
	if nestFactory.varName == "" {
		return
	}

	if listen.Identifier.Content() != nestFactory.varName {
		return
	}

	//TODO: look into moving this runtime-specific logic elsewhere
	if unitType == "lambda" {
		// After CommentNode, `listen` is not a valid node
		if listen.Expression.Parent().Parent().Type() == "await_expression" {
			newfileContent = CommentNodes(fileContent, listen.Expression.Parent().Parent().Content())
		} else {
			newfileContent = CommentNodes(fileContent, listen.Expression.Content())
		}
		annot.Detach() // prevents this annotation from being rebound to the next non-comment node in the file on reparse
	}

	nestFactory.appName = appName
	nestFactory.id = annot.Capability.ID
	h.output.factories = append(h.output.factories, nestFactory)

	newfileContent += fmt.Sprintf(`
	exports.%s = %s
	`, strings.TrimPrefix(appName, "exports."), appName)
	actedOn = true
	return
}

type nestFactoryResult struct {
	varName          string
	moduleImportName string
	moduleImportPath string
	appName          string
	id               string
	f                *types.SourceFile
}

func (h *NestJsHandler) findNestFactory(f *types.SourceFile) nestFactoryResult {
	nextMatch := DoQuery(f.Tree().RootNode(), nestJsFactory)
	for {
		match, found := nextMatch()
		if !found {
			break
		}

		varName, moduleImportId, moduleProp := match["var"], match["id"], match["moduleProp"]

		if !validateNestFactory(match, f) {
			continue
		}

		imp := FindImportForVar(f.Tree().RootNode(), moduleImportId.Content())
		return nestFactoryResult{
			f:                f,
			varName:          varName.Content(),
			moduleImportName: moduleProp.Content(),
			moduleImportPath: imp.Source,
		}
	}
	return nestFactoryResult{}
}

func (h *NestJsHandler) queryResources(f *types.SourceFile) {

	h.output.controllers = append(h.output.controllers, query.FindReferencesInFile(
		f,
		nestJsController,
		validateController,
	)...)

	h.output.modules = append(h.output.modules, query.FindReferencesInFile(
		f,
		nestJsModule,
		ValidateModule,
	)...)

	h.output.routes = append(h.output.routes, query.FindReferencesInFile(
		f,
		nestJsRoute,
		validateRoute,
	)...)
}

type nestController struct {
	f      *types.SourceFile
	routes []gatewayRouteDefinition
	name   string
}

func (h *NestJsHandler) findControllers(unitName string) map[string]nestController {
	controllers := make(map[string]nestController)
	for _, ref := range h.output.controllers {
		f := ref.File
		result := ref.QueryResult

		varName, basePath := result["name"], result["basePath"]

		controllerName := varName.Content()

		routes := h.findRoutesForController(controllerName, StringLiteralContent(basePath), unitName)

		controllers[controllerName] = nestController{
			f:      f,
			routes: routes,
			name:   controllerName,
		}

	}
	return controllers
}

func (h *NestJsHandler) findRoutesForController(controllerName string, basePath string, unitName string) []gatewayRouteDefinition {
	var routes []gatewayRouteDefinition
	for _, ref := range h.output.routes {
		f := ref.File
		result := ref.QueryResult

		controller, method, routePath := result["controller"], result["method"], result["path"]

		if controller.Content() != controllerName {
			continue
		}

		methodPath := basePath

		if routePath != nil {
			methodPath = path.Join(basePath, StringLiteralContent(routePath))
		}

		verb := method.Content()
		if verb == "All" {
			verb = "Any"
		}
		routes = append(routes, gatewayRouteDefinition{
			Route: types.Route{
				Path:          methodPath,
				ExecUnitName:  unitName,
				Verb:          types.Verb(verb),
				HandledInFile: f.Path(),
			},
			DefinedInPath: f.Path(),
		})

	}
	return routes
}

type nestModuleResult struct {
	controllers []nestController
	f           *types.SourceFile
}

func (h *NestJsHandler) findModules(controllers map[string]nestController) map[string]*nestModuleResult {
	modules := make(map[string]*nestModuleResult)
	for _, ref := range h.output.modules {
		f := ref.File
		result := ref.QueryResult

		varName, pairKey, controllerName, controllerImport := result["name"], result["pairKey"], result["controllerName"], result["controllerImport"]
		moduleName := varName.Content()

		var moduleControllers []nestController
		controllersImport := controllerImport.Content()
		controllersName := controllerName.Content()
		key := pairKey.Content()
		if key == "controllers" {
			controller, ok := controllers[controllersName]
			if !ok {
				continue
			}

			relPath, err := filepath.Rel(filepath.Dir(f.Path()), controller.f.Path())
			if err != nil {
				continue
			}
			if controllerImports := FindImportsInFile(f).Filter(filter.NewSimpleFilter(
				IsRelativeImportOfModule(relPath),
				predicate.Not(IsImportOfType(ImportTypeSideEffect)),
				IsImportInScope(ImportScopeModule),
				ImportedAs(controllersImport))); len(controllerImports) != 1 {
				continue
			}

			moduleControllers = append(moduleControllers, controller)
		}

		if found, ok := modules[moduleName]; ok {
			found.controllers = append(found.controllers, moduleControllers...)
		} else {
			modules[moduleName] = &nestModuleResult{
				controllers: moduleControllers,
				f:           f,
			}
		}
	}
	return modules
}

// Validation functions

func validateController(match map[string]*sitter.Node, f *types.SourceFile) bool {
	importName, method := match["import"], match["method"]
	imp := FindImportForVar(f.Tree().RootNode(), importName.Content())
	return imp.Source == "@nestjs/common" && method.Content() == "Controller"
}

func ValidateModule(match map[string]*sitter.Node, f *types.SourceFile) bool {
	importName, method := match["import"], match["method"]
	imp := FindImportForVar(f.Tree().RootNode(), importName.Content())
	return imp.Source == "@nestjs/common" && method.Content() == "Module"
}

func validateRoute(match map[string]*sitter.Node, f *types.SourceFile) bool {
	importName := match["import"]
	imp := FindImportForVar(f.Tree().RootNode(), importName.Content())
	return imp.Source == "@nestjs/common"
}

func validateNestFactory(match map[string]*sitter.Node, f *types.SourceFile) bool {
	importName, call := match["import"], match["call"]
	importedName := importName.Content()
	imp := FindImportForVar(f.Tree().RootNode(), importName.Content())
	return imp.Source == "@nestjs/core" && call.Content() == importedName+".NestFactory.create"
}
