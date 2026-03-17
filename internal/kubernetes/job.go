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
		"app":     "sandbox-runner",
		"run_id":  runID,
		"attempt": attempt,
	}

	command := []string{"/usr/local/bin/sandbox-run", "run", "--config", "/etc/sandbox/run.yaml", "--policy", "/etc/sandbox/policy.yaml", "--"}
	command = append(command, req.RunConfig.Run.Command...)

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-run-" + runID,
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
					ServiceAccountName: "sandbox-runner",
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
						{Name: "run-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "sandbox-runner-config"}}}},
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
