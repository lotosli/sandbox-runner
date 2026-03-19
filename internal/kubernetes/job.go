package kubernetes

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func BuildJob(req model.RunRequest, namespace string) *batchv1.Job {
	runID := req.RunConfig.Run.RunID
	attempt := fmt.Sprintf("%d", req.RunConfig.Run.Attempt)
	labels := map[string]string{
		"app":              "sandbox-runner",
		"run_id":           runID,
		"attempt":          attempt,
		"backend_kind":     string(req.RunConfig.Backend.Kind),
		"backend_provider": backendProvider(req.RunConfig),
		"runtime_profile":  string(req.RunConfig.Runtime.Profile),
	}

	command := []string{"/usr/local/bin/sandbox-runner", "run", "--config", "/etc/sandbox/run.yaml", "--policy", "/etc/sandbox/policy.yaml", "--"}
	command = append(command, req.RunConfig.Run.Command...)
	runConfigName := configMapName(runID)

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-runner-" + runID,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: int32Ptr(1800),
			BackoffLimit:            int32Ptr(0),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: serviceAccountName(req),
					RuntimeClassName:   runtimeClassName(req),
					Containers: []corev1.Container{
						{
							Name:            "runner",
							Image:           defaultImage(req),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         command,
							Env: []corev1.EnvVar{
								{Name: "RUN_ID", Value: runID},
								{Name: "ATTEMPT", Value: attempt},
								{Name: "SANDBOX_ID", Value: req.RunConfig.Run.SandboxID},
								{Name: "BACKEND_KIND", Value: string(req.RunConfig.Backend.Kind)},
								{Name: "BACKEND_PROVIDER", Value: backendProvider(req.RunConfig)},
								{Name: "RUNTIME_PROFILE", Value: string(req.RunConfig.Runtime.Profile)},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
								{Name: "run-config", MountPath: "/etc/sandbox"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: resourceList("1", "2Gi"),
								Limits:   resourceList("4", "8Gi"),
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "run-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: runConfigName}}}},
					},
				},
			},
		},
	}
}

func defaultImage(req model.RunRequest) string {
	if req.RunConfig.Run.Image != "" {
		return req.RunConfig.Run.Image
	}
	return "registry.company.internal/ai/sandbox-runner:latest"
}

func runtimeClassName(req model.RunRequest) *string {
	if !model.RequiresRuntimeClass(model.ExecutionRuntimeProfileForConfig(req.RunConfig)) {
		return nil
	}
	value := model.RuntimeClassNameForConfig(req.RunConfig)
	if value == "" {
		return nil
	}
	return &value
}

func backendProvider(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "docker"
	case model.BackendKindK8s:
		return string(model.ExecutionProviderForK8sProvider(cfg.K8s.Provider))
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	default:
		return string(cfg.Backend.Kind)
	}
}

func serviceAccountName(req model.RunRequest) string {
	if req.RunConfig.K8s.ServiceAccountName != "" {
		return req.RunConfig.K8s.ServiceAccountName
	}
	return "default"
}
