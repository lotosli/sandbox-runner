package capability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestProbeK8sSupportsNamedProviders(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderMinikube,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.K8s.Provider = model.K8sProviderMinikube
	cfg.K8s.Context = "minikube"
	cfg.K8s.Kubeconfig = writeKubeconfig(t, "minikube")

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatal("Probe() OK = false, want true")
	}
	if got := result.Details["provider"]; got != model.ProviderMinikube {
		t.Fatalf("details[provider] = %#v, want %s", got, model.ProviderMinikube)
	}
	if got := result.Details["kube_context"]; got != "minikube" {
		t.Fatalf("details[kube_context] = %#v, want minikube", got)
	}
}

func TestProbeK8sRejectsUnknownContext(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderK3s,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.K8s.Provider = model.K8sProviderK3s
	cfg.K8s.Context = "missing-context"
	cfg.K8s.Kubeconfig = writeKubeconfig(t, "k3s-default")

	_, err := Probe(context.Background(), cfg.Execution, cfg)
	if err == nil {
		t.Fatal("Probe() error = nil, want provider unreachable error")
	}
	var runnerErr model.RunnerError
	if !errors.As(err, &runnerErr) {
		t.Fatalf("Probe() error = %T, want RunnerError", err)
	}
	if runnerErr.Code != string(model.ErrorCodeCapabilityProviderUnreachable) {
		t.Fatalf("Probe() code = %s, want %s", runnerErr.Code, model.ErrorCodeCapabilityProviderUnreachable)
	}
}

func TestProbeK8sChecksRuntimeClassForMicroVM(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/node.k8s.io/v1/runtimeclasses/sandbox-runner-microvm":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"apiVersion": "node.k8s.io/v1",
				"kind":       "RuntimeClass",
				"metadata": map[string]any{
					"name": "sandbox-runner-microvm",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderK3s,
		RuntimeProfile: model.ExecutionRuntimeProfileFirecracker,
	}
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.K8s.Provider = model.K8sProviderK3s
	cfg.K8s.Context = "k3d-k3s"
	cfg.K8s.Kubeconfig = writeKubeconfigForServer(t, "k3d-k3s", server.URL)
	cfg.Runtime.Profile = model.RuntimeProfileFirecracker
	cfg.Runtime.ClassName = "sandbox-runner-microvm"

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatal("Probe() OK = false, want true")
	}
	if got := result.Details["runtime_class"]; got != "sandbox-runner-microvm" {
		t.Fatalf("details[runtime_class] = %#v, want sandbox-runner-microvm", got)
	}
}

func writeKubeconfig(t *testing.T, contextName string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	data := []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: demo\n  cluster:\n    server: https://127.0.0.1:6443\ncontexts:\n- name: " + contextName + "\n  context:\n    cluster: demo\n    user: demo\ncurrent-context: " + contextName + "\nusers:\n- name: demo\n  user:\n    token: demo-token\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeKubeconfigForServer(t *testing.T, contextName string, serverURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	data := []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: demo\n  cluster:\n    server: " + serverURL + "\n    insecure-skip-tls-verify: true\ncontexts:\n- name: " + contextName + "\n  context:\n    cluster: demo\n    user: demo\ncurrent-context: " + contextName + "\nusers:\n- name: demo\n  user:\n    token: demo-token\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
