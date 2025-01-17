package engine2

import (
	"errors"

	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/engine2/constraints"
	"github.com/klothoplatform/klotho/pkg/engine2/solution_context"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
)

type (
	// Engine is a struct that represents the object which processes the resource graph and applies constraints
	Engine struct {
		Kb knowledgebase.TemplateKB
	}

	// EngineContext is a struct that represents the context of the engine
	// The context is used to store the state of the engine
	EngineContext struct {
		Constraints  constraints.Constraints
		InitialState construct.Graph
		Solutions    []solution_context.SolutionContext
	}
)

func NewEngine(kb knowledgebase.TemplateKB) *Engine {
	return &Engine{
		Kb: kb,
	}
}

func (e *Engine) Run(context *EngineContext) error {
	solutionCtx := NewSolutionContext(e.Kb)
	solutionCtx.constraints = &context.Constraints
	err := solutionCtx.LoadGraph(context.InitialState)
	if err != nil {
		return err
	}
	err = ApplyConstraints(solutionCtx)
	if err != nil {
		return err
	}
	err = solutionCtx.Solve()
	context.Solutions = append(context.Solutions, solutionCtx)
	return err
}

func (e *Engine) getPropertyValidation(ctx solution_context.SolutionContext) ([]solution_context.PropertyValidationDecision, error) {
	decisions := ctx.GetDecisions().GetRecords()
	validationDecisions := make([]solution_context.PropertyValidationDecision, 0)
	for _, decision := range decisions {
		if validation, ok := decision.(solution_context.PropertyValidationDecision); ok {
			if validation.Error != nil {
				validationDecisions = append(validationDecisions, validation)
			}
		}
	}
	var errs error
	for _, decision := range validationDecisions {
		for _, c := range ctx.Constraints().Resources {
			if c.Target == decision.Resource && c.Property == decision.Property.Details().Path {
				errs = errors.Join(errs, decision.Error)
			}
		}
	}
	return validationDecisions, errs
}
