package resources

import (
	"testing"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/construct/coretesting"
	"github.com/stretchr/testify/assert"
)

func Test_RdsInstanceCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[RdsInstanceCreateParams, *RdsInstance]{
		{
			Name: "nil check ip",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_instance:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, instance *RdsInstance) {
				assert.Equal(instance.Name, "my-app-instance")
				assert.Equal(instance.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "nil check ip",
			Existing: &RdsInstance{Name: "my-app-instance", ConstructRefs: initialRefs},
			WantErr:  true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = RdsInstanceCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "instance",
			}
			tt.Run(t)
		})
	}
}

func Test_RdsSubnetGroupCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[RdsSubnetGroupCreateParams, *RdsSubnetGroup]{
		{
			Name: "nil subnet group",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_subnet_group:my-app-sg",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, sg *RdsSubnetGroup) {
				assert.Equal(sg.Name, "my-app-sg")
				assert.Equal(sg.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "existing subnet group",
			Existing: &RdsSubnetGroup{Name: "my-app-sg", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_subnet_group:my-app-sg",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, sg *RdsSubnetGroup) {
				assert.Equal(sg.Name, "my-app-sg")
				assert.Equal(sg.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = RdsSubnetGroupCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "sg",
			}
			tt.Run(t)
		})
	}
}

func Test_RdsProxyCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[RdsProxyCreateParams, *RdsProxy]{
		{
			Name: "nil proxy",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_proxy:my-app-proxy",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, proxy *RdsProxy) {
				assert.Equal(proxy.Name, "my-app-proxy")
				assert.Equal(proxy.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "existing proxy",
			Existing: &RdsProxy{Name: "my-app-proxy", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_proxy:my-app-proxy",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, proxy *RdsProxy) {
				assert.Equal(proxy.Name, "my-app-proxy")
				assert.Equal(proxy.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = RdsProxyCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "proxy",
			}
			tt.Run(t)
		})
	}
}

func Test_RdsProxyTargetGroupCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[RdsProxyTargetGroupCreateParams, *RdsProxyTargetGroup]{
		{
			Name: "nil proxy",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_proxy_target_group:my-app-proxy",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, proxy *RdsProxyTargetGroup) {
				assert.Equal(proxy.Name, "my-app-proxy")
				assert.Equal(proxy.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "existing proxy",
			Existing: &RdsProxyTargetGroup{Name: "my-app-proxy", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:rds_proxy_target_group:my-app-proxy",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, proxy *RdsProxyTargetGroup) {
				assert.Equal(proxy.Name, "my-app-proxy")
				assert.Equal(proxy.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = RdsProxyTargetGroupCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "proxy",
			}
			tt.Run(t)
		})
	}
}
