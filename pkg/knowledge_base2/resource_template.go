package knowledgebase2

import (
	"strings"

	"github.com/klothoplatform/klotho/pkg/collectionutil"
	"github.com/klothoplatform/klotho/pkg/construct"
)

type (
	// ResourceTemplate defines how rules are handled by the engine in terms of making sure they are functional in the graph
	ResourceTemplate struct {
		QualifiedTypeName string `json:"qualified_type_name" yaml:"qualified_type_name"`

		Properties map[string]Property `json:"properties" yaml:"properties"`

		Classification Classification `json:"classification" yaml:"classification"`

		// DeleteContext defines the context in which a resource can be deleted
		DeleteContext construct.DeleteContext `json:"delete_context" yaml:"delete_context"`
		// Views defines the views that the resource should be added to as a distinct node
		Views map[string]string `json:"views" yaml:"views"`
	}

	Property struct {
		Name string `json:"name" yaml:"name"`
		// Type defines the type of the property
		Type string `json:"type" yaml:"type"`

		Namespace bool `json:"namespace" yaml:"namespace"`

		DefaultValue any `json:"default_value" yaml:"default_value"`

		UserConfigurable bool `json:"user_configurable" yaml:"user_configurable"`

		OperationalStep *OperationalStep `json:"operational_step" yaml:"operational_step"`
	}

	Classification struct {
		Is    []string `json:"is"`
		Gives []Gives  `json:"gives"`
	}

	Gives struct {
		Attribute     string
		Functionality []string
	}
)

func (g *Gives) UnmarshalJSON(content []byte) error {
	givesString := string(content)
	if givesString == "" {
		return nil
	}
	gives := strings.Split(givesString, ":")
	g.Attribute = strings.ReplaceAll(gives[0], "\"", "")
	if len(gives) == 1 {
		g.Functionality = []string{"*"}
		return nil
	}
	g.Functionality = strings.Split(strings.ReplaceAll(gives[1], "\"", ""), ",")
	return nil
}

func (template ResourceTemplate) Id() construct.ResourceId {
	args := strings.Split(template.QualifiedTypeName, ":")
	return construct.ResourceId{
		Provider: args[0],
		Type:     args[1],
	}
}

func (template ResourceTemplate) GivesAttributeForFunctionality(attribute string, functionality construct.Functionality) bool {
	for _, give := range template.Classification.Gives {
		if give.Attribute == attribute && (collectionutil.Contains(give.Functionality, string(functionality)) || collectionutil.Contains(give.Functionality, "*")) {
			return true
		}
	}
	return false
}

func (template ResourceTemplate) getFunctionality() construct.Functionality {
	if len(template.Classification.Is) == 0 {
		return construct.Unknown
	}
	var functionality construct.Functionality
	for _, c := range template.Classification.Is {
		matched := true
		alreadySet := functionality != ""
		switch c {
		case "compute":
			functionality = construct.Compute
		case "cluster":
			functionality = construct.Cluster
		case "storage":
			functionality = construct.Storage
		case "api":
			functionality = construct.Api
		case "messaging":
			functionality = construct.Messaging
		default:
			matched = false
		}
		if matched && alreadySet {
			return construct.Unknown
		}
	}
	if functionality == "" {
		return construct.Unknown
	}
	return functionality
}

func (template ResourceTemplate) ResourceContainsClassifications(needs []string) bool {
	for _, need := range needs {
		if !collectionutil.Contains(template.Classification.Is, need) && template.QualifiedTypeName != need {
			return false
		}
	}
	return true
}

func (template ResourceTemplate) GetNamespacedProperty() *Property {
	for _, property := range template.Properties {
		if property.Namespace {
			return &property
		}
	}
	return nil
}

func (template ResourceTemplate) GetProperty(name string) *Property {
	for _, property := range template.Properties {
		if property.Name == name {
			return &property
		}
	}
	return nil
}