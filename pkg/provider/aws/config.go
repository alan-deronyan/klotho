package aws

import (
	"github.com/klothoplatform/klotho/pkg/config"
	kubernetes "github.com/klothoplatform/klotho/pkg/provider/kubernetes/resources"
)

// Enums for the types we allow in the aws provider so that we can reuse the same string within the provider
const (
	Ecs                    = "ecs"
	Lambda                 = "lambda"
	Ec2Instance            = "ec2"
	Rds_postgres           = "rds_postgres"
	Secrets_manager        = "secrets_manager"
	S3                     = "s3"
	Dynamodb               = "dynamodb"
	Elasticache            = "elasticache"
	Memorydb               = "memorydb"
	Sns                    = "sns"
	Cockroachdb_serverless = "cockroachdb_serverless"
	ApiGateway             = "apigateway"
	Alb                    = "alb"
	AppRunner              = "app_runner"
)

var (
	eksDefaults = config.KubernetesTypeParams{
		NodeType: "fargate",
		Replicas: 2,
	}

	ecsDefaults = config.ContainerTypeParams{
		Memory:             512,
		Cpu:                256,
		DesiredCount:       1,
		ForceNewDeployment: true,
		DeploymentCircuitBreaker: config.EcsDeploymentCircuitBreaker{
			Enable:   true,
			Rollback: false,
		},
	}

	lambdaDefaults = config.ServerlessTypeParams{
		Timeout: 180,
		Memory:  512,
	}
)

var defaultConfig = config.Defaults{
	ExecutionUnit: config.KindDefaults{
		Type: Lambda,
		InfraParamsByType: map[string]config.InfraParams{
			Lambda:                     config.ConvertToInfraParams(lambdaDefaults),
			Ecs:                        config.ConvertToInfraParams(ecsDefaults),
			kubernetes.DEPLOYMENT_TYPE: config.ConvertToInfraParams(eksDefaults),
		},
	},
	StaticUnit: config.KindDefaults{
		Type: S3,
	},
	Expose: config.KindDefaults{
		Type: ApiGateway,
		InfraParamsByType: map[string]config.InfraParams{
			ApiGateway: config.ConvertToInfraParams(config.GatewayTypeParams{
				ApiType: "REST",
			}),
			Alb: config.ConvertToInfraParams(config.LoadBalancerTypeParams{}),
		},
	},
	PubSub: config.KindDefaults{
		Type: Sns,
	},
	Config: config.KindDefaults{
		Type: S3,
	},
	PersistKv: config.KindDefaults{
		Type: Dynamodb,
		InfraParamsByType: map[string]config.InfraParams{
			Dynamodb: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
	PersistFs: config.KindDefaults{
		Type: S3,
		InfraParamsByType: map[string]config.InfraParams{
			S3: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
	PersistSecrets: config.KindDefaults{
		Type: Secrets_manager,
		InfraParamsByType: map[string]config.InfraParams{
			Secrets_manager: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
	PersistOrm: config.KindDefaults{
		Type: Rds_postgres,
		InfraParamsByType: map[string]config.InfraParams{
			Rds_postgres: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
	PersistRedisNode: config.KindDefaults{
		Type: Elasticache,
		InfraParamsByType: map[string]config.InfraParams{
			Elasticache: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
	PersistRedisCluster: config.KindDefaults{
		Type: Memorydb,
		InfraParamsByType: map[string]config.InfraParams{
			Memorydb: config.ConvertToInfraParams(config.InfraParams{}),
		},
	},
}

func (a *AWS) GetDefaultConfig() config.Defaults {
	return defaultConfig
}
