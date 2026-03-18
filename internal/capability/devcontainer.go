package capability

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func probeDevContainer(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	path, err := exec.LookPath(fullConfig.DevContainer.CLIPath)
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "devcontainer CLI not found: %v", err)
	}
	workspace := fullConfig.DevContainer.WorkspaceFolder
	if workspace == "" {
		workspace = fullConfig.Run.WorkspaceDir
	}
	if _, err := os.Stat(workspace); err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "devcontainer workspace not accessible: %v", err)
	}
	configPath := fullConfig.DevContainer.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(workspace, ".devcontainer", "devcontainer.json")
	}
	if _, err := os.Stat(configPath); err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "devcontainer config not accessible: %v", err)
	}
	return okResult(map[string]any{
		"cli_path":         path,
		"workspace_folder": workspace,
		"config_path":      configPath,
	}), nil
}
