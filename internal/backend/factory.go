package backend

import (
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func New(resolution model.ExecutionResolution, runCfg model.RunConfig) (SandboxBackend, error) {
	switch resolution.Config.Backend {
	case model.ExecutionBackendDirect:
		return NewLocalBackend(model.BackendKindDirect, runCfg), nil
	case model.ExecutionBackendDocker:
		return NewLocalBackend(model.BackendKindDocker, runCfg), nil
	case model.ExecutionBackendDevContainer:
		return NewDevContainerBackend(runCfg)
	case model.ExecutionBackendAppleContainer:
		return NewAppleContainerBackend(runCfg)
	case model.ExecutionBackendMachine:
		return NewOrbStackMachineBackend(runCfg)
	case model.ExecutionBackendK8s:
		return NewLocalBackend(model.BackendKindK8s, runCfg), nil
	case model.ExecutionBackendOpenSandbox:
		return NewOpenSandboxBackend(runCfg), nil
	default:
		return nil, fmt.Errorf("unsupported backend kind: %s", resolution.Config.Backend)
	}
}
