package backend

import "github.com/lotosli/sandbox-runner/internal/model"

func backendProviderForConfig(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "docker"
	case model.BackendKindK8s:
		if cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
			return "orbstack"
		}
		return "k8s"
	case model.BackendKindOpenSandbox:
		return "opensandbox"
	case model.BackendKindDevContainer:
		return "devcontainer"
	case model.BackendKindAppleContainer:
		return "apple-container"
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	default:
		return string(cfg.Backend.Kind)
	}
}
