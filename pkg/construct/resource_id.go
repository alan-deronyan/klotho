package construct

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type ResourceId struct {
	Provider string `yaml:"provider" toml:"provider"`
	Type     string `yaml:"type" toml:"type"`
	// Namespace is optional and is used to disambiguate resources that might have
	// the same name. It can also be used to associate an imported resource with
	// a specific namespace such as a subnet to a VPC.
	Namespace string `yaml:"namespace" toml:"namespace"`
	Name      string `yaml:"name" toml:"name"`
}

var zeroId = ResourceId{}

func (id ResourceId) IsZero() bool {
	return id == zeroId
}

func (id ResourceId) String() string {
	if id.IsZero() {
		return ""
	}

	sb := strings.Builder{}
	sb.Grow(len(id.Provider) + len(id.Type) + len(id.Namespace) + len(id.Name) + 3)

	sb.WriteString(id.Provider)
	sb.WriteByte(':')
	sb.WriteString(id.Type)
	if id.Namespace != "" || strings.Contains(id.Name, ":") {
		sb.WriteByte(':')
		sb.WriteString(id.Namespace)
	}
	if id.Name != "" {
		sb.WriteByte(':')
		sb.WriteString(id.Name)
	}
	return sb.String()
}

func (id ResourceId) QualifiedTypeName() string {
	return id.Provider + ":" + id.Type
}

func (id ResourceId) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// Matches uses `id` (the receiver) as a filter for `other` (the argument) and returns true if all the non-empty fields from
// `id` match the corresponding fields in `other`.
func (id ResourceId) Matches(other ResourceId) bool {
	if id.Provider != "" && id.Provider != other.Provider {
		return false
	}
	if id.Type != "" && id.Type != other.Type {
		return false
	}
	if id.Namespace != "" && id.Namespace != other.Namespace {
		return false
	}
	if id.Name != "" && id.Name != other.Name {
		return false
	}
	return true
}

var (
	resourceProviderPattern  = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	resourceTypePattern      = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	resourceNamespacePattern = regexp.MustCompile(`^[a-zA-Z0-9_./\-\[\]]*$`) // like name, but `:` not allowed
	resourceNamePattern      = regexp.MustCompile(`^[a-zA-Z0-9_./\-:\[\]#]*$`)
)

func (id *ResourceId) UnmarshalText(data []byte) error {
	parts := strings.SplitN(string(data), ":", 4)
	switch len(parts) {
	case 4:
		id.Name = parts[3]
		fallthrough
	case 3:
		if len(parts) == 4 {
			id.Namespace = parts[2]
		} else {
			id.Name = parts[2]
		}
		fallthrough
	case 2:
		id.Type = parts[1]
		id.Provider = parts[0]
	case 1:
		if parts[0] != "" {
			return fmt.Errorf("must have trailing ':' for provider-only ID")
		}
	}
	if id.IsZero() {
		return nil
	}
	var err error
	if !resourceProviderPattern.MatchString(id.Provider) {
		err = errors.Join(err, fmt.Errorf("invalid provider '%s' (must match %s)", id.Provider, resourceProviderPattern))
	}
	if id.Type != "" && !resourceTypePattern.MatchString(id.Type) {
		err = errors.Join(err, fmt.Errorf("invalid type '%s' (must match %s)", id.Type, resourceTypePattern))
	}
	if id.Namespace != "" && !resourceNamespacePattern.MatchString(id.Namespace) {
		err = errors.Join(err, fmt.Errorf("invalid namespace '%s' (must match %s)", id.Namespace, resourceNamespacePattern))
	}
	if !resourceNamePattern.MatchString(id.Name) {
		err = errors.Join(err, fmt.Errorf("invalid name '%s' (must match %s)", id.Name, resourceNamePattern))
	}
	if err != nil {
		return fmt.Errorf("invalid resource id '%s': %w", string(data), err)
	}
	return nil
}

func (id ResourceId) MarshalTOML() ([]byte, error) {
	return id.MarshalText()
}

func (id *ResourceId) UnmarshalTOML(data []byte) error {
	return id.UnmarshalText(data)
}
