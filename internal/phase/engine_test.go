package phase

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestEngineRunLocalDirect(t *testing.T) {
	workspace := t.TempDir()
	runCfg := config.DefaultRunConfig()
	runCfg.Run.WorkspaceDir = workspace
	runCfg.Run.ArtifactDir = filepath.Join(workspace, ".sandbox-runner")
	runCfg.Run.Command = []string{"sh", "-lc", "echo hello"}
	runCfg.Collector.Mode = model.CollectorModeSkip
	runCfg.Phases.Setup.Enabled = false
	runCfg.Phases.Verify.Enabled = false

	req := &model.RunRequest{
		RunConfig: runCfg,
		Policy:    config.DefaultPolicyConfig(),
	}

	engine := NewEngine()
	result, err := engine.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != model.StatusSucceeded {
		t.Fatalf("status = %s, want %s", result.Status, model.StatusSucceeded)
	}
	if _, err := os.Stat(filepath.Join(runCfg.Run.ArtifactDir, "results.json")); err != nil {
		t.Fatalf("results.json not found: %v", err)
	}
}
