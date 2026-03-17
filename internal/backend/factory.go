package backend

import (
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func New(runCfg model.RunConfig) (SandboxBackend, error) {
	switch runCfg.Backend.Kind {
	case model.BackendKindDirect:
		return NewLocalBackend(model.BackendKindDirect, runCfg), nil
	case model.BackendKindDocker:
		return NewLocalBackend(model.BackendKindDocker, runCfg), nil
	case model.BackendKindDevContainer:
		return NewDevContainerBackend(runCfg)
	case model.BackendKindAppleContainer:
		return NewAppleContainerBackend(runCfg)
	case model.BackendKindOrbStackMachine:
		return NewOrbStackMachineBackend(runCfg)
	case model.BackendKindK8s:
		return NewLocalBackend(model.BackendKindK8s, runCfg), nil
	case model.BackendKindOpenSandbox:
		return NewOpenSandboxBackend(runCfg), nil
	default:
		return nil, fmt.Errorf("unsupported backend kind: %s", runCfg.Backend.Kind)
	}
}
