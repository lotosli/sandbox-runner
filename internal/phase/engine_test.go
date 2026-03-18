package phase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/artifact"
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
	if _, err := os.Stat(filepath.Join(runCfg.Run.ArtifactDir, artifact.IndexFileName)); err != nil {
		t.Fatalf("index.json not found: %v", err)
	}
	resultsData, err := os.ReadFile(filepath.Join(runCfg.Run.ArtifactDir, artifact.ResultsFileName))
	if err != nil {
		t.Fatalf("ReadFile(results.json) error = %v", err)
	}
	var persisted model.RunResult
	if err := json.Unmarshal(resultsData, &persisted); err != nil {
		t.Fatalf("Unmarshal(results.json) error = %v", err)
	}
	assertArtifactRefPath(t, persisted.Artifacts, artifact.ResultsFileName)
	assertArtifactRefPath(t, persisted.Artifacts, artifact.IndexFileName)
	assertArtifactRefPath(t, persisted.Artifacts, artifact.ReplayFileName)

	indexData, err := os.ReadFile(filepath.Join(runCfg.Run.ArtifactDir, artifact.IndexFileName))
	if err != nil {
		t.Fatalf("ReadFile(index.json) error = %v", err)
	}
	var index model.ArtifactIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("Unmarshal(index.json) error = %v", err)
	}
	if index.Files["results"] != artifact.ResultsFileName {
		t.Fatalf("index results path = %q, want %q", index.Files["results"], artifact.ResultsFileName)
	}
	if len(index.SuggestedReadOrder) == 0 || index.SuggestedReadOrder[0] != "index" {
		t.Fatalf("read order = %#v, want index first", index.SuggestedReadOrder)
	}
}

func assertArtifactRefPath(t *testing.T, refs []model.ArtifactRef, want string) {
	t.Helper()
	for _, ref := range refs {
		if ref.Path == want {
			return
		}
	}
	t.Fatalf("artifact %q not found in refs: %#v", want, refs)
}
