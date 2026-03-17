package kubernetes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func BuildConfigMap(req model.RunRequest, namespace string) (*corev1.ConfigMap, error) {
	runYAML, err := yaml.Marshal(req.RunConfig)
	if err != nil {
		return nil, err
	}
	policyYAML, err := yaml.Marshal(req.Policy)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-runner-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"run.yaml":    string(runYAML),
			"policy.yaml": string(policyYAML),
		},
	}, nil
}

func RenderJobYAML(job any) (string, error) {
	data, err := yaml.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RenderConfigMapYAML(cm *corev1.ConfigMap) (string, error) {
	data, err := yaml.Marshal(cm)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func resourceList(cpu, memory string) corev1.ResourceList {
	result := corev1.ResourceList{}
	if parsedCPU, err := resourceParse(cpu); err == nil {
		result[corev1.ResourceCPU] = parsedCPU
	}
	if parsedMem, err := resourceParse(memory); err == nil {
		result[corev1.ResourceMemory] = parsedMem
	}
	return result
}

func resourceParse(value string) (resource.Quantity, error) {
	q, err := resource.ParseQuantity(value)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("parse quantity %s: %w", value, err)
	}
	return q, nil
}
