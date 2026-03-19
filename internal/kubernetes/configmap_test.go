package kubernetes

import (
	"strings"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestBuildConfigMapClearsHostKubeconfigForInClusterRunner(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderK3s,
		RuntimeProfile: model.ExecutionRuntimeProfileFirecracker,
	}
	cfg.K8s.Provider = model.K8sProviderK3s
	cfg.K8s.Kubeconfig = "/Users/lotosli/.kube/config"
	cfg.K8s.Context = "k3d-k3s"
	cfg.Run.RunID = "run-k8s"

	cm, err := BuildConfigMap(model.RunRequest{RunConfig: cfg}, "sandbox-system")
	if err != nil {
		t.Fatalf("BuildConfigMap() error = %v", err)
	}
	runYAML := cm.Data["run.yaml"]
	if strings.Contains(runYAML, "/Users/lotosli/.kube/config") {
		t.Fatalf("run.yaml unexpectedly contains host kubeconfig path:\n%s", runYAML)
	}
	if strings.Contains(runYAML, "context: k3d-k3s") {
		t.Fatalf("run.yaml unexpectedly contains host kube context:\n%s", runYAML)
	}
}
