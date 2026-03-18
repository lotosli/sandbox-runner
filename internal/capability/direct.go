package capability

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func probeDirect(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	if _, err := os.Stat(fullConfig.Run.WorkspaceDir); err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "workspace not accessible: %v", err)
	}
	probeDir := fullConfig.Run.ArtifactDir
	if probeDir == "" {
		probeDir = os.TempDir()
	}
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "artifact dir not writable: %v", err)
	}
	f, err := os.CreateTemp(probeDir, ".probe-*")
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "temp dir not writable: %v", err)
	}
	_ = f.Close()
	_ = os.Remove(f.Name())

	command := ""
	if len(fullConfig.Run.Command) > 0 {
		command = fullConfig.Run.Command[0]
	}
	if command != "" && command[0] != '/' && command[0] != '.' {
		if _, err := exec.LookPath(command); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "command %q is not executable on this host: %v", command, err)
		}
	} else if command != "" {
		if _, err := os.Stat(filepath.Clean(command)); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "command path %q is not accessible: %v", command, err)
		}
	}

	return okResult(map[string]any{
		"workspace_dir": fullConfig.Run.WorkspaceDir,
		"artifact_dir":  fullConfig.Run.ArtifactDir,
		"command":       command,
	}), nil
}
