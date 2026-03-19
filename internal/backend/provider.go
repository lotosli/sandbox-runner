package backend

import "github.com/lotosli/sandbox-runner/internal/model"

func backendProviderForConfig(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "native"
	case model.BackendKindK8s:
		return string(model.ExecutionProviderForK8sProvider(cfg.K8s.Provider))
	case model.BackendKindOpenSandbox:
		return "opensandbox"
	case model.BackendKindDevContainer, model.BackendKindAppleContainer:
		return "native"
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	default:
		return string(cfg.Backend.Kind)
	}
}
