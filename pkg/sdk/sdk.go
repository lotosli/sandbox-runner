package sdk

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/lotosli/sandbox-runner/internal/kubernetes"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type RunRequest = model.RunRequest

type JobSpecBuilder struct{}

func NewJobSpecBuilder() JobSpecBuilder { return JobSpecBuilder{} }

func (JobSpecBuilder) Build(req model.RunRequest, namespace string) (*batchv1.Job, error) {
	return kubernetes.BuildJob(req, namespace), nil
}

type SubmitOptions struct {
	Namespace string
}

type SubmitResult struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
}

type Submitter struct {
	client kubernetes.Client
}

func NewSubmitter(kubeconfig string) (*Submitter, error) {
	client, err := kubernetes.NewClient(kubeconfig)
	if err != nil {
		return nil, err
	}
	return &Submitter{client: client}, nil
}

func (s *Submitter) SubmitJob(ctx context.Context, job *batchv1.Job) (*SubmitResult, error) {
	created, err := s.client.CreateJob(ctx, job)
	if err != nil {
		return nil, err
	}
	return &SubmitResult{
		Namespace: created.Namespace,
		Name:      created.Name,
		Labels:    created.Labels,
	}, nil
}

func RenderJobYAML(job *batchv1.Job) (string, error) {
	return kubernetes.RenderJobYAML(job)
}

func RenderConfigMapYAML(configMap *corev1.ConfigMap) (string, error) {
	return kubernetes.RenderConfigMapYAML(configMap)
}
