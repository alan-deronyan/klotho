package operational_eval

import (
	"testing"

	"github.com/dominikbraun/graph"
	construct "github.com/klothoplatform/klotho/pkg/construct2"
	"github.com/klothoplatform/klotho/pkg/engine2/constraints"
	knowledgebase "github.com/klothoplatform/klotho/pkg/knowledge_base2"
	"github.com/klothoplatform/klotho/pkg/knowledge_base2/properties"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
)

func Test_propertyVertex_evaluateResourceOperational(t *testing.T) {
	rule := &knowledgebase.PropertyRule{
		Value: "test",
	}
	type args struct {
		v   *propertyVertex
		res *construct.Resource
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "property rule",
			args: args{
				v: &propertyVertex{
					Ref: construct.PropertyRef{
						Property: "test",
						Resource: construct.ResourceId{Name: "test"},
					},
					Template: &properties.StringProperty{
						PropertyDetails: knowledgebase.PropertyDetails{
							OperationalRule: rule,
						},
					},
				},
				res: &construct.Resource{ID: construct.ResourceId{Name: "test"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			ctrl := gomock.NewController(t)
			opctx := NewMockOpRuleHandler(ctrl)
			opctx.EXPECT().HandlePropertyRule(*rule).Return(nil).Times(1)
			err := tt.args.v.evaluateResourceOperational(tt.args.res, opctx)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			ctrl.Finish()
		})
	}
}

func Test_propertyVertex_evaluateEdgeOperational(t *testing.T) {
	rule := knowledgebase.OperationalRule{
		If: "test",
	}
	type args struct {
		v   *propertyVertex
		res *construct.Resource
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "property rule",
			args: args{
				v: &propertyVertex{
					Ref: construct.PropertyRef{
						Property: "test",
						Resource: construct.ResourceId{Name: "test"},
					},
					EdgeRules: map[construct.SimpleEdge][]knowledgebase.OperationalRule{
						{
							Source: construct.ResourceId{Name: "test"},
							Target: construct.ResourceId{Name: "test2"},
						}: {
							rule,
						},
					},
				},
				res: &construct.Resource{ID: construct.ResourceId{Name: "test"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			ctrl := gomock.NewController(t)
			opctx := NewMockOpRuleHandler(ctrl)
			opctx.EXPECT().SetData(knowledgebase.DynamicValueData{
				Resource: tt.args.v.Ref.Resource,
				Edge:     &graph.Edge[construct.ResourceId]{Source: construct.ResourceId{Name: "test"}, Target: construct.ResourceId{Name: "test2"}},
			}).Times(1)
			opctx.EXPECT().HandleOperationalRule(rule).Return(nil).Times(1)
			err := tt.args.v.evaluateEdgeOperational(tt.args.res, opctx)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			ctrl.Finish()
		})
	}
}

func Test_propertyVertex_evaluateConstraints(t *testing.T) {
	id := construct.ResourceId{Provider: "test", Type: "test", Name: "test"}
	type testData struct {
		name    string
		ref     construct.PropertyRef
		res     *construct.Resource
		rcs     []constraints.ResourceConstraint
		mocks   func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData)
		wantErr bool
	}
	tests := []testData{
		{
			name: "existing value and no constraints",
			ref: construct.PropertyRef{
				Resource: id,
				Property: "test",
			},
			res: &construct.Resource{ID: id, Properties: construct.Properties{"test": "test"}},
			mocks: func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData) {
			},
		},
		{
			name: "no value and no constraints sets default",
			ref: construct.PropertyRef{
				Resource: id,
				Property: "test",
			},
			res: &construct.Resource{ID: id},
			mocks: func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData) {
				mockProperty.EXPECT().GetDefaultValue(gomock.Any(),
					knowledgebase.DynamicValueData{Resource: id}).Return("test", nil).Times(1)
				mockRc.EXPECT().ConfigureResource(data.res,
					knowledgebase.Configuration{Field: "test", Value: "test"},
					knowledgebase.DynamicValueData{Resource: id},
					"set",
					false).Times(1)
			},
		},
		{
			name: "imported - no value and no constraints does not set default",
			ref: construct.PropertyRef{
				Resource: id,
				Property: "test",
			},
			res:   &construct.Resource{ID: id, Imported: true},
			mocks: func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData) {},
		},
		{
			name: "set constraint",
			ref: construct.PropertyRef{
				Resource: id,
				Property: "test",
			},
			res: &construct.Resource{ID: id},
			rcs: []constraints.ResourceConstraint{
				{
					Operator: constraints.EqualsConstraintOperator,
					Target:   id,
					Property: "test",
					Value:    "test",
				},
			},
			mocks: func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData) {
				mockRc.EXPECT().ConfigureResource(data.res,
					knowledgebase.Configuration{Field: "test", Value: "test"},
					knowledgebase.DynamicValueData{Resource: id},
					"set",
					true).Times(1)
			},
		},
		{
			name: "add constraints",
			ref: construct.PropertyRef{
				Resource: id,
				Property: "test",
			},
			res: &construct.Resource{ID: id},
			rcs: []constraints.ResourceConstraint{
				{
					Operator: constraints.AddConstraintOperator,
					Target:   id,
					Property: "test",
					Value:    "test",
				},
				{
					Operator: constraints.AddConstraintOperator,
					Target:   id,
					Property: "test",
					Value:    "test2",
				},
			},
			mocks: func(mockProperty *MockProperty, mockRc *MockResourceConfigurer, data testData) {
				mockProperty.EXPECT().GetDefaultValue(gomock.Any(),
					knowledgebase.DynamicValueData{Resource: id}).Return("test", nil).Times(1)
				mockRc.EXPECT().ConfigureResource(data.res,
					knowledgebase.Configuration{Field: "test", Value: "test"},
					knowledgebase.DynamicValueData{Resource: id},
					"set",
					false).Times(1)
				mockRc.EXPECT().ConfigureResource(data.res,
					knowledgebase.Configuration{Field: "test", Value: "test"},
					knowledgebase.DynamicValueData{Resource: id},
					"add",
					true).Times(1)
				mockRc.EXPECT().ConfigureResource(data.res,
					knowledgebase.Configuration{Field: "test", Value: "test2"},
					knowledgebase.DynamicValueData{Resource: id},
					"add",
					true).Times(1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockProperty := NewMockProperty(ctrl)
			mockRc := NewMockResourceConfigurer(ctrl)
			tt.mocks(mockProperty, mockRc, tt)
			v := &propertyVertex{
				Ref:      tt.ref,
				Template: mockProperty,
			}
			err := v.evaluateConstraints(mockRc, knowledgebase.DynamicValueContext{}, tt.res, tt.rcs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			ctrl.Finish()
		})
	}
}
