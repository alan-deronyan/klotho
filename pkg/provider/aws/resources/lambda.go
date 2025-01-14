package resources

import (
	"fmt"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/sanitization/aws"
)

const (
	LAMBDA_FUNCTION_TYPE   = "lambda_function"
	LAMBDA_PERMISSION_TYPE = "lambda_permission"
)

var lambdaFunctionSanitizer = aws.LambdaFunctionSanitizer
var LambdaPermissionSanitizer = aws.LambdaPermissionSanitizer

type (
	LambdaFunction struct {
		Name                 string
		ConstructRefs        construct.BaseConstructSet `yaml:"-"`
		Role                 *IamRole
		Image                *EcrImage
		EnvironmentVariables map[string]construct.IaCValue `yaml:"-"`
		SecurityGroups       []*SecurityGroup
		Subnets              []*Subnet
		Timeout              int
		MemorySize           int
		EfsAccessPoint       *EfsAccessPoint
	}

	LambdaPermission struct {
		Name          string
		ConstructRefs construct.BaseConstructSet `yaml:"-"`
		Function      *LambdaFunction
		Principal     string
		Source        construct.IaCValue
		Action        string
	}
)

type LambdaCreateParams struct {
	AppName string
	Refs    construct.BaseConstructSet
	Name    string
}

func (lambda *LambdaFunction) Create(dag *construct.ResourceGraph, params LambdaCreateParams) error {

	name := lambdaFunctionSanitizer.Apply(fmt.Sprintf("%s-%s", params.AppName, params.Name))
	lambda.Name = name
	lambda.ConstructRefs = params.Refs.Clone()

	existingLambda := dag.GetResource(lambda.Id())
	if existingLambda != nil {
		return fmt.Errorf("lambda with name %s already exists", name)
	}

	logGroup, err := construct.CreateResource[*LogGroup](dag, params)
	if err != nil {
		return err
	}
	dag.AddDependency(lambda, logGroup)
	return nil
}

type LambdaPermissionCreateParams struct {
	AppName string
	Refs    construct.BaseConstructSet
	Name    string
}

func (permission *LambdaPermission) Create(dag *construct.ResourceGraph, params LambdaPermissionCreateParams) error {

	permission.Name = LambdaPermissionSanitizer.Apply(fmt.Sprintf("%s-%s", params.AppName, params.Name))
	if params.AppName == "" {
		permission.Name = LambdaPermissionSanitizer.Apply(params.Name)
	}
	permission.ConstructRefs = params.Refs.Clone()

	existingLambdaPermission := dag.GetResource(permission.Id())
	if existingLambdaPermission != nil {
		graphLambdaPermission := existingLambdaPermission.(*LambdaPermission)
		graphLambdaPermission.ConstructRefs.AddAll(params.Refs)
		return nil
	}
	dag.AddResource(permission)
	return nil
}

// BaseConstructRefs returns AnnotationKey of the klotho resource the cloud resource is correlated to
func (lambda *LambdaFunction) BaseConstructRefs() construct.BaseConstructSet {
	return lambda.ConstructRefs
}

// Id returns the id of the cloud resource
func (lambda *LambdaFunction) Id() construct.ResourceId {
	return construct.ResourceId{
		Provider: AWS_PROVIDER,
		Type:     LAMBDA_FUNCTION_TYPE,
		Name:     lambda.Name,
	}
}

func (lambda *LambdaFunction) DeleteContext() construct.DeleteContext {
	return construct.DeleteContext{
		RequiresNoUpstream:     true,
		RequiresNoDownstream:   true,
		RequiresExplicitDelete: true,
	}
}

// BaseConstructRefs returns AnnotationKey of the klotho resource the cloud resource is correlated to
func (permission *LambdaPermission) BaseConstructRefs() construct.BaseConstructSet {
	return permission.ConstructRefs
}

// Id returns the id of the cloud resource
func (permission *LambdaPermission) Id() construct.ResourceId {
	return construct.ResourceId{
		Provider: AWS_PROVIDER,
		Type:     LAMBDA_PERMISSION_TYPE,
		Name:     permission.Name,
	}
}

func (permission *LambdaPermission) DeleteContext() construct.DeleteContext {
	return construct.DeleteContext{
		RequiresNoUpstream: true,
	}
}
