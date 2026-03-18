package capability

import (
	"context"
	"os"

	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/proc"
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
	if command != "" {
		if _, err := proc.ResolveCommandPath(command, fullConfig.Run.WorkspaceDir, probeEnv(fullConfig)); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "command %q is not executable on this host: %v", command, err)
		}
	}

	return okResult(map[string]any{
		"workspace_dir": fullConfig.Run.WorkspaceDir,
		"artifact_dir":  fullConfig.Run.ArtifactDir,
		"command":       command,
	}), nil
}

func probeEnv(cfg model.RunConfig) map[string]string {
	env := map[string]string{}
	for k, v := range cfg.Run.ExtraEnv {
		env[k] = v
	}
	for k, v := range cfg.Go.ExtraEnv {
		env[k] = v
	}
	return env
}
