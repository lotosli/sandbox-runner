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
