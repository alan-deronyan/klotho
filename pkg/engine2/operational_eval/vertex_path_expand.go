package operational_eval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dominikbraun/graph"
	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/engine2/constraints"
	"github.com/klothoplatform/klotho/pkg/engine2/operational_rule"
	"github.com/klothoplatform/klotho/pkg/engine2/path_selection"
	"github.com/klothoplatform/klotho/pkg/engine2/solution_context"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
	"github.com/klothoplatform/klotho/pkg/set"
	"go.uber.org/zap"
)

//go:generate mockgen -source=./vertex_path_expand.go --destination=../operational_eval/vertex_path_expand_mock_test.go --package=operational_eval

type (
	pathExpandVertex struct {
		Edge          construct.SimpleEdge
		TempGraph     construct.Graph
		Satisfication knowledgebase.EdgePathSatisfaction
	}

	expansionRunner interface {
		getExpansionsToRun(v *pathExpandVertex) ([]path_selection.ExpansionInput, error)
		handleResultProperties(v *pathExpandVertex, result path_selection.ExpansionResult) error
		addSubExpansion(result path_selection.ExpansionResult, expansion path_selection.ExpansionInput, v *pathExpandVertex) error
		addResourcesAndEdges(result path_selection.ExpansionResult, expansion path_selection.ExpansionInput, v *pathExpandVertex) error
		consumeExpansionProperties(expansion path_selection.ExpansionInput) error
	}

	pathExpandVertexRunner struct {
		Eval *Evaluator
	}
)

func (v *pathExpandVertex) Key() Key {
	return Key{PathSatisfication: v.Satisfication, Edge: v.Edge}
}

func (v *pathExpandVertex) Evaluate(eval *Evaluator) error {
	runner := &pathExpandVertexRunner{Eval: eval}
	edgeExpander := &path_selection.EdgeExpand{Ctx: eval.Solution}
	return v.runEvaluation(eval, runner, edgeExpander)
}

func (v *pathExpandVertex) runEvaluation(eval *Evaluator, runner expansionRunner, edgeExpander path_selection.EdgeExpander) error {
	var errs error
	expansions, err := runner.getExpansionsToRun(v)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("could not get expansions to run: %w", err))
	}
	log := eval.Log()
	if len(expansions) > 1 && log.Desugar().Core().Enabled(zap.DebugLevel) {
		log.Debugf("Expansion %s subexpansions:", v.Edge)
		for _, expansion := range expansions {
			log.Debugf(" %s -> %s", expansion.Dep.Source.ID, expansion.Dep.Target.ID)
		}
	}

	createExpansionErr := func(err error) error {
		return fmt.Errorf("could not run expansion %s -> %s <%s>: %w",
			v.Edge.Source, v.Edge.Target, v.Satisfication.Classification, err,
		)
	}

	for _, expansion := range expansions {
		result, err := edgeExpander.ExpandEdge(expansion)
		if err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}

		resultStr, err := expansionResultString(result.Graph, expansion.Dep)
		if err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}
		if v.Satisfication.Classification != "" {
			eval.Log().Infof("Satisfied %s for %s through %s", v.Satisfication.Classification, v.Edge, resultStr)
		} else {
			eval.Log().Infof("Satisfied %s -> %s through %s", v.Edge.Source, v.Edge.Target, resultStr)
		}

		if err := runner.addResourcesAndEdges(result, expansion, v); err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}

		if err := runner.addSubExpansion(result, expansion, v); err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}

		if err := runner.consumeExpansionProperties(expansion); err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}

		// do this after weve added all resources and edges to the sol ctx so that we replace the ids properly
		if err := runner.handleResultProperties(v, result); err != nil {
			errs = errors.Join(errs, createExpansionErr(err))
			continue
		}
	}
	return errs
}

func (v *pathExpandVertex) UpdateFrom(other Vertex) {
	otherVertex := other.(*pathExpandVertex)
	v.TempGraph = otherVertex.TempGraph
}

// addDepsFromProps checks to see if any properties in `res` match any of the `dependencies`.
// If they do, add a dependency to that property - it may set up a resource that we could reuse,
// depending on the path chosen. This is a conservative dependency, since we don't know which path
// will be chosen.
func (v *pathExpandVertex) addDepsFromProps(
	eval *Evaluator,
	changes graphChanges,
	res construct.ResourceId,
	dependencies []construct.ResourceId,
) error {
	tmpl, err := eval.Solution.KnowledgeBase().GetResourceTemplate(res)
	if err != nil {
		return err
	}
	var errs error
	for k, prop := range tmpl.Properties {
		details := prop.Details()
		if details.OperationalRule == nil {
			// If the property can't create resources, skip it.
			continue
		}
		ready, err := operational_rule.EvaluateIfCondition(details.OperationalRule.If,
			eval.Solution, knowledgebase.DynamicValueData{Resource: res})
		if err != nil || !ready {
			continue
		}

		ref := construct.PropertyRef{Resource: res, Property: k}
		for _, dep := range dependencies {
			if dep == v.Edge.Source || dep == v.Edge.Target {
				continue
			}
			resource, err := eval.Solution.RawView().Vertex(res)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			// if this dependency could pass validation for the resources property, consider it as a dependent vertex
			if err := prop.Validate(resource, dep, solution_context.DynamicCtx(eval.Solution)); err == nil {
				changes.addEdge(v.Key(), Key{Ref: ref})
			}
		}
	}
	return errs
}

// addDepsFromEdge checks to see if the edge's template sets any properties via configuration rules.
// If it does, go through all the existing resources and add an incoming dependency to any that match
// the resource and property from that configuration rule.
func (v *pathExpandVertex) addDepsFromEdge(
	eval *Evaluator,
	changes graphChanges,
	edge construct.Edge,
) error {
	kb := eval.Solution.KnowledgeBase()
	tmpl := kb.GetEdgeTemplate(edge.Source, edge.Target)
	if tmpl == nil {
		return nil
	}

	allRes, err := construct.TopologicalSort(eval.Solution.RawView())
	if err != nil {
		return err
	}

	se := construct.SimpleEdge{Source: edge.Source, Target: edge.Target}
	se.Source.Name = ""
	se.Target.Name = ""

	addDepsMatching := func(ref construct.PropertyRef) error {
		for _, res := range allRes {
			if !ref.Resource.Matches(res) {
				continue
			}
			tmpl, err := kb.GetResourceTemplate(res)
			if err != nil {
				return err
			}
			// TODO: Go into nested properties to determine dependencies
			if _, hasProp := tmpl.Properties[ref.Property]; hasProp {
				actualRef := construct.PropertyRef{
					Resource: res,
					Property: ref.Property,
				}
				changes.addEdge(Key{Ref: actualRef}, v.Key())

				eval.Log().Debugf(
					"Adding speculative dependency %s -> %s (matches %s from %s)",
					actualRef, v.Key(), ref, se,
				)
			}
		}
		return nil
	}

	dyn := solution_context.DynamicCtx(eval.Solution)

	var errs error
	for i, rule := range tmpl.OperationalRules {
		for j, cfg := range rule.ConfigurationRules {
			var err error
			data := knowledgebase.DynamicValueData{Edge: &edge}
			data.Resource, err = knowledgebase.ExecuteDecodeAsResourceId(dyn, cfg.Resource, data)

			// We ignore the error because it just means that we cant resolve the resource yet
			// therefore we cant add a dependency on this invocation
			if err != nil || data.Resource.IsZero() {
				continue
			}

			// NOTE(gg): does this need to consider `Fields`?
			field := cfg.Config.Field
			err = dyn.ExecuteDecode(field, data, &field)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("could not decode field for rule %d cfg %d: %w", i, j, err))
				continue
			}
			if field == "" {
				continue
			}

			ref := construct.PropertyRef{Resource: data.Resource, Property: field}
			errs = errors.Join(errs, addDepsMatching(ref))
		}
	}
	return errs
}

// getDepsForPropertyRef takes a property reference and recurses down until the property is not filled in on the resource
// When we reach resources with missing property references, we know they are the property vertex keys we must depend on
func getDepsForPropertyRef(
	sol solution_context.SolutionContext,
	res construct.ResourceId,
	propertyRef string,
) set.Set[Key] {
	if propertyRef == "" {
		return nil
	}
	keys := make(set.Set[Key])
	cfgCtx := solution_context.DynamicCtx(sol)
	currResources := []construct.ResourceId{res}
	parts := strings.Split(propertyRef, "#")
	for _, part := range parts {
		var nextResources []construct.ResourceId
		for _, currResource := range currResources {
			keys.Add(Key{Ref: construct.PropertyRef{Resource: currResource, Property: part}})
			val, err := cfgCtx.FieldValue(part, currResource)
			if err != nil {
				// The field hasn't resolved yet. Skip it for now, future calls to dependencies will pick it up.
				continue
			}
			if id, ok := val.(construct.ResourceId); ok {
				nextResources = append(nextResources, id)
			} else if ids, ok := val.([]construct.ResourceId); ok {
				nextResources = append(nextResources, ids...)
			}
		}
		currResources = nextResources
	}
	return keys
}

func (v *pathExpandVertex) Dependencies(eval *Evaluator) (graphChanges, error) {
	changes := newChanges()
	srcKey := v.Key()

	changes.addEdges(srcKey, getDepsForPropertyRef(eval.Solution, v.Edge.Source, v.Satisfication.Source.PropertyReference))
	changes.addEdges(srcKey, getDepsForPropertyRef(eval.Solution, v.Edge.Target, v.Satisfication.Target.PropertyReference))

	// if we have a temp graph we can analyze the paths in it for possible dependencies on property vertices
	// if we dont, we should return what we currently have
	// This has to be run after we analyze the refs used in path expansion to make sure the operational rules
	// dont create other resources that need to be operated on in the path expand vertex
	if v.TempGraph == nil {
		return changes, nil
	}

	var errs error
	srcDeps, err := construct.AllDownstreamDependencies(v.TempGraph, v.Edge.Source)
	if err != nil {
		return changes, err
	}
	errs = errors.Join(errs, v.addDepsFromProps(eval, changes, v.Edge.Source, srcDeps))

	targetDeps, err := construct.AllUpstreamDependencies(v.TempGraph, v.Edge.Target)
	if err != nil {
		return changes, err
	}
	errs = errors.Join(errs, v.addDepsFromProps(eval, changes, v.Edge.Target, targetDeps))
	if errs != nil {
		return changes, errs
	}

	edges, err := v.TempGraph.Edges()
	if err != nil {
		return changes, err
	}
	for _, edge := range edges {
		errs = errors.Join(errs, v.addDepsFromEdge(eval, changes, edge))
	}

	return changes, errs
}

func (runner *pathExpandVertexRunner) getExpansionsToRun(v *pathExpandVertex) ([]path_selection.ExpansionInput, error) {
	eval := runner.Eval
	var errs error
	sourceRes, err := eval.Solution.RawView().Vertex(v.Edge.Source)
	if err != nil {
		return nil, fmt.Errorf("could not find source resource %s: %w", v.Edge.Source, err)
	}
	targetRes, err := eval.Solution.RawView().Vertex(v.Edge.Target)
	if err != nil {
		return nil, fmt.Errorf("could not find target resource %s: %w", v.Edge.Target, err)
	}
	edge := construct.ResourceEdge{Source: sourceRes, Target: targetRes}
	expansions, err := path_selection.DeterminePathSatisfactionInputs(eval.Solution, v.Satisfication, edge)
	if err != nil {
		errs = errors.Join(errs, err)
	}

	result := make([]path_selection.ExpansionInput, len(expansions))
	for i, expansion := range expansions {
		input := path_selection.ExpansionInput{
			Dep:            expansion.Dep,
			Classification: expansion.Classification,
			TempGraph:      v.TempGraph,
		}
		if expansion.Dep.Source != edge.Source || expansion.Dep.Target != edge.Target {
			simple := construct.SimpleEdge{Source: expansion.Dep.Source.ID, Target: expansion.Dep.Target.ID}
			tempGraph, err := path_selection.BuildPathSelectionGraph(simple, eval.Solution.KnowledgeBase(), expansion.Classification)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("error getting expansions to run. could not build path selection graph: %w", err))
				continue
			}
			input.TempGraph = tempGraph
		}
		result[i] = input
	}
	return result, errs
}

func (runner *pathExpandVertexRunner) addResourcesAndEdges(
	result path_selection.ExpansionResult,
	expansion path_selection.ExpansionInput,
	v *pathExpandVertex,
) error {
	eval := runner.Eval
	adj, err := result.Graph.AdjacencyMap()
	if err != nil {
		return err
	}
	if len(adj) > 2 {
		_, err := eval.Solution.OperationalView().Edge(v.Edge.Source, v.Edge.Target)
		if err == nil {
			if err := eval.Solution.OperationalView().RemoveEdge(v.Edge.Source, v.Edge.Target); err != nil {
				return err
			}
		} else if !errors.Is(err, graph.ErrEdgeNotFound) {
			return err
		}
	} else if len(adj) == 2 {
		err = eval.Solution.RawView().AddEdge(expansion.Dep.Source.ID, expansion.Dep.Target.ID)
		if err != nil {
			return err
		}
		return eval.Solution.OperationalView().MakeEdgesOperational([]construct.Edge{
			{Source: expansion.Dep.Source.ID, Target: expansion.Dep.Target.ID},
		})
	}

	// Once the path is selected & expanded, first add all the resources to the graph
	var errs error
	resources := []*construct.Resource{}
	for pathId := range adj {
		res, err := eval.Solution.OperationalView().Vertex(pathId)
		switch {
		case errors.Is(err, graph.ErrVertexNotFound):
			res, err = result.Graph.Vertex(pathId)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			// add the resource to the raw view because we want to wait until after the edges are added to make it operational
			errs = errors.Join(errs, eval.Solution.OperationalView().AddVertex(res))

		case err != nil:
			errs = errors.Join(errs, err)
		}
		resources = append(resources, res)
	}
	if errs != nil {
		return errs
	}

	// After all the resources, then add all the dependencies
	edges := []construct.Edge{}
	for _, edgeMap := range adj {
		for _, edge := range edgeMap {
			err := eval.Solution.OperationalView().AddEdge(edge.Source, edge.Target)
			if err != nil {
				errs = errors.Join(errs, err)
			}
			edges = append(edges, edge)
		}
	}
	if errs != nil {
		return errs
	}
	if err := eval.AddResources(resources...); err != nil {
		return err
	}
	return eval.AddEdges(edges...)
}

func (runner *pathExpandVertexRunner) addSubExpansion(
	result path_selection.ExpansionResult,
	expansion path_selection.ExpansionInput,
	v *pathExpandVertex,
) error {
	// add sub expansions returned from the result, only for the classification of this expansion
	eval := runner.Eval
	changes := newChanges()
	for _, subExpand := range result.Edges {
		pathSatisfications, err := eval.Solution.KnowledgeBase().GetPathSatisfactionsFromEdge(subExpand.Source, subExpand.Target)
		if err != nil {
			return fmt.Errorf("could not get path satisfications for sub expansion %s -> %s: %w",
				subExpand.Source, subExpand.Target, err)
		}

		for _, satisfication := range pathSatisfications {
			if satisfication.Classification == v.Satisfication.Classification {
				// we cannot evaluate these vertices immediately because we are unsure if their dependencies have settled
				changes.addNode(&pathExpandVertex{
					Edge:          construct.SimpleEdge{Source: subExpand.Source, Target: subExpand.Target},
					TempGraph:     expansion.TempGraph,
					Satisfication: satisfication,
				})
			}
		}
	}
	return eval.enqueue(changes)
}

func (runner *pathExpandVertexRunner) consumeExpansionProperties(expansion path_selection.ExpansionInput) error {
	delays, err := knowledgebase.ConsumeFromResource(
		expansion.Dep.Source,
		expansion.Dep.Target,
		solution_context.DynamicCtx(runner.Eval.Solution),
	)
	if err != nil {
		return err
	}
	// we add constrains for the delayed consumption here since their property has not yet been evaluated
	c := runner.Eval.Solution.Constraints()
	for _, delay := range delays {
		c.Resources = append(c.Resources, constraints.ResourceConstraint{
			Operator: constraints.AddConstraintOperator,
			Target:   delay.Resource,
			Property: delay.PropertyPath,
			Value:    delay.Value,
		})
	}
	return nil
}

// handleProperties
func (runner *pathExpandVertexRunner) handleResultProperties(
	v *pathExpandVertex,
	result path_selection.ExpansionResult,
) error {
	eval := runner.Eval
	adj, err := result.Graph.AdjacencyMap()
	if err != nil {
		return err
	}
	pred, err := result.Graph.PredecessorMap()
	if err != nil {
		return err
	}

	handleResultProperties := func(
		res *construct.Resource,
		rt *knowledgebase.ResourceTemplate,
		resources map[construct.ResourceId]graph.Edge[construct.ResourceId],
		Direction knowledgebase.Direction,
	) error {
		var errs error
		for target := range resources {
			targetRes, err := result.Graph.Vertex(target)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			errs = errors.Join(errs, rt.LoopProperties(res, func(prop knowledgebase.Property) error {
				opRuleCtx := operational_rule.OperationalRuleContext{
					Solution: eval.Solution,
					Property: prop,
					Data:     knowledgebase.DynamicValueData{Resource: res.ID},
				}
				details := prop.Details()
				if details.OperationalRule == nil || len(details.OperationalRule.Step.Resources) == 0 {
					return nil
				}
				step := details.OperationalRule.Step
				for _, selector := range step.Resources {
					if step.Direction == Direction {
						canUse, err := selector.CanUse(
							solution_context.DynamicCtx(eval.Solution),
							knowledgebase.DynamicValueData{Resource: res.ID},
							targetRes,
						)
						if canUse && err == nil {
							err = opRuleCtx.SetField(res, targetRes, step)
							if err != nil {
								errs = errors.Join(errs, err)
							}
						}
					}
				}
				return nil
			}))
		}
		return errs
	}

	var errs error
	for id, downstreams := range adj {
		oldId := id
		rt, err := eval.Solution.KnowledgeBase().GetResourceTemplate(id)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		res, err := eval.Solution.RawView().Vertex(id)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		errs = errors.Join(errs, handleResultProperties(res, rt, downstreams, knowledgebase.DirectionDownstream))
		errs = errors.Join(errs, handleResultProperties(res, rt, pred[id], knowledgebase.DirectionUpstream))

		if oldId != res.ID {
			errs = errors.Join(errs, eval.UpdateId(oldId, res.ID))
		}
	}
	return errs
}

func expansionResultString(result construct.Graph, dep construct.ResourceEdge) (string, error) {
	sb := new(strings.Builder)
	handled := make(set.Set[construct.SimpleEdge])

	path, err := graph.ShortestPathStable(result, dep.Source.ID, dep.Target.ID, construct.ResourceIdLess)
	if err != nil {
		return "", fmt.Errorf("expansion result does not contain path from %s to %s: %w", dep.Source.ID, dep.Target.ID, err)
	}
	for i, res := range path {
		if i == 0 {
			sb.WriteString(res.String())
			continue
		}
		fmt.Fprintf(sb, " -> %s", res)
		handled.Add(construct.SimpleEdge{Source: path[i-1], Target: res})
	}

	edges, err := result.Edges()
	if err != nil {
		return sb.String(), err
	}

	for _, e := range edges {
		se := construct.SimpleEdge{Source: e.Source, Target: e.Target}
		if handled.Contains(se) {
			continue
		}
		fmt.Fprintf(sb, ", %s", se.String())
	}

	return sb.String(), nil
}
