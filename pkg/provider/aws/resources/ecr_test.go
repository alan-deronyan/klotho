package resources

import (
	"testing"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/construct/coretesting"
	"github.com/stretchr/testify/assert"
)

func Test_EcrRepositoryCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "first"}
	eu2 := &types.ExecutionUnit{Name: "test"}
	initialRefs := construct.BaseConstructSetOf(eu)
	cases := []struct {
		name string
		repo *EcrRepository
		want coretesting.ResourcesExpectation
	}{
		{
			name: "nil repo",
			want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:ecr_repo:my-app",
				},
				Deps: []coretesting.StringDep{},
			},
		},
		{
			name: "existing repo",
			repo: &EcrRepository{Name: "my-app", ConstructRefs: initialRefs, ForceDelete: true},
			want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:ecr_repo:my-app",
				},
				Deps: []coretesting.StringDep{},
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			dag := construct.NewResourceGraph()
			if tt.repo != nil {
				dag.AddResource(tt.repo)
			}
			metadata := RepoCreateParams{
				AppName: "my-app",
				Refs:    construct.BaseConstructSetOf(eu2),
			}

			repo := &EcrRepository{}
			err := repo.Create(dag, metadata)

			if !assert.NoError(err) {
				return
			}

			tt.want.Assert(t, dag)
			graphRepo := dag.GetResource(repo.Id())
			repo = graphRepo.(*EcrRepository)
			assert.Equal(repo.Name, "my-app")
			if tt.repo == nil {
				assert.Equal(repo.ConstructRefs, metadata.Refs)
			} else {
				assert.Equal(repo, tt.repo)
				expect := initialRefs.CloneWith(construct.BaseConstructSetOf(eu2))
				assert.Equal(repo.BaseConstructRefs(), expect)
			}
		})
	}
}

func Test_EcrImageCreate(t *testing.T) {
	eu := &types.ExecutionUnit{Name: "test"}
	eu2 := &types.ExecutionUnit{Name: "first"}
	initialRefs := construct.BaseConstructSetOf(eu2)
	cases := []coretesting.CreateCase[ImageCreateParams, *EcrImage]{
		{
			Name: "nil image",
			Want: coretesting.ResourcesExpectation{
				Nodes: []string{
					"aws:ecr_image:image",
				},
				Deps: []coretesting.StringDep{},
			},
			Check: func(assert *assert.Assertions, image *EcrImage) {
				assert.Equal(image.Name, "image")
				assert.Equal(image.ConstructRefs, construct.BaseConstructSetOf(eu))
			},
		},
		{
			Name:     "existing image",
			Existing: &EcrImage{Name: "image", ConstructRefs: initialRefs},
			WantErr:  true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Params = ImageCreateParams{
				AppName: "my-app",
				Refs:    construct.BaseConstructSetOf(eu),
				Name:    "image",
			}
			tt.Run(t)
		})
	}
}
