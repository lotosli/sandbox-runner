package capability

import (
	"context"
	"os"

	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/platform"
)

func probeK8s(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	_ = ctx
	kubeconfig := platform.KubeconfigPath(fullConfig.K8s.Kubeconfig)
	if kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "kubeconfig not accessible: %v", err)
		}
	}
	if cfg.RuntimeProfile != model.ExecutionRuntimeProfileDefault {
		runtimeClass := fullConfig.Kata.RuntimeClassName
		if runtimeClass == "" {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityRuntimeUnavailable, cfg, "runtime profile %s requires runtime class or equivalent cluster support", cfg.RuntimeProfile)
		}
		return okResult(map[string]any{
			"kubeconfig":      kubeconfig,
			"kube_context":    fullConfig.K8s.Context,
			"namespace":       fullConfig.K8s.Namespace,
			"runtime_class":   runtimeClass,
			"runtime_profile": cfg.RuntimeProfile,
		}, "runtime class presence is inferred from configuration; live cluster verification is not implemented yet"), nil
	}
	return okResult(map[string]any{
		"kubeconfig":   kubeconfig,
		"kube_context": fullConfig.K8s.Context,
		"namespace":    fullConfig.K8s.Namespace,
	}), nil
}
