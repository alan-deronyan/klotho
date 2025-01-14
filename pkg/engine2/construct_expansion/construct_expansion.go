package constructexpansion

import (
	"errors"
	"fmt"

	"github.com/dominikbraun/graph"
	"github.com/klothoplatform/klotho/pkg/collectionutil"
	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/engine2/constraints"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
	"go.uber.org/zap"
)

type (
	ExpansionSet struct {
		Construct  *construct.Resource
		Attributes []string
	}

	ExpansionSolution struct {
		Edges                  []graph.Edge[construct.Resource]
		DirectlyMappedResource construct.ResourceId
	}

	ConstructExpansionContext struct {
		Construct *construct.Resource
		Kb        knowledgebase.TemplateKB
	}
)

// ExpandConstructs expands all constructs in the working state using the engines provider
//
// The resources that result from the expanded constructs are written to the engines resource graph
// All dependencies are copied over to the resource graph
// If a dependency in the working state included a construct, the engine copies the dependency to all directly linked resources
func (ctx *ConstructExpansionContext) ExpandConstruct(res *construct.Resource, constraints []constraints.ConstructConstraint) ([]ExpansionSolution, error) {
	if res.ID.IsAbstractResource() {
		return nil, fmt.Errorf("unable to expand construct %s, resource is not an abstract construct", res.ID)
	}
	zap.S().Debugf("Expanding construct %s", res.ID)
	constructType := ""
	attributes := make(map[string]any)
	for _, constructConstraint := range constraints {
		if constructConstraint.Target == res.ID {
			constructType = constructConstraint.Type
			if constructType != "" && constructType != constructConstraint.Type {
				return nil, fmt.Errorf("unable to expand construct %s, conflicting types in constraints", res.ID)
			}
			for k, v := range constructConstraint.Attributes {
				if val, ok := attributes[k]; ok {
					if v != val {
						return nil, fmt.Errorf("unable to expand construct %s, attribute %s has conflicting values", res.ID, k)
					}
				}
				attributes[k] = v
			}
		}
	}
	expansionSet := ExpansionSet{Construct: res}
	for attribute := range attributes {
		expansionSet.Attributes = append(expansionSet.Attributes, attribute)
	}
	return ctx.findPossibleExpansions(expansionSet, constructType)
}

func (ctx *ConstructExpansionContext) findPossibleExpansions(expansionSet ExpansionSet, constructQualifiedType string) ([]ExpansionSolution, error) {
	var possibleExpansions []ExpansionSolution
	var joinedErr error
	functionality := knowledgebase.GetFunctionality(ctx.Kb, expansionSet.Construct.ID)
	for _, res := range ctx.Kb.ListResources() {
		if constructQualifiedType != "" && res.Id().QualifiedTypeName() != constructQualifiedType {
			continue
		}
		classifications := res.Classification
		if !collectionutil.Contains(classifications.Is, string(functionality)) {
			continue
		}
		unsatisfiedAttributes := []string{}
		for _, ms := range expansionSet.Attributes {
			if !collectionutil.Contains(classifications.Is, ms) {
				unsatisfiedAttributes = append(unsatisfiedAttributes, ms)
			}
		}
		baseRes, err := knowledgebase.CreateResource(ctx.Kb, construct.ResourceId{
			Provider: res.Id().Provider,
			Type:     res.Id().Type,
			Name:     expansionSet.Construct.ID.Name,
		})
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
		expansions, err := ctx.findExpansions(unsatisfiedAttributes, []graph.Edge[construct.Resource](nil), *baseRes, functionality)
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
		for _, expansion := range expansions {
			possibleExpansions = append(possibleExpansions, ExpansionSolution{Edges: expansion, DirectlyMappedResource: baseRes.ID})
		}
	}
	if len(possibleExpansions) == 0 {
		return nil, fmt.Errorf("no expansions found for attributes %v", expansionSet.Attributes)
	}
	return possibleExpansions, nil
}

// findExpansions finds all possible expansions for a given construct and a set of attributes
// It returns a list of all possible expansions by recursing down and calling itself until
func (ctx *ConstructExpansionContext) findExpansions(attributes []string, edges []graph.Edge[construct.Resource], baseResource construct.Resource, functionality knowledgebase.Functionality) ([][]graph.Edge[construct.Resource], error) {
	if len(attributes) == 0 {
		return [][]graph.Edge[construct.Resource]{edges}, nil
	}
	var result [][]graph.Edge[construct.Resource]
	for _, attribute := range attributes {
		for _, res := range ctx.Kb.ListResources() {
			if res.Id().QualifiedTypeName() == baseResource.ID.QualifiedTypeName() {
				continue
			}
			if ctx.Kb.HasFunctionalPath(baseResource.ID, res.Id()) {
				if res.GivesAttributeForFunctionality(attribute, functionality) {
					resource := construct.Resource{
						ID:         construct.ResourceId{Type: res.Id().Type, Name: baseResource.ID.Name, Provider: res.Id().Provider},
						Properties: make(construct.Properties),
					}
					edges = append(edges, graph.Edge[construct.Resource]{Source: baseResource, Target: resource})
					unsatisfiedAttributes := []string{}
					for _, ms := range attributes {
						if ms != attribute {
							unsatisfiedAttributes = append(unsatisfiedAttributes, ms)
						}
					}

					expansions, err := ctx.findExpansions(unsatisfiedAttributes, edges, baseResource, functionality)
					if err != nil {
						return nil, err
					}
					result = append(result, expansions...)
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no expansions found for attributes %v", attributes)
	}
	return result, nil
}
