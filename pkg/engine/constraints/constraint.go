package constraints

import (
	"errors"
	"fmt"
	"os"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/engine/classification"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base"
	"github.com/klothoplatform/klotho/pkg/yaml_util"
	"gopkg.in/yaml.v3"
)

type (

	// Constraint is an interface detailing different intents that can be applied to a resource graph
	Constraint interface {
		// Scope returns where on the resource graph the constraint is applied
		Scope() ConstraintScope
		// IsSatisfied returns whether or not the constraint is satisfied based on the resource graph
		// For a resource graph to be valid all constraints must be satisfied
		IsSatisfied(dag *construct.ResourceGraph, kb knowledgebase.EdgeKB, mappedConstructResources map[construct.ResourceId][]construct.Resource, classifier classification.Classifier) bool
		// Validate returns whether or not the constraint is valid
		Validate() error

		String() string
	}

	// BaseConstraint is the base struct for all constraints
	// BaseConstraint is used in our parsing to determine the Scope of the constraint and what go struct it corresponds to
	BaseConstraint struct {
		Scope ConstraintScope `yaml:"scope"`
	}

	// Edge is a struct that represents how we take in data about an edge in the resource graph
	Edge struct {
		Source construct.ResourceId `yaml:"source"`
		Target construct.ResourceId `yaml:"target"`
	}

	// ConstraintScope is an enum that represents the different scopes that a constraint can be applied to
	ConstraintScope string
	// ConstraintOperator is an enum that represents the different operators that can be applied to a constraint
	ConstraintOperator string

	Constraints []Constraint
)

const (
	ApplicationConstraintScope ConstraintScope = "application"
	ConstructConstraintScope   ConstraintScope = "construct"
	EdgeConstraintScope        ConstraintScope = "edge"
	ResourceConstraintScope    ConstraintScope = "resource"

	MustExistConstraintOperator      ConstraintOperator = "must_exist"
	MustNotExistConstraintOperator   ConstraintOperator = "must_not_exist"
	MustContainConstraintOperator    ConstraintOperator = "must_contain"
	MustNotContainConstraintOperator ConstraintOperator = "must_not_contain"
	AddConstraintOperator            ConstraintOperator = "add"
	RemoveConstraintOperator         ConstraintOperator = "remove"
	ReplaceConstraintOperator        ConstraintOperator = "replace"
	EqualsConstraintOperator         ConstraintOperator = "equals"
)

func (cs Constraints) MarshalYAML() (interface{}, error) {
	var list []yaml.Node
	for _, c := range cs {
		var n yaml.Node
		err := n.Encode(c)
		if err != nil {
			return nil, err
		}
		scope := []*yaml.Node{
			{
				Kind:  yaml.ScalarNode,
				Value: "scope",
			},
			{
				Kind:  yaml.ScalarNode,
				Value: string(c.Scope()),
			},
		}
		n.Content = append(scope, n.Content...)
		list = append(list, n)
	}
	return list, nil
}

func (cs *Constraints) UnmarshalYAML(node *yaml.Node) error {
	var list []yaml_util.RawNode
	err := node.Decode(&list)
	if err != nil {
		return err
	}

	*cs = make(Constraints, len(list))
	var errs error
	for i, raw := range list {
		var base BaseConstraint
		err = raw.Decode(&base)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		var c Constraint
		switch base.Scope {

		case ApplicationConstraintScope:
			var constraint ApplicationConstraint
			err = raw.Decode(&constraint)
			c = &constraint

		case ConstructConstraintScope:
			var constraint ConstructConstraint
			err = raw.Decode(&constraint)
			c = &constraint

		case EdgeConstraintScope:
			var constraint EdgeConstraint
			err = raw.Decode(&constraint)
			c = &constraint

		case ResourceConstraintScope:
			var constraint ResourceConstraint
			err = raw.Decode(&constraint)
			c = &constraint

		default:
			err = fmt.Errorf("invalid scope %s", base.Scope)
		}
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if err := c.Validate(); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		(*cs)[i] = c
	}
	return errs
}

func LoadConstraintsFromFile(path string) (map[ConstraintScope][]Constraint, error) {
	var input struct {
		Constraints Constraints `yaml:"constraints"`
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // nolint:errcheck

	err = yaml.NewDecoder(f).Decode(&input)
	if err != nil {
		return nil, err
	}

	constraintsByScope := make(map[ConstraintScope][]Constraint)
	for _, constraint := range input.Constraints {
		constraintsByScope[constraint.Scope()] = append(constraintsByScope[constraint.Scope()], constraint)
	}
	return constraintsByScope, nil
}

// ParseConstraintsFromFile parses a yaml file into a map of constraints
//
// Future spec may include ordering of the application of constraints, but for now we assume that the order of the constraints is based on the yaml file and they cannot be grouped outside of scope
func ParseConstraintsFromFile(bytes []byte) (map[ConstraintScope][]Constraint, error) {
	var constraints Constraints
	err := yaml.Unmarshal(bytes, &constraints)
	if err != nil {
		return nil, err
	}
	constraintsByScope := make(map[ConstraintScope][]Constraint)
	for _, constraint := range constraints {
		scope := constraint.Scope()
		constraintsByScope[scope] = append(constraintsByScope[scope], constraint)
	}
	return constraintsByScope, nil
}
