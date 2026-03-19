package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestClientCreateJobUsesMinimalRESTRequest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/sandbox-system/jobs" {
			t.Fatalf("path = %s, want /apis/batch/v1/namespaces/sandbox-system/jobs", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		var job batchv1.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if job.Name != "demo-job" {
			t.Fatalf("job name = %s, want demo-job", job.Name)
		}
		_ = json.NewEncoder(w).Encode(batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "sandbox-system",
				Name:      "demo-job",
				Labels:    map[string]string{"app": "sandbox-runner"},
			},
		})
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:        server.URL,
		BearerToken: "test-token",
		Timeout:     5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	created, err := restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "demo-job",
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if created.Namespace != "sandbox-system" {
		t.Fatalf("namespace = %s, want sandbox-system", created.Namespace)
	}
	if created.Name != "demo-job" {
		t.Fatalf("name = %s, want demo-job", created.Name)
	}
}

func TestClientCreateJobReportsServerErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:    server.URL,
		Timeout: 5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	_, err = restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "demo-job",
		},
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want server error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("CreateJob() error = %v, want status text", err)
	}
}

func TestClientCreateJobRequiresNamespace(t *testing.T) {
	restClient := &client{
		baseURL:    &url.URL{Scheme: "https", Host: "127.0.0.1"},
		httpClient: &http.Client{},
		userAgent:  "sandbox-runner-test",
	}

	_, err := restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-job"},
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want namespace validation error")
	}
	if !strings.Contains(err.Error(), "namespace is required") {
		t.Fatalf("CreateJob() error = %v, want namespace validation", err)
	}
}

func TestClientApplyConfigMapCreatesThenUpdates(t *testing.T) {
	requests := []string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-1":
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/namespaces/sandbox-system/configmaps":
			var configMap corev1.ConfigMap
			if err := json.NewDecoder(r.Body).Decode(&configMap); err != nil {
				t.Fatalf("Decode(create) error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(configMap)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-2":
			_ = json.NewEncoder(w).Encode(corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "sandbox-system",
					Name:      "sandbox-runner-config-run-2",
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-2":
			var configMap corev1.ConfigMap
			if err := json.NewDecoder(r.Body).Decode(&configMap); err != nil {
				t.Fatalf("Decode(update) error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(configMap)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:    server.URL,
		Timeout: 5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	created, err := restClient.ApplyConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "sandbox-runner-config-run-1",
		},
	})
	if err != nil {
		t.Fatalf("ApplyConfigMap(create) error = %v", err)
	}
	if created.Name != "sandbox-runner-config-run-1" {
		t.Fatalf("created name = %q, want sandbox-runner-config-run-1", created.Name)
	}

	updated, err := restClient.ApplyConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "sandbox-runner-config-run-2",
		},
	})
	if err != nil {
		t.Fatalf("ApplyConfigMap(update) error = %v", err)
	}
	if updated.Name != "sandbox-runner-config-run-2" {
		t.Fatalf("updated name = %q, want sandbox-runner-config-run-2", updated.Name)
	}

	want := []string{
		"GET /api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-1",
		"POST /api/v1/namespaces/sandbox-system/configmaps",
		"GET /api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-2",
		"PUT /api/v1/namespaces/sandbox-system/configmaps/sandbox-runner-config-run-2",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestClientReadRuntimeClassUsesNodeAPIGroup(t *testing.T) {
	var sawPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "node.k8s.io/v1",
			"kind":       "RuntimeClass",
			"metadata": map[string]any{
				"name": "sandbox-runner-microvm",
			},
		})
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:    server.URL,
		Timeout: 5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	if err := restClient.ReadRuntimeClass(context.Background(), "sandbox-runner-microvm"); err != nil {
		t.Fatalf("ReadRuntimeClass() error = %v", err)
	}
	if sawPath != "/apis/node.k8s.io/v1/runtimeclasses/sandbox-runner-microvm" {
		t.Fatalf("path = %q, want /apis/node.k8s.io/v1/runtimeclasses/sandbox-runner-microvm", sawPath)
	}
}

func TestRESTConfigFallsBackToServiceAccountTokenWithoutClusterEnv(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	if err := os.WriteFile(caPath, []byte("test-ca"), 0o600); err != nil {
		t.Fatalf("WriteFile(ca) error = %v", err)
	}

	originalTokenPath := inClusterTokenPath
	originalCAPath := inClusterCAPath
	inClusterTokenPath = tokenPath
	inClusterCAPath = caPath
	t.Cleanup(func() {
		inClusterTokenPath = originalTokenPath
		inClusterCAPath = originalCAPath
	})

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBECONFIG", "")

	cfg, err := RESTConfig("", "minikube")
	if err != nil {
		t.Fatalf("RESTConfig() error = %v", err)
	}
	if cfg.Host != "https://kubernetes.default.svc:443" {
		t.Fatalf("host = %q, want https://kubernetes.default.svc:443", cfg.Host)
	}
	if cfg.BearerToken != "test-token" {
		t.Fatalf("bearer token = %q, want test-token", cfg.BearerToken)
	}
	if cfg.TLSClientConfig.CAFile != caPath {
		t.Fatalf("CAFile = %q, want %q", cfg.TLSClientConfig.CAFile, caPath)
	}
}
