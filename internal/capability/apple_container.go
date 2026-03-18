package capability

import (
	"context"
	"os/exec"
	"runtime"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func probeAppleContainer(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	if runtime.GOOS != "darwin" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple-container backend requires macOS")
	}
	path, err := exec.LookPath(fullConfig.AppleContainer.Binary)
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple container CLI not found: %v", err)
	}
	image := fullConfig.AppleContainer.Image
	if image == "" {
		image = fullConfig.Run.Image
	}
	return okResult(map[string]any{
		"binary": path,
		"image":  image,
	}), nil
}
