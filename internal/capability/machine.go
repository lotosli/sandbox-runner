package capability

import (
	"context"
	"os/exec"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func probeMachine(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	binary := fullConfig.OrbStack.OrbCtlBinary
	if binary == "" {
		binary = fullConfig.OrbStack.OrbBinary
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "machine provider CLI not found: %v", err)
	}
	if fullConfig.OrbStack.MachineName == "" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "machine name is required")
	}
	return okResult(map[string]any{
		"binary":       path,
		"machine_name": fullConfig.OrbStack.MachineName,
		"distro":       fullConfig.OrbStack.MachineDistro,
	}), nil
}
