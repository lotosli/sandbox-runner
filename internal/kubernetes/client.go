package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/lotosli/sandbox-runner/internal/platform"
)

type Client interface {
	CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error)
}

type client struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
}

func NewClient(kubeconfig, contextName string) (Client, error) {
	cfg, err := kubeRESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}
	return newClientFromRESTConfig(cfg)
}

func kubeRESTConfig(kubeconfig, contextName string) (*rest.Config, error) {
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
	return cfg, nil
}

func newClientFromRESTConfig(cfg *rest.Config) (Client, error) {
	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kube transport: %w", err)
	}
	baseURL, err := url.Parse(strings.TrimRight(cfg.Host, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse kube host: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = http.DefaultClient.Timeout
	}
	return &client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		userAgent: rest.DefaultKubernetesUserAgent(),
	}, nil
}

func (c *client) CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil {
		return nil, fmt.Errorf("job is required")
	}
	if strings.TrimSpace(job.Namespace) == "" {
		return nil, fmt.Errorf("job namespace is required")
	}
	body, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("marshal job: %w", err)
	}
	endpoint := c.resourceURL("/apis", "batch", "v1", "namespaces", job.Namespace, "jobs")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build create job request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("create job: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var created batchv1.Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode create job response: %w", err)
	}
	return &created, nil
}

func (c *client) resourceURL(prefix string, segments ...string) string {
	urlCopy := *c.baseURL
	all := []string{strings.Trim(urlCopy.Path, "/"), strings.Trim(prefix, "/")}
	all = append(all, segments...)
	urlCopy.Path = "/" + path.Join(all...)
	return urlCopy.String()
}
