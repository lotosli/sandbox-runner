package kubernetes

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/lotosli/sandbox-runner/internal/platform"
)

type Client interface {
	CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error)
}

type client struct {
	cs *kubernetes.Clientset
}

func NewClient(kubeconfig, contextName string) (Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil || kubeconfig != "" || contextName != "" {
		path := platform.KubeconfigPath(kubeconfig)
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
		overrides := &clientcmd.ConfigOverrides{}
		if contextName != "" {
			overrides.CurrentContext = contextName
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("build kube config: %w", err)
		}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &client{cs: cs}, nil
}

func (c *client) CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	return c.cs.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
}
