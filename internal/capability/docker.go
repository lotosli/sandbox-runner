package capability

import (
	"context"
	"os/exec"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func probeDocker(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	binary := fullConfig.Docker.Binary
	if binary == "" {
		binary = "docker"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "docker CLI not found: %v", err)
	}
	warnings := []string{}
	if cfg.Provider == model.ProviderOrbStack && fullConfig.OrbStack.OrbBinary == "" && fullConfig.Docker.Context == "" {
		warnings = append(warnings, "orbstack provider selected without explicit orb binary or docker context; relying on default docker environment")
	}
	return okResult(map[string]any{
		"docker_binary": path,
		"provider":      cfg.Provider,
		"context":       fullConfig.Docker.Context,
	}, warnings...), nil
}
