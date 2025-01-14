package constraints

import (
	"testing"

	"github.com/klothoplatform/klotho/pkg/construct"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base"

	"github.com/klothoplatform/klotho/pkg/provider/aws/resources"
	"github.com/stretchr/testify/assert"
)

func Test_NodeConstraint_IsSatisfied(t *testing.T) {
	tests := []struct {
		name       string
		constraint ResourceConstraint
		resources  []construct.Resource
		want       bool
	}{
		{
			name: "property value is correct",
			constraint: ResourceConstraint{
				Operator: EqualsConstraintOperator,
				Target:   construct.ResourceId{Provider: "aws", Type: "rds_instance", Name: "my_instance"},
				Property: "InstanceClass",
				Value:    "db.t3.micro",
			},
			resources: []construct.Resource{
				&resources.RdsInstance{
					Name:          "my_instance",
					InstanceClass: "db.t3.micro",
				},
			},
			want: true,
		},
		{
			name: "property value is incorrect",
			constraint: ResourceConstraint{
				Operator: EqualsConstraintOperator,
				Target:   construct.ResourceId{Provider: "aws", Type: "rds_instance", Name: "my_instance"},
				Property: "InstanceClass",
				Value:    "db.t3.large",
			},
			resources: []construct.Resource{
				&resources.RdsInstance{
					Name:          "my_instance",
					InstanceClass: "db.t3.micro",
				},
			},
			want: false,
		},
		{
			name: "property value is nil",
			constraint: ResourceConstraint{
				Operator: EqualsConstraintOperator,
				Target:   construct.ResourceId{Provider: "aws", Type: "rds_instance", Name: "my_instance"},
				Property: "InstanceClass",
				Value:    "db.t3.large",
			},
			resources: []construct.Resource{
				&resources.RdsInstance{
					Name: "my_instance",
				},
			},
			want: false,
		},
		{
			name: "resource does not exist",
			constraint: ResourceConstraint{
				Operator: EqualsConstraintOperator,
				Target:   construct.ResourceId{Provider: "aws", Type: "rds_instance", Name: "my_instance"},
				Property: "InstanceClass",
				Value:    "db.t3.large",
			},
			resources: []construct.Resource{
				&resources.RdsInstance{
					Name: "my_other_instance",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			dag := construct.NewResourceGraph()
			for _, res := range tt.resources {
				dag.AddResource(res)
			}
			result := tt.constraint.IsSatisfied(dag, knowledgebase.EdgeKB{}, make(map[construct.ResourceId][]construct.Resource), nil)
			assert.Equal(tt.want, result)
		})
	}
}
