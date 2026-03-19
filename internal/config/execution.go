package config

import "github.com/lotosli/sandbox-runner/internal/model"

func normalizeExecutionConfig(cfg model.RunConfig) model.RunConfig {
	if cfg.Execution.Backend == "" {
		cfg.Execution = deriveExecutionConfig(cfg)
	} else {
		if cfg.Execution.Provider == "" {
			cfg.Execution.Provider = defaultProviderForBackend(cfg.Execution.Backend)
		}
		if cfg.Execution.RuntimeProfile == "" {
			cfg.Execution.RuntimeProfile = model.ExecutionRuntimeProfileDefault
		}
	}
	return applyExecutionConfig(cfg)
}

func deriveExecutionConfig(cfg model.RunConfig) model.ExecutionConfig {
	backend := executionBackendFromLegacy(cfg)
	provider := executionProviderFromLegacy(cfg, backend)
	runtimeProfile := executionRuntimeProfileFromLegacy(cfg)
	if provider == "" {
		provider = defaultProviderForBackend(backend)
	}
	if runtimeProfile == "" {
		runtimeProfile = model.ExecutionRuntimeProfileDefault
	}
	return model.ExecutionConfig{
		Backend:        backend,
		Provider:       provider,
		RuntimeProfile: runtimeProfile,
	}
}

func applyExecutionConfig(cfg model.RunConfig) model.RunConfig {
	execCfg := cfg.Execution
	if execCfg.Provider == "" {
		execCfg.Provider = defaultProviderForBackend(execCfg.Backend)
	}
	if execCfg.RuntimeProfile == "" {
		execCfg.RuntimeProfile = model.ExecutionRuntimeProfileDefault
	}
	cfg.Execution = execCfg

	cfg.Backend.Kind = legacyBackendKindForExecution(execCfg.Backend)
	if cfg.Platform.RunMode == "" {
		cfg.Platform.RunMode = inferRunModeFromExecution(cfg)
	}
	cfg.Runtime.Profile = internalRuntimeProfileForExecution(execCfg, cfg)

	switch execCfg.Backend {
	case model.ExecutionBackendDocker:
		if execCfg.Provider == model.ProviderOrbStack {
			cfg.Docker.Provider = model.DockerProviderOrbStack
		} else if cfg.Docker.Provider == "" {
			cfg.Docker.Provider = model.DockerProviderDocker
		}
	case model.ExecutionBackendK8s:
		cfg.K8s.Provider = model.LegacyK8sProviderForExecutionProvider(execCfg.Provider)
	}

	return cfg
}

func executionBackendFromLegacy(cfg model.RunConfig) model.ExecutionBackend {
	kind := cfg.Backend.Kind
	inferredKind := inferBackendKind(cfg.Platform.RunMode)
	if kind == "" {
		kind = inferredKind
	}
	if kind == model.BackendKindDirect && inferredKind != model.BackendKindDirect && cfg.Platform.RunMode != "" && cfg.Platform.RunMode != model.RunModeLocalDirect {
		kind = inferredKind
	}
	switch kind {
	case model.BackendKindDirect:
		return model.ExecutionBackendDirect
	case model.BackendKindDocker:
		return model.ExecutionBackendDocker
	case model.BackendKindK8s:
		return model.ExecutionBackendK8s
	case model.BackendKindOpenSandbox:
		return model.ExecutionBackendOpenSandbox
	case model.BackendKindDevContainer:
		return model.ExecutionBackendDevContainer
	case model.BackendKindAppleContainer:
		return model.ExecutionBackendAppleContainer
	case model.BackendKindOrbStackMachine, model.BackendKind("machine"):
		return model.ExecutionBackendMachine
	default:
		return model.ExecutionBackendDirect
	}
}

func executionProviderFromLegacy(cfg model.RunConfig, backend model.ExecutionBackend) model.ProviderKind {
	switch backend {
	case model.ExecutionBackendDirect, model.ExecutionBackendDevContainer, model.ExecutionBackendAppleContainer:
		return model.ProviderNative
	case model.ExecutionBackendDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return model.ProviderOrbStack
		}
		return model.ProviderNative
	case model.ExecutionBackendK8s:
		return model.ExecutionProviderForK8sProvider(cfg.K8s.Provider)
	case model.ExecutionBackendOpenSandbox:
		return model.ProviderOpenSandbox
	case model.ExecutionBackendMachine:
		return model.ProviderOrbStack
	default:
		return ""
	}
}

func executionRuntimeProfileFromLegacy(cfg model.RunConfig) model.ExecutionRuntimeProfile {
	switch cfg.Runtime.Profile {
	case model.RuntimeProfileKata:
		return model.ExecutionRuntimeProfileKata
	case model.RuntimeProfileGVisor:
		return model.ExecutionRuntimeProfileGVisor
	case model.RuntimeProfileFirecracker:
		return model.ExecutionRuntimeProfileFirecracker
	default:
		if cfg.Kata.Enabled {
			return model.ExecutionRuntimeProfileKata
		}
		return model.ExecutionRuntimeProfileDefault
	}
}

func defaultProviderForBackend(backend model.ExecutionBackend) model.ProviderKind {
	switch backend {
	case model.ExecutionBackendDirect, model.ExecutionBackendDevContainer, model.ExecutionBackendAppleContainer:
		return model.ProviderNative
	case model.ExecutionBackendDocker:
		return model.ProviderNative
	case model.ExecutionBackendK8s:
		return model.ProviderNative
	case model.ExecutionBackendOpenSandbox:
		return model.ProviderOpenSandbox
	case model.ExecutionBackendMachine:
		return model.ProviderOrbStack
	default:
		return ""
	}
}

func legacyBackendKindForExecution(backend model.ExecutionBackend) model.BackendKind {
	switch backend {
	case model.ExecutionBackendDirect:
		return model.BackendKindDirect
	case model.ExecutionBackendDocker:
		return model.BackendKindDocker
	case model.ExecutionBackendK8s:
		return model.BackendKindK8s
	case model.ExecutionBackendOpenSandbox:
		return model.BackendKindOpenSandbox
	case model.ExecutionBackendDevContainer:
		return model.BackendKindDevContainer
	case model.ExecutionBackendAppleContainer:
		return model.BackendKindAppleContainer
	case model.ExecutionBackendMachine:
		return model.BackendKindOrbStackMachine
	default:
		return model.BackendKindDirect
	}
}

func inferRunModeFromExecution(cfg model.RunConfig) model.RunMode {
	switch cfg.Execution.Backend {
	case model.ExecutionBackendDirect:
		return model.RunModeLocalDirect
	case model.ExecutionBackendDocker:
		return model.RunModeLocalDocker
	case model.ExecutionBackendDevContainer:
		return model.RunModeLocalDevContainer
	case model.ExecutionBackendAppleContainer:
		return model.RunModeLocalAppleContainer
	case model.ExecutionBackendMachine:
		return model.RunModeLocalOrbStackMachine
	case model.ExecutionBackendK8s:
		return model.RunModeSTGLinux
	case model.ExecutionBackendOpenSandbox:
		if cfg.OpenSandbox.Runtime == model.OpenSandboxRuntimeKubernetes {
			return model.RunModeSTGOpenSandboxK8s
		}
		return model.RunModeLocalOpenSandboxDocker
	default:
		return cfg.Platform.RunMode
	}
}

func internalRuntimeProfileForExecution(execCfg model.ExecutionConfig, cfg model.RunConfig) model.RuntimeProfile {
	switch execCfg.RuntimeProfile {
	case model.ExecutionRuntimeProfileKata:
		return model.RuntimeProfileKata
	case model.ExecutionRuntimeProfileGVisor:
		return model.RuntimeProfileGVisor
	case model.ExecutionRuntimeProfileFirecracker:
		return model.RuntimeProfileFirecracker
	default:
		switch execCfg.Backend {
		case model.ExecutionBackendAppleContainer:
			return model.RuntimeProfileAppleContainer
		case model.ExecutionBackendMachine:
			return model.RuntimeProfileOrbStackMachine
		default:
			return model.RuntimeProfileNative
		}
	}
}
