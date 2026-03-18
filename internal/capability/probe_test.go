package capability

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestProbeDirectDefault(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendDirect,
		Provider:       model.ProviderNative,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.Run.WorkspaceDir = t.TempDir()
	cfg.Run.ArtifactDir = filepath.Join(t.TempDir(), ".sandbox-runner")
	cfg.Run.Command = []string{"sh", "-c", "true"}

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("Probe() OK = false, want true")
	}
}

func TestProbeK8sConditionalRuntimeRequiresRuntimeClass(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderNative,
		RuntimeProfile: model.ExecutionRuntimeProfileKata,
	}
	cfg.K8s.Kubeconfig = filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg.K8s.Kubeconfig, []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg.Kata.RuntimeClassName = ""

	_, err := Probe(context.Background(), cfg.Execution, cfg)
	if err == nil {
		t.Fatal("Probe() error = nil, want runtime profile unavailable error")
	}
	var runnerErr model.RunnerError
	if !errors.As(err, &runnerErr) {
		t.Fatalf("Probe() error = %T, want RunnerError", err)
	}
	if runnerErr.Code != string(model.ErrorCodeCapabilityRuntimeUnavailable) {
		t.Fatalf("Probe() code = %s, want %s", runnerErr.Code, model.ErrorCodeCapabilityRuntimeUnavailable)
	}
}
