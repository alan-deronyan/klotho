package resources

import (
	"testing"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/construct/coretesting"
	"github.com/stretchr/testify/assert"
)

func Test_LoadBalancerCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[LoadBalancerCreateParams, *LoadBalancer]{
		{
			Name: "nil check ip",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:load_balancer:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, lb *LoadBalancer) {
				assert.Equal(lb.Name, "my-app-instance")
				assert.Equal(lb.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "nil check ip",
			Existing: &LoadBalancer{Name: "my-app-instance", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:load_balancer:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, lb *LoadBalancer) {
				assert.Equal(lb.Name, "my-app-instance")
				assert.Equal(lb.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = LoadBalancerCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "instance",
			}
			tt.Run(t)
		})
	}
}

func Test_TargetGroupCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[TargetGroupCreateParams, *TargetGroup]{
		{
			Name: "nil check ip",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:target_group:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, tg *TargetGroup) {
				assert.Equal(tg.Name, "my-app-instance")
				assert.Equal(tg.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "nil check ip",
			Existing: &TargetGroup{Name: "my-app-instance", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:target_group:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, tg *TargetGroup) {
				assert.Equal(tg.Name, "my-app-instance")
				assert.Equal(tg.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = TargetGroupCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "instance",
			}
			tt.Run(t)
		})
	}
}

func Test_ListenerCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[ListenerCreateParams, *Listener]{
		{
			Name: "nil check ip",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:load_balancer_listener:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, l *Listener) {
				assert.Equal(l.Name, "my-app-instance")
				assert.Equal(l.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "nil check ip",
			Existing: &Listener{Name: "my-app-instance", ConstructRefs: initialRefs},
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:load_balancer_listener:my-app-instance",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, l *Listener) {
				assert.Equal(l.Name, "my-app-instance")
				assert.Equal(l.ConstructRefs, construct.BaseConstructSetOf(eu, eu2))
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = ListenerCreateParams{
				Refs:    construct.BaseConstructSetOf(eu),
				AppName: "my-app",
				Name:    "instance",
			}
			tt.Run(t)
		})
	}
}
