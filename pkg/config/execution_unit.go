package config

import (
	"encoding/json"

	"go.uber.org/zap"
)

type (
	// ExecutionUnit is how execution unit Klotho constructs are represented in the klotho configuration
	ExecutionUnit struct {
		Type                 string            `json:"type" yaml:"type" toml:"type"`
		NetworkPlacement     string            `json:"network_placement,omitempty" yaml:"network_placement,omitempty" toml:"network_placement,omitempty"`
		EnvironmentVariables map[string]string `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty" toml:"environment_variables,omitempty"`
		HelmChartOptions     HelmChartOptions  `json:"helm_chart_options,omitempty" yaml:"helm_chart_options,omitempty" toml:"helm_chart_options,omitempty"`
		InfraParams          InfraParams       `json:"infra_params,omitempty" yaml:"infra_params,omitempty" toml:"infra_params,omitempty"`
	}

	// HelmChartOptions represents configuration for execution units attempting to generate helm charts
	HelmChartOptions struct {
		Directory   string   `json:"directory,omitempty" yaml:"directory,omitempty" toml:"directory,omitempty"` // Directory signals the directory which will contain the helm chart outputs
		ValuesFiles []string `json:"values_files,omitempty" yaml:"values_files,omitempty" toml:"values_files,omitempty"`
	}

	// ServerlessKindParams represents the KindParams, configurability, of execution units which match the serverless compatibility
	ServerlessTypeParams struct {
		Timeout int `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
		Memory  int `json:"memory,omitempty" yaml:"memory,omitempty" toml:"memory,omitempty"`
	}

	ContainerTypeParams struct {
		// Cpu specifies the number of cpu units to allocate to the ECS task
		Cpu int `json:"cpu,omitempty" yaml:"cpu,omitempty" toml:"cpu,omitempty"`
		// DesiredCount specifies the number of instances of an ECS task definition to deploy and keep running
		DesiredCount int `json:"desired_count,omitempty" yaml:"desired_count,omitempty" toml:"desired_count,omitempty"`
		// Memory specifies the memory allocation for the ECS task (in MiB)
		Memory int `json:"memory,omitempty" yaml:"memory,omitempty" toml:"memory,omitempty"`
		// ForceNewDeployment specifies whether to force a new deployment of the ECS service's task definition
		ForceNewDeployment bool `json:"force_new_deployment,omitempty" yaml:"force_new_deployment,omitempty" toml:"force_new_deployment,omitempty"`
		// DeploymentCircuitBreaker specifies the deployment circuit breaker configuration for the ECS service
		DeploymentCircuitBreaker EcsDeploymentCircuitBreaker `json:"deployment_circuit_breaker,omitempty" yaml:"deployment_circuit_breaker,omitempty" toml:"deployment_circuit_breaker,omitempty"`
	}

	EcsDeploymentCircuitBreaker struct {
		// Enable specifies whether to enable the deployment circuit breaker
		Enable bool `json:"enable,omitempty" yaml:"enable,omitempty" toml:"enable,omitempty"`
		// Rollback specifies whether to roll back the service if a deployment fails
		Rollback bool `json:"rollback,omitempty" yaml:"rollback,omitempty" toml:"rollback,omitempty"`
	}

	// KubernetesKindParams represents the KindParams, configurability, of execution units which match the kubernetes compatibility
	KubernetesTypeParams struct {
		ClusterId                      string                                   `json:"cluster_id,omitempty" yaml:"cluster_id,omitempty" toml:"cluster_id,omitempty"`
		NodeType                       string                                   `json:"node_type,omitempty" yaml:"node_type,omitempty" toml:"node_type,omitempty"`
		Replicas                       int                                      `json:"replicas,omitempty" yaml:"replicas,omitempty" toml:"replicas,omitempty"`
		InstanceType                   string                                   `json:"instance_type,omitempty" yaml:"instance_type,omitempty" toml:"instance_type,omitempty"`
		DiskSizeGiB                    int                                      `json:"disk_size_gib,omitempty" yaml:"disk_size_gib,omitempty" toml:"disk_size_gib,omitempty"`
		Limits                         KubernetesLimits                         `json:"limits,omitempty" yaml:"limits,omitempty" toml:"limits,omitempty"`
		HorizontalPodAutoScalingConfig KubernetesHorizontalPodAutoScalingConfig `json:"horizontal_pod_autoscaling,omitempty" yaml:"horizontal_pod_autoscaling,omitempty" toml:"horizontal_pod_autoscaling,omitempty"`
	}

	// KubernetesLimits represents the configurability of kubernetes limits for execution units which match the kubernetes compatibility
	KubernetesLimits struct {
		// Cpu specifies the limit per pod in millicores. It is "any" so that the user can specify it as either a string or a number
		Cpu any `json:"cpu,omitempty" yaml:"cpu,omitempty" toml:"cpu,omitempty"`
		// Memory specifies the limit per pod in MB
		Memory any `json:"memory,omitempty" yaml:"memory,omitempty" toml:"memory,omitempty"`
	}

	// KubernetesLimits represents the configurability of kubernetes limits for execution units which match the kubernetes compatibility
	KubernetesHorizontalPodAutoScalingConfig struct {
		// MemoryUtilization specifies the percentage of cpu a pod can utilize before the cluster will attempt to scale the pod
		CpuUtilization int `json:"cpu_utilization,omitempty" yaml:"cpu_utilization,omitempty" toml:"cpu_utilization,omitempty"`
		// MemoryUtilization specifies the percentage of memory a pod can utilize before the cluster will attempt to scale the pod
		MemoryUtilization int `json:"memory_utilization,omitempty" yaml:"memory_utilization,omitempty" toml:"memory_utilization,omitempty"`
		// MaxReplicas specifies the maximum number of pods the cluster will scale the pod spec to
		MaxReplicas int `json:"max_replicas,omitempty" yaml:"max_replicas,omitempty" toml:"max_replicas,omitempty"`
	}
)

// GetExecutionUnit returns the `ExecutionUnit` config for the resource specified by `id`
// merged with the defaults.
func (a Application) GetExecutionUnit(id string) ExecutionUnit {
	cfg := ExecutionUnit{
		Type:                 a.Defaults.ExecutionUnit.Type,
		NetworkPlacement:     "private",
		EnvironmentVariables: make(map[string]string),
	}

	ecfg, hasOverride := a.ExecutionUnits[id]
	if hasOverride {
		overrideValue(&cfg.Type, ecfg.Type)
		overrideValue(&cfg.NetworkPlacement, ecfg.NetworkPlacement)
		for k, v := range ecfg.EnvironmentVariables {
			cfg.EnvironmentVariables[k] = v
		}
		overrideValue(&cfg.HelmChartOptions.Directory, ecfg.HelmChartOptions.Directory)
		cfg.HelmChartOptions.ValuesFiles = append(cfg.HelmChartOptions.ValuesFiles, ecfg.HelmChartOptions.ValuesFiles...)
		cfg.InfraParams = ecfg.InfraParams
	}
	cfg.InfraParams.ApplyDefaults(a.Defaults.ExecutionUnit.InfraParamsByType[cfg.Type])

	return cfg
}

func (cfg ExecutionUnit) GetExecutionUnitParamsAsKubernetes() KubernetesTypeParams {

	infraParams := cfg.InfraParams
	jsonString, err := json.Marshal(infraParams)
	if err != nil {
		zap.S().Error(err)
	}

	params := KubernetesTypeParams{}
	if err := json.Unmarshal(jsonString, &params); err != nil {
		zap.S().Error(err)
	}
	return params
}

func (hpa KubernetesHorizontalPodAutoScalingConfig) NotEmpty() bool {
	var zero KubernetesHorizontalPodAutoScalingConfig
	return hpa != zero
}
