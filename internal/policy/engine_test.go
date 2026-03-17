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
	runCfg.Run.ArtifactDir = "/tmp/workspace/.sandbox-runner"
	engine := NewEngine(config.DefaultPolicyConfig(), runCfg)
	if err := engine.CheckPathRead(context.Background(), model.PhasePrepare, "/tmp/workspace/project/main.go"); err != nil {
		t.Fatalf("expected workspace path to be allowed, got %v", err)
	}
}

func TestCheckPathLocalDockerAllowsHostWorkspace(t *testing.T) {
	runCfg := config.DefaultRunConfig()
	runCfg.Platform.RunMode = model.RunModeLocalDocker
	runCfg.Backend.Kind = model.BackendKindDocker
	runCfg.Run.WorkspaceDir = "/Users/example/repo"
	runCfg.Run.ArtifactDir = "/Users/example/repo/.sandbox-runner"

	engine := NewEngine(config.DefaultPolicyConfig(), runCfg)
	if err := engine.CheckPathWrite(context.Background(), model.PhasePrepare, "/Users/example/repo/.sandbox-runner/results.json"); err != nil {
		t.Fatalf("expected host artifact path to be allowed for local docker, got %v", err)
	}
}
