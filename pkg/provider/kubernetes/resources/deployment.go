package resources

import (
	"errors"
	"fmt"

	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/engine/classification"
	"github.com/klothoplatform/klotho/pkg/provider"
	"github.com/klothoplatform/klotho/pkg/sanitization/kubernetes"
	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	Deployment struct {
		Name          string
		ConstructRefs construct.BaseConstructSet `yaml:"-"`
		Object        *apps.Deployment
		Values        map[string]construct.IaCValue
		FilePath      string
		Cluster       construct.ResourceId
	}
)

const (
	DEPLOYMENT_TYPE = "deployment"
)

func (deployment *Deployment) BaseConstructRefs() construct.BaseConstructSet {
	return deployment.ConstructRefs
}

func (deployment *Deployment) Id() construct.ResourceId {
	return construct.ResourceId{
		Provider: provider.KUBERNETES,
		Type:     DEPLOYMENT_TYPE,
		Name:     deployment.Name,
	}
}

func (deployment *Deployment) DeleteContext() construct.DeleteContext {
	return construct.DeleteContext{
		RequiresNoUpstream:     true,
		RequiresExplicitDelete: true,
	}
}

func (deployment *Deployment) GetObject() v1.Object {
	return deployment.Object
}

func (deployment *Deployment) Kind() string {
	return deployment.Object.Kind
}

func (deployment *Deployment) Path() string {
	return deployment.FilePath
}

func (deployment *Deployment) GetServiceAccount(dag *construct.ResourceGraph) *ServiceAccount {
	if deployment.Object == nil {
		sas := construct.GetDownstreamResourcesOfType[*ServiceAccount](dag, deployment)
		if len(sas) == 1 {
			return sas[0]
		}
		return nil
	}
	for _, sa := range construct.GetDownstreamResourcesOfType[*ServiceAccount](dag, deployment) {
		if sa.Object != nil && sa.Object.Name == deployment.Object.Spec.Template.Spec.ServiceAccountName {
			return sa
		}
	}
	return nil
}

func (deployment *Deployment) AddEnvVar(iacVal construct.IaCValue, envVarName string) error {

	log := zap.L().Sugar()
	log.Debugf("Adding environment variables to pod, %s", deployment.Name)

	if len(deployment.Object.Spec.Template.Spec.Containers) != 1 {
		return errors.New("expected one container in Deployment spec, cannot add environment variable")
	} else {
		k, v := GenerateEnvVarKeyValue(envVarName)

		newEv := corev1.EnvVar{
			Name:  k,
			Value: fmt.Sprintf("{{ .Values.%s }}", v),
		}

		deployment.Object.Spec.Template.Spec.Containers[0].Env = append(deployment.Object.Spec.Template.Spec.Containers[0].Env, newEv)
		if deployment.Values == nil {
			deployment.Values = make(map[string]construct.IaCValue)
		}
		deployment.Values[v] = iacVal
	}
	return nil
}

func (deployment *Deployment) MakeOperational(dag *construct.ResourceGraph, appName string, classifier classification.Classifier) error {
	if deployment.Cluster.Name == "" {
		return fmt.Errorf("deployment %s has no cluster", deployment.Name)
	}
	if deployment.Object == nil {
		deployment.Object = &apps.Deployment{}
	}

	SetDefaultObjectMeta(deployment, deployment.Object.GetObjectMeta())
	deployment.FilePath = ManifestFilePath(deployment)

	// Add klothoId label to the deployment's pod template and as a selector properly associate the pods with their owning deployment
	if deployment.Object.Spec.Template.Labels == nil {
		deployment.Object.Spec.Template.Labels = make(map[string]string)
	}
	deployment.Object.Spec.Template.Labels[KLOTHO_ID_LABEL] = kubernetes.LabelValueSanitizer.Apply(deployment.Id().String())
	deployment.Object.Spec.Selector = &v1.LabelSelector{MatchLabels: KlothoIdSelector(deployment.Object)}

	// TODO: consider changing this once ports are properly configurable
	// Map default port for containers if none are specified
	for i, container := range deployment.Object.Spec.Template.Spec.Containers {
		containerP := &container
		if len(containerP.Ports) == 0 {
			containerP.Ports = append(containerP.Ports, corev1.ContainerPort{
				Name:          "default-tcp",
				ContainerPort: 3000,
				HostPort:      3000 + int32(i),
				Protocol:      corev1.ProtocolTCP,
			})
			deployment.Object.Spec.Template.Spec.Containers[i] = *containerP
		}
	}

	return nil
}

func (deployment *Deployment) GetValues() map[string]construct.IaCValue {
	return deployment.Values
}
