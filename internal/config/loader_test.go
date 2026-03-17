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
  artifact_dir: .sandbox-run
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
	if cfg.Sandbox.Image != "alpine:3.20" {
		t.Fatalf("sandbox.image = %s, want alpine:3.20", cfg.Sandbox.Image)
	}
}
