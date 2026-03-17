package platform

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestResolveFeaturesRejectsOBIOnDarwin(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Go.OBIEnabled = true
	target := model.ExecutionTarget{OS: "darwin", Arch: "arm64", Mode: model.RunModeLocalDirect}
	if _, _, err := ResolveFeatures(cfg, target); err == nil {
		t.Fatal("expected OBI enablement on darwin to fail")
	}
}
