package capability

import (
	"context"
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type CapabilityProbe interface {
	Probe(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error)
}

func Probe(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	switch cfg.Backend {
	case model.ExecutionBackendDirect:
		return probeDirect(ctx, cfg, fullConfig)
	case model.ExecutionBackendDocker:
		return probeDocker(ctx, cfg, fullConfig)
	case model.ExecutionBackendK8s:
		return probeK8s(ctx, cfg, fullConfig)
	case model.ExecutionBackendOpenSandbox:
		return probeOpenSandbox(ctx, cfg, fullConfig)
	case model.ExecutionBackendDevContainer:
		return probeDevContainer(ctx, cfg, fullConfig)
	case model.ExecutionBackendAppleContainer:
		return probeAppleContainer(ctx, cfg, fullConfig)
	case model.ExecutionBackendMachine:
		return probeMachine(ctx, cfg, fullConfig)
	default:
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "no capability probe implemented for backend %s", cfg.Backend)
	}
}

func okResult(details map[string]any, warnings ...string) model.CapabilityProbeResult {
	return model.CapabilityProbeResult{
		OK:       true,
		Details:  details,
		Warnings: warnings,
	}
}

func probeFailure(code model.ErrorCode, cfg model.ExecutionConfig, format string, args ...any) error {
	return model.RunnerError{
		Code:        string(code),
		Message:     fmt.Sprintf(format, args...),
		BackendKind: string(cfg.Backend),
	}
}
