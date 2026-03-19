package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/lotosli/sandbox-runner/internal/platform"
)

type Client interface {
	CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error)
	ApplyConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
	ReadRuntimeClass(ctx context.Context, name string) error
}

type client struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
}

var (
	inClusterTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	inClusterCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

func NewClient(kubeconfig, contextName string) (Client, error) {
	cfg, err := RESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}
	return newClientFromRESTConfig(cfg)
}

func RESTConfig(kubeconfig, contextName string) (*rest.Config, error) {
	if kubeconfig == "" {
		if cfg, err := rest.InClusterConfig(); err == nil {
			return cfg, nil
		}
		if cfg, ok, err := serviceAccountRESTConfig(); ok {
			if err != nil {
				return nil, fmt.Errorf("build in-cluster config: %w", err)
			}
			return cfg, nil
		}
	}
	path := platform.KubeconfigPath(kubeconfig)
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kube config: %w", err)
	}
	return cfg, nil
}

func serviceAccountRESTConfig() (*rest.Config, bool, error) {
	if _, err := os.Stat(inClusterTokenPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, err
	}
	tokenBytes, err := os.ReadFile(inClusterTokenPath)
	if err != nil {
		return nil, true, err
	}
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	if host == "" {
		host = "kubernetes.default.svc"
	}
	port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
	if port == "" {
		port = "443"
	}
	cfg := &rest.Config{
		Host:        "https://" + net.JoinHostPort(host, port),
		BearerToken: strings.TrimSpace(string(tokenBytes)),
	}
	if _, err := os.Stat(inClusterCAPath); err == nil {
		cfg.TLSClientConfig = rest.TLSClientConfig{CAFile: inClusterCAPath}
	} else if !os.IsNotExist(err) {
		return nil, true, err
	} else {
		cfg.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	}
	return cfg, true, nil
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

func (c *client) ApplyConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	if configMap == nil {
		return nil, fmt.Errorf("configmap is required")
	}
	if strings.TrimSpace(configMap.Namespace) == "" {
		return nil, fmt.Errorf("configmap namespace is required")
	}
	if strings.TrimSpace(configMap.Name) == "" {
		return nil, fmt.Errorf("configmap name is required")
	}
	body, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("marshal configmap: %w", err)
	}
	resourcePath := c.resourceURL("/api", "v1", "namespaces", configMap.Namespace, "configmaps", configMap.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourcePath, nil)
	if err != nil {
		return nil, fmt.Errorf("build get configmap request: %w", err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lookup configmap: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		updateReq, err := http.NewRequestWithContext(ctx, http.MethodPut, resourcePath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build update configmap request: %w", err)
		}
		updateReq.Header.Set("Accept", "application/json")
		updateReq.Header.Set("Content-Type", "application/json")
		if c.userAgent != "" {
			updateReq.Header.Set("User-Agent", c.userAgent)
		}
		return c.doConfigMap(updateReq, "update configmap")
	case http.StatusNotFound:
		createPath := c.resourceURL("/api", "v1", "namespaces", configMap.Namespace, "configmaps")
		createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createPath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build create configmap request: %w", err)
		}
		createReq.Header.Set("Accept", "application/json")
		createReq.Header.Set("Content-Type", "application/json")
		if c.userAgent != "" {
			createReq.Header.Set("User-Agent", c.userAgent)
		}
		return c.doConfigMap(createReq, "create configmap")
	default:
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("lookup configmap: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
}

func (c *client) ReadRuntimeClass(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("runtime class name is required")
	}
	endpoint := c.resourceURL("/apis", "node.k8s.io", "v1", "runtimeclasses", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build read runtimeclass request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("read runtimeclass: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("read runtimeclass: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (c *client) doConfigMap(req *http.Request, action string) (*corev1.ConfigMap, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s: %s: %s", action, resp.Status, strings.TrimSpace(string(data)))
	}
	var configMap corev1.ConfigMap
	if err := json.NewDecoder(resp.Body).Decode(&configMap); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", action, err)
	}
	return &configMap, nil
}

func (c *client) resourceURL(prefix string, segments ...string) string {
	urlCopy := *c.baseURL
	all := []string{strings.Trim(urlCopy.Path, "/"), strings.Trim(prefix, "/")}
	all = append(all, segments...)
	urlCopy.Path = "/" + path.Join(all...)
	return urlCopy.String()
}
