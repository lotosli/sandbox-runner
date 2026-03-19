package capability

import (
	"context"
	"os"

	"github.com/lotosli/sandbox-runner/internal/kubernetes"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/platform"
)

func probeK8s(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	kubeconfig, kubeconfigMode := probeKubeconfig(fullConfig)
	if kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "kubeconfig not accessible: %v", err)
		}
	}
	if model.RequiresRuntimeClass(cfg.RuntimeProfile) {
		runtimeClass := model.RuntimeClassNameForConfig(fullConfig)
		if runtimeClass == "" {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityRuntimeUnavailable, cfg, "runtime profile %s requires runtime class or equivalent cluster support", cfg.RuntimeProfile)
		}
		client, err := kubernetes.NewClient(kubeconfig, fullConfig.K8s.Context)
		if err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "kubernetes provider config is not usable: %v", err)
		}
		if err := client.ReadRuntimeClass(ctx, runtimeClass); err != nil {
			return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityRuntimeUnavailable, cfg, "runtime class %s is not available: %v", runtimeClass, err)
		}
		return okResult(map[string]any{
			"provider":        cfg.Provider,
			"kubeconfig":      kubeconfigDetail(kubeconfig, kubeconfigMode),
			"kube_context":    fullConfig.K8s.Context,
			"namespace":       fullConfig.K8s.Namespace,
			"runtime_class":   runtimeClass,
			"runtime_profile": cfg.RuntimeProfile,
		}), nil
	}
	if _, err := kubernetes.RESTConfig(kubeconfig, fullConfig.K8s.Context); err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "kubernetes provider config is not usable: %v", err)
	}
	return okResult(map[string]any{
		"provider":     cfg.Provider,
		"kubeconfig":   kubeconfigDetail(kubeconfig, kubeconfigMode),
		"kube_context": fullConfig.K8s.Context,
		"namespace":    fullConfig.K8s.Namespace,
	}), nil
}

func probeKubeconfig(fullConfig model.RunConfig) (string, string) {
	if explicit := fullConfig.K8s.Kubeconfig; explicit != "" {
		return platform.KubeconfigPath(explicit), "explicit"
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "", "in-cluster"
	}
	if os.Getenv("KUBECONFIG") != "" {
		return platform.KubeconfigPath(""), "env"
	}
	return "", "in-cluster"
}

func kubeconfigDetail(kubeconfig string, mode string) string {
	if kubeconfig != "" {
		return kubeconfig
	}
	return mode
}
