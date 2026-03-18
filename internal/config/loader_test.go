package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestLoadRunConfigInfersOpenSandboxBackend(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "run.yaml")
	data := []byte(`
run:
  service_name: test-runner
  attempt: 1
  sandbox_id: local
  workspace_dir: .
  artifact_dir: .sandbox-runner
  language: auto
  image: alpine:3.20
platform:
  run_mode: local_opensandbox_docker
opensandbox:
  base_url: http://127.0.0.1:8080
`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadRunConfig(configPath)
	if err != nil {
		t.Fatalf("LoadRunConfig() error = %v", err)
	}
	if cfg.Backend.Kind != model.BackendKindOpenSandbox {
		t.Fatalf("backend.kind = %s, want %s", cfg.Backend.Kind, model.BackendKindOpenSandbox)
	}
	if cfg.Execution.Backend != model.ExecutionBackendOpenSandbox {
		t.Fatalf("execution.backend = %s, want %s", cfg.Execution.Backend, model.ExecutionBackendOpenSandbox)
	}
	if cfg.Execution.Provider != model.ProviderOpenSandbox {
		t.Fatalf("execution.provider = %s, want %s", cfg.Execution.Provider, model.ProviderOpenSandbox)
	}
	if cfg.Execution.RuntimeProfile != model.ExecutionRuntimeProfileDefault {
		t.Fatalf("execution.runtime_profile = %s, want %s", cfg.Execution.RuntimeProfile, model.ExecutionRuntimeProfileDefault)
	}
	if cfg.Sandbox.Image != "alpine:3.20" {
		t.Fatalf("sandbox.image = %s, want alpine:3.20", cfg.Sandbox.Image)
	}
}

func TestLoadRunConfigResolvesPathsFromConfigDirectory(t *testing.T) {
	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configPath := filepath.Join(configDir, "run.yaml")
	data := []byte(`
run:
  service_name: test-runner
  attempt: 1
  sandbox_id: local
  workspace_dir: ..
  artifact_dir: ../.sandbox-runner
  language: auto
collector:
  local_collector_config: otelcol.local.yaml
devcontainer:
  config_path: ../.devcontainer/devcontainer.json
k8s:
  kubeconfig: ../.kube/config
`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadRunConfig(configPath)
	if err != nil {
		t.Fatalf("LoadRunConfig() error = %v", err)
	}
	if cfg.Run.WorkspaceDir != rootDir {
		t.Fatalf("workspace_dir = %s, want %s", cfg.Run.WorkspaceDir, rootDir)
	}
	wantArtifactDir := filepath.Join(rootDir, ".sandbox-runner")
	if cfg.Run.ArtifactDir != wantArtifactDir {
		t.Fatalf("artifact_dir = %s, want %s", cfg.Run.ArtifactDir, wantArtifactDir)
	}
	wantCollectorConfig := filepath.Join(configDir, "otelcol.local.yaml")
	if cfg.Collector.LocalCollectorConfig != wantCollectorConfig {
		t.Fatalf("local_collector_config = %s, want %s", cfg.Collector.LocalCollectorConfig, wantCollectorConfig)
	}
	wantDevcontainerConfig := filepath.Join(rootDir, ".devcontainer", "devcontainer.json")
	if cfg.DevContainer.ConfigPath != wantDevcontainerConfig {
		t.Fatalf("devcontainer.config_path = %s, want %s", cfg.DevContainer.ConfigPath, wantDevcontainerConfig)
	}
	wantKubeconfig := filepath.Join(rootDir, ".kube", "config")
	if cfg.K8s.Kubeconfig != wantKubeconfig {
		t.Fatalf("k8s.kubeconfig = %s, want %s", cfg.K8s.Kubeconfig, wantKubeconfig)
	}
}
