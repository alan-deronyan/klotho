package operational_eval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dominikbraun/graph"
	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/engine2/constraints"
	"github.com/klothoplatform/klotho/pkg/engine2/operational_rule"
	"github.com/klothoplatform/klotho/pkg/engine2/solution_context"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
	"github.com/klothoplatform/klotho/pkg/set"
)

type (
	propertyVertex struct {
		Ref construct.PropertyRef

		Template      knowledgebase.Property
		EdgeRules     map[construct.SimpleEdge][]knowledgebase.OperationalRule
		ResourceRules map[string][]knowledgebase.OperationalRule
	}
)

func (prop propertyVertex) Key() Key {
	return Key{Ref: prop.Ref}
}

func (prop *propertyVertex) Dependencies(eval *Evaluator, propCtx dependencyCapturer) error {

	res, err := eval.Solution.RawView().Vertex(prop.Ref.Resource)
	if err != nil {
		return fmt.Errorf("could not get resource for property vertex dependency calculation %s: %w", prop.Ref, err)
	}
	path, err := res.PropertyPath(prop.Ref.Property)

	if err != nil {
		return fmt.Errorf("could not get property path for %s: %w", prop.Ref, err)
	}

	resData := knowledgebase.DynamicValueData{Resource: prop.Ref.Resource, Path: path}

	// Template can be nil when checking for dependencies from a propertyVertex when adding an edge template
	if prop.Template != nil {
		_, _ = prop.Template.GetDefaultValue(propCtx, resData)
		details := prop.Template.Details()
		if opRule := details.OperationalRule; opRule != nil {
			if err := propCtx.ExecutePropertyRule(resData, *opRule); err != nil {
				return fmt.Errorf("could not execute resource operational rule for %s: %w", prop.Ref, err)
			}
		}
	}

	for edge, rule := range prop.EdgeRules {
		edgeData := knowledgebase.DynamicValueData{
			Resource: prop.Ref.Resource,
			Edge:     &construct.Edge{Source: edge.Source, Target: edge.Target},
		}
		for _, opRule := range rule {
			if err := propCtx.ExecuteOpRule(edgeData, opRule); err != nil {
				return fmt.Errorf("could not execute edge operational rule for %s: %w", prop.Ref, err)
			}
		}
	}

	for _, rule := range prop.ResourceRules {
		for _, opRule := range rule {
			if err := propCtx.ExecuteOpRule(resData, opRule); err != nil {
				return fmt.Errorf("could not execute resource operational rule for %s: %w", prop.Ref, err)
			}
		}
	}

	return nil
}

func (prop *propertyVertex) UpdateFrom(otherV Vertex) {
	if prop == otherV {
		return
	}
	other, ok := otherV.(*propertyVertex)
	if !ok {
		panic(fmt.Sprintf("cannot merge property with non-property vertex: %T", otherV))
	}
	if prop.Ref != other.Ref {
		panic(fmt.Sprintf("cannot merge properties with different refs: %s != %s", prop.Ref, other.Ref))
	}

	if prop.Template == nil {
		prop.Template = other.Template
	}
	if prop.EdgeRules == nil {
		prop.EdgeRules = make(map[construct.SimpleEdge][]knowledgebase.OperationalRule)
	}

	for edge, rules := range other.EdgeRules {
		if _, ok := prop.EdgeRules[edge]; ok {
			// already have rules for this edge, don't duplicate them
			continue
		}
		prop.EdgeRules[edge] = rules
	}
}

func (v *propertyVertex) Evaluate(eval *Evaluator) error {
	sol := eval.Solution.With("resource", v.Ref.Resource).With("property", v.Ref.Property)
	res, err := sol.RawView().Vertex(v.Ref.Resource)
	if err != nil {
		return fmt.Errorf("could not get resource to evaluate %s: %w", v.Ref, err)
	}
	path, err := res.PropertyPath(v.Ref.Property)

	if err != nil {
		return fmt.Errorf("could not get property path for %s: %w", v.Ref, err)
	}

	dynData := knowledgebase.DynamicValueData{Resource: res.ID, Path: path}
	if err := v.evaluateConstraints(sol, res, dynData); err != nil {
		return err
	}
	opCtx := operational_rule.OperationalRuleContext{
		Solution: sol,
		Property: v.Template,
		Data:     dynData,
	}

	// we know we cannot change properties of imported resources only users through constraints
	// we still want to be able to update ids in case they are setting the property of a namespaced resource
	// so we just conditionally run the edge operational rules
	//
	// we still need to run the resource operational rules though,
	// to make sure dependencies exist where properties have operational rules set
	if err := v.evaluateResourceOperational(res, &opCtx); err != nil {
		return err
	}

	if err := v.evaluateEdgeOperational(res, &opCtx); err != nil {
		return err
	}

	if err := eval.UpdateId(v.Ref.Resource, res.ID); err != nil {
		return err
	}
	propertyType := v.Template.Type()
	if strings.HasPrefix(propertyType, "list") || strings.HasPrefix(propertyType, "set") || strings.HasPrefix(propertyType, "map") {
		property, err := res.GetProperty(v.Ref.Property)
		if err != nil {
			return fmt.Errorf("could not get property %s on resource %s: %w", v.Ref.Property, v.Ref.Resource, err)
		}
		if property != nil {
			if strings.HasPrefix(propertyType, "set") {
				if set, ok := property.(set.HashedSet[string, any]); ok {
					property = set.ToSlice()
				} else {
					return fmt.Errorf("could not convert property %s on resource %s to set", v.Ref.Property, v.Ref.Resource)
				}
			}

			err = eval.cleanupPropertiesSubVertices(v.Ref, res, property)
			if err != nil {
				return fmt.Errorf("could not cleanup sub vertices for %s: %w", v.Ref, err)
			}
		}
		// If we have modified a list or set we want to re add the resource to be evaluated
		// so the nested fields are ensured to be set if required
		return eval.AddResources(res)
	}

	// Now that the vertex is evaluated, we will check it for validity and record our decision
	val, err := res.GetProperty(v.Ref.Property)
	if err != nil {
		return fmt.Errorf("error while validating resource property: could not get property %s on resource %s: %w", v.Ref.Property, v.Ref.Resource, err)
	}
	err = v.Template.Validate(res, val, solution_context.DynamicCtx(eval.Solution))
	eval.Solution.RecordDecision(solution_context.PropertyValidationDecision{
		Resource: v.Ref.Resource,
		Property: v.Template,
		Value:    val,
		Error:    err,
	})
	return nil
}

func (v *propertyVertex) evaluateConstraints(
	sol solution_context.SolutionContext,
	res *construct.Resource,
	dynData knowledgebase.DynamicValueData,
) error {
	var setConstraint constraints.ResourceConstraint
	var addConstraints []constraints.ResourceConstraint
	for _, c := range sol.Constraints().Resources {
		if c.Target != res.ID || c.Property != v.Ref.Property {
			continue
		}
		if c.Operator == constraints.EqualsConstraintOperator {
			setConstraint = c
			continue
		}
		addConstraints = append(addConstraints, c)
	}
	currentValue, err := res.GetProperty(v.Ref.Property)
	if err != nil {
		return fmt.Errorf("could not get current value for %s: %w", v.Ref, err)
	}

	ctx := solution_context.DynamicCtx(sol)
	var defaultVal any
	if currentValue == nil && !res.Imported {
		defaultVal, err = v.Template.GetDefaultValue(ctx, dynData)
		if err != nil {
			return fmt.Errorf("could not get default value for %s: %w", v.Ref, err)
		}
	}
	if currentValue == nil && setConstraint.Operator == "" && v.Template != nil && defaultVal != nil && !res.Imported {
		err = solution_context.ConfigureResource(
			sol,
			res,
			knowledgebase.Configuration{Field: v.Ref.Property, Value: defaultVal},
			dynData,
			"set",
			false,
		)
		if err != nil {
			return fmt.Errorf("could not set default value for %s: %w", v.Ref, err)
		}

	} else if setConstraint.Operator != "" {
		kb := sol.KnowledgeBase()
		resTemplate, err := kb.GetResourceTemplate(res.ID)
		if err != nil {
			return fmt.Errorf("could not get resource template for %s: %w", res.ID, err)
		}
		property := resTemplate.GetProperty(v.Ref.Property)
		if property == nil {
			return fmt.Errorf("could not get property %s from resource %s", v.Ref.Property, res.ID)
		}
		err = solution_context.ConfigureResource(
			sol,
			res,
			knowledgebase.Configuration{Field: v.Ref.Property, Value: setConstraint.Value},
			dynData,
			"set",
			true,
		)
		if err != nil {
			return fmt.Errorf("could not apply initial constraint for %s: %w", v.Ref, err)
		}
	}
	dynData.Resource = res.ID // Update in case the property changes the ID

	var errs error
	for _, c := range addConstraints {
		if c.Operator == constraints.EqualsConstraintOperator {
			continue
		}
		action, err := solution_context.ConstraintOperatorToAction(c.Operator)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("could not apply constraint for %s: %w", v.Ref, err))
			continue
		}
		errs = errors.Join(errs, solution_context.ConfigureResource(
			sol,
			res,
			knowledgebase.Configuration{Field: v.Ref.Property, Value: c.Value},
			dynData,
			action,
			true,
		))
		dynData.Resource = res.ID
	}
	if errs != nil {
		return fmt.Errorf("could not apply constraints for %s: %w", v.Ref, errs)
	}

	return nil
}

func (v *propertyVertex) evaluateResourceOperational(
	res *construct.Resource,
	opCtx operational_rule.OpRuleHandler,
) error {
	if v.Template == nil || v.Template.Details().OperationalRule == nil {
		return nil
	}

	err := opCtx.HandlePropertyRule(*v.Template.Details().OperationalRule)
	if err != nil {
		return fmt.Errorf("could not apply operational rule for %s: %w", v.Ref, err)
	}

	return nil
}

func (v *propertyVertex) evaluateEdgeOperational(
	res *construct.Resource,
	opCtx operational_rule.OpRuleHandler,
) error {
	oldId := v.Ref.Resource
	var errs error
	for edge, rules := range v.EdgeRules {
		for _, rule := range rules {
			// In case one of the previous rules changed the ID, update it
			edge = UpdateEdgeId(edge, oldId, res.ID)

			opCtx.SetData(knowledgebase.DynamicValueData{
				Resource: res.ID,
				Edge:     &graph.Edge[construct.ResourceId]{Source: edge.Source, Target: edge.Target},
			})

			err := opCtx.HandleOperationalRule(rule)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf(
					"could not apply edge %s -> %s operational rule for %s: %w",
					edge.Source, edge.Target, v.Ref, err,
				))
			}
		}
	}
	return errs
}

func (v *propertyVertex) Ready(eval *Evaluator) (ReadyPriority, error) {
	if v.Template == nil {
		// wait until we have a template
		return NotReadyMax, nil
	}
	if v.Template.Details().OperationalRule != nil {
		// operational rules should run as soon as possible to create any resources they need
		return ReadyNow, nil
	}
	ptype := v.Template.Type()
	if strings.HasPrefix(ptype, "list") || strings.HasPrefix(ptype, "set") {
		// never sure when a list/set is ready - it'll just be appended to by edges through
		// `v.EdgeRules`
		return NotReadyHigh, nil
	}
	if strings.HasPrefix(ptype, "map") && len(v.Template.SubProperties()) == 0 {
		// maps without sub-properties (ie, not objects) are also appended to by edges
		return NotReadyHigh, nil
	}
	// properties that have values set via edge rules dont' have default values
	res, err := eval.Solution.RawView().Vertex(v.Ref.Resource)
	if err != nil {
		return NotReadyHigh, fmt.Errorf("could not get resource for property vertex dependency calculation %s: %w", v.Ref, err)
	}
	path, err := res.PropertyPath(v.Ref.Property)

	if err != nil {
		return NotReadyHigh, fmt.Errorf("could not get property path for %s: %w", v.Ref, err)
	}

	defaultVal, err := v.Template.GetDefaultValue(solution_context.DynamicCtx(eval.Solution),
		knowledgebase.DynamicValueData{Resource: v.Ref.Resource, Path: path})
	if err != nil {
		return NotReadyMid, nil
	}
	if defaultVal != nil {
		return ReadyNow, nil
	}
	// for non-list/set types, once an edge is here to set the value, it can be run
	if len(v.EdgeRules) > 0 {
		return ReadyNow, nil
	}
	return NotReadyMid, nil
}

// addConfigurationRuleToPropertyVertex adds a configuration rule to a property vertex
// if the vertex parameter is a edgeVertex or resourceRuleVertex, it will add the rule to the
// appropriate property vertex and field on the property vertex.
//
// The method returns a map of rules which can be evaluated immediately, and an error if any
func addConfigurationRuleToPropertyVertex(
	rule knowledgebase.OperationalRule,
	v Vertex,
	cfgCtx knowledgebase.DynamicValueContext,
	data knowledgebase.DynamicValueData,
	eval *Evaluator,
) (map[construct.ResourceId][]knowledgebase.ConfigurationRule, error) {
	configuration := make(map[construct.ResourceId][]knowledgebase.ConfigurationRule)

	log := eval.Log().With("op", "eval")
	pred, err := eval.graph.PredecessorMap()
	if err != nil {
		return configuration, err
	}

	var errs error

	for _, config := range rule.ConfigurationRules {

		var ref construct.PropertyRef
		err := cfgCtx.ExecuteDecode(config.Resource, data, &ref.Resource)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf(
				"could not decode resource for %s: %w",
				config.Resource, err,
			))
			continue
		}
		err = cfgCtx.ExecuteDecode(config.Config.Field, data, &ref.Property)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf(
				"could not decode property for %s: %w",
				config.Config.Field, err,
			))
			continue
		}

		key := Key{Ref: ref}
		vertex, err := eval.graph.Vertex(key)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("could not attempt to get existing vertex for %s: %w", ref, err))
			continue
		}
		_, unevalErr := eval.unevaluated.Vertex(key)
		if errors.Is(unevalErr, graph.ErrVertexNotFound) {
			var evalDeps []string
			for dep := range pred[key] {
				depEvaled, err := eval.isEvaluated(dep)
				if err != nil {
					errs = errors.Join(errs, fmt.Errorf("could not check if %s is evaluated: %w", dep, err))
					continue
				}
				if depEvaled {
					evalDeps = append(evalDeps, `"`+dep.String()+`"`)
				}
			}
			if len(evalDeps) == 0 {
				configuration[ref.Resource] = append(configuration[ref.Resource], config)
				log.Debugf("Allowing config on %s to be evaluated due to no dependents", key)
			} else {
				errs = errors.Join(errs, fmt.Errorf(
					"cannot add rules to evaluated node %s: evaluated dependents: %s",
					ref, strings.Join(evalDeps, ", "),
				))
			}
			continue
		} else if err != nil {
			errs = errors.Join(errs, fmt.Errorf("could not get existing unevaluated vertex for %s: %w", ref, err))
			continue
		}
		pv, ok := vertex.(*propertyVertex)
		if !ok {
			errs = errors.Join(errs,
				fmt.Errorf("existing vertex for %s is not a property vertex", ref),
			)
		}

		switch v := v.(type) {
		case *edgeVertex:
			pv.EdgeRules[v.Edge] = append(pv.EdgeRules[v.Edge], knowledgebase.OperationalRule{
				If:                 rule.If,
				ConfigurationRules: []knowledgebase.ConfigurationRule{config},
			})
		case *resourceRuleVertex:
			pv.ResourceRules[v.Rule.Hash()] = append(pv.ResourceRules[v.Rule.Hash()], knowledgebase.OperationalRule{
				If:                 rule.If,
				ConfigurationRules: []knowledgebase.ConfigurationRule{config},
			})
		default:
			errs = errors.Join(errs,
				fmt.Errorf("existing vertex for %s is not able to add configuration rules to property vertex", ref),
			)
		}

	}
	return configuration, errs
}
