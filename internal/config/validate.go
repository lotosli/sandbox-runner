package config

import (
	"fmt"
	"runtime"

	"github.com/lotosli/sandbox-runner/internal/compat"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func ValidateRunConfig(cfg model.RunConfig) error {
	cfg = normalizeExecutionConfig(cfg)
	if cfg.Run.ServiceName == "" {
		return invalidSchema("run.service_name is required")
	}
	if cfg.Run.Attempt <= 0 {
		return invalidSchema("run.attempt must be positive")
	}
	if cfg.Run.SandboxID == "" && !backendCreatesSandbox(cfg.Backend.Kind) {
		return invalidSchema("run.sandbox_id is required")
	}
	if cfg.Run.WorkspaceDir == "" {
		return invalidSchema("run.workspace_dir is required")
	}
	if cfg.Run.ArtifactDir == "" {
		return invalidSchema("run.artifact_dir is required")
	}
	if cfg.Run.Language == "" {
		return invalidSchema("run.language is required")
	}
	if cfg.Execution.Backend == "" {
		return invalidSchema("execution.backend is required")
	}
	switch cfg.Execution.Backend {
	case model.ExecutionBackendDirect, model.ExecutionBackendDocker, model.ExecutionBackendK8s, model.ExecutionBackendOpenSandbox, model.ExecutionBackendDevContainer, model.ExecutionBackendAppleContainer, model.ExecutionBackendMachine:
	default:
		return invalidSchema(fmt.Sprintf("unsupported execution.backend: %s", cfg.Execution.Backend))
	}
	if cfg.Execution.Provider == "" {
		return invalidSchema("execution.provider is required")
	}
	switch cfg.Execution.Provider {
	case model.ProviderNative, model.ProviderOrbStack, model.ProviderKindKind, model.ProviderMinikube, model.ProviderDockerDesktop, model.ProviderColima, model.ProviderGKE, model.ProviderEKS, model.ProviderAKS, model.ProviderOpenSandbox:
	default:
		return invalidSchema(fmt.Sprintf("unsupported execution.provider: %s", cfg.Execution.Provider))
	}
	if cfg.Execution.RuntimeProfile == "" {
		return invalidSchema("execution.runtime_profile is required")
	}
	switch cfg.Execution.RuntimeProfile {
	case model.ExecutionRuntimeProfileDefault, model.ExecutionRuntimeProfileKata, model.ExecutionRuntimeProfileGVisor, model.ExecutionRuntimeProfileFirecracker:
	default:
		return invalidSchema(fmt.Sprintf("unsupported execution.runtime_profile: %s", cfg.Execution.RuntimeProfile))
	}
	if cfg.Platform.RunMode == "" {
		return invalidSchema("platform.run_mode is required")
	}
	switch cfg.Platform.RunMode {
	case model.RunModeLocalDirect, model.RunModeLocalDocker, model.RunModeLocalDevContainer, model.RunModeLocalAppleContainer, model.RunModeLocalOrbStackMachine, model.RunModeSTGLinux, model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
	default:
		return invalidSchema(fmt.Sprintf("unsupported platform.run_mode: %s", cfg.Platform.RunMode))
	}
	if cfg.Platform.RunMode == model.RunModeLocalDocker && cfg.Run.Image == "" {
		return invalidSchema("run.image is required for local_docker mode")
	}
	if cfg.Backend.Kind == "" {
		return invalidSchema("backend.kind is required")
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDirect, model.BackendKindDocker, model.BackendKindDevContainer, model.BackendKindAppleContainer, model.BackendKindOrbStackMachine, model.BackendKindK8s, model.BackendKindOpenSandbox:
	default:
		return invalidSchema(fmt.Sprintf("unsupported backend.kind: %s", cfg.Backend.Kind))
	}
	if cfg.Runtime.Profile == "" {
		return invalidSchema("runtime.profile is required")
	}
	switch cfg.Runtime.Profile {
	case model.RuntimeProfileDefault, model.RuntimeProfileNative, model.RuntimeProfileKata, model.RuntimeProfileGVisor, model.RuntimeProfileFirecracker, model.RuntimeProfileAppleContainer, model.RuntimeProfileOrbStackDocker, model.RuntimeProfileOrbStackK8s, model.RuntimeProfileOrbStackMachine:
	default:
		return invalidSchema(fmt.Sprintf("unsupported runtime.profile: %s", cfg.Runtime.Profile))
	}
	if err := validateBackendRunMode(cfg); err != nil {
		return err
	}
	if err := validateExecutionRuntime(cfg); err != nil {
		return err
	}
	compatibility := compat.ValidateCompatibility(cfg.Execution)
	if compatibility.Level == model.SupportUnsupported {
		return model.RunnerError{
			Code:        string(model.ErrorCodeUnsupportedExecutionCombo),
			Message:     compatibility.Message,
			BackendKind: string(cfg.Execution.Backend),
		}
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		if cfg.OpenSandbox.BaseURL == "" {
			return invalidSchema("opensandbox.base_url is required for opensandbox backend")
		}
		if cfg.Sandbox.Image == "" && cfg.Run.Image == "" {
			return invalidSchema("sandbox.image or run.image is required for opensandbox backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindDevContainer {
		if cfg.DevContainer.CLIPath == "" {
			return invalidSchema("devcontainer.cli_path is required for devcontainer backend")
		}
		if cfg.DevContainer.WorkspaceFolder == "" && cfg.Run.WorkspaceDir == "" {
			return invalidSchema("devcontainer.workspace_folder or run.workspace_dir is required for devcontainer backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindAppleContainer {
		if cfg.AppleContainer.Binary == "" {
			return invalidSchema("apple_container.binary is required for apple-container backend")
		}
		if cfg.Run.Image == "" && cfg.AppleContainer.Image == "" {
			return invalidSchema("run.image or apple_container.image is required for apple-container backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		if cfg.OrbStack.OrbCtlBinary == "" && cfg.OrbStack.OrbBinary == "" {
			return invalidSchema("orbstack.orbctl_binary or orbstack.orb_binary is required for machine backend")
		}
		if cfg.OrbStack.MachineName == "" {
			return invalidSchema("orbstack.machine_name is required for machine backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindDocker {
		switch cfg.Docker.Provider {
		case model.DockerProviderDocker, model.DockerProviderOrbStack:
		default:
			return invalidSchema(fmt.Sprintf("unsupported docker.provider: %s", cfg.Docker.Provider))
		}
	}
	if cfg.Backend.Kind == model.BackendKindK8s {
		switch cfg.K8s.Provider {
		case model.K8sProviderRemote, model.K8sProviderOrbStackLocal:
		default:
			return invalidSchema(fmt.Sprintf("unsupported k8s.provider: %s", cfg.K8s.Provider))
		}
	}
	return nil
}

func backendCreatesSandbox(kind model.BackendKind) bool {
	switch kind {
	case model.BackendKindOpenSandbox, model.BackendKindDevContainer, model.BackendKindAppleContainer, model.BackendKindOrbStackMachine:
		return true
	default:
		return false
	}
}

func ValidatePolicyConfig(cfg model.PolicyConfig) error {
	if len(cfg.Tools.Allow) == 0 {
		return invalidSchema("tools.allow must not be empty")
	}
	if len(cfg.Tools.DenyPatterns) == 0 {
		return invalidSchema("tools.deny_patterns must not be empty")
	}
	if cfg.Resources.TimeoutSecDefault <= 0 {
		return invalidSchema("resources.timeout_sec_default must be positive")
	}
	return nil
}

func validateBackendRunMode(cfg model.RunConfig) error {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		if cfg.Platform.RunMode != model.RunModeLocalDirect {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDirect))
		}
	case model.BackendKindDocker:
		if cfg.Platform.RunMode != model.RunModeLocalDocker {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDocker))
		}
	case model.BackendKindDevContainer:
		if cfg.Platform.RunMode != model.RunModeLocalDevContainer {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDevContainer))
		}
	case model.BackendKindK8s:
		if cfg.Platform.RunMode != model.RunModeSTGLinux {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeSTGLinux))
		}
	case model.BackendKindAppleContainer:
		if cfg.Platform.RunMode != model.RunModeLocalAppleContainer {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalAppleContainer))
		}
	case model.BackendKindOrbStackMachine:
		if cfg.Platform.RunMode != model.RunModeLocalOrbStackMachine {
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalOrbStackMachine))
		}
	case model.BackendKindOpenSandbox:
		switch cfg.Platform.RunMode {
		case model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
		default:
			return invalidSchema(fmt.Sprintf("backend.kind=%s requires opensandbox run_mode", cfg.Backend.Kind))
		}
	}
	return nil
}

func validateExecutionRuntime(cfg model.RunConfig) error {
	if cfg.Execution.RuntimeProfile == model.ExecutionRuntimeProfileKata && (cfg.Execution.Backend == model.ExecutionBackendK8s || cfg.Execution.Backend == model.ExecutionBackendOpenSandbox) && cfg.Kata.RuntimeClassName == "" {
		return model.RunnerError{
			Code:        string(model.ErrorCodeKataRuntimeClassNotFound),
			Message:     "kata.runtime_class_name is required when execution.runtime_profile=kata",
			BackendKind: string(cfg.Execution.Backend),
		}
	}
	if cfg.Execution.Backend == model.ExecutionBackendAppleContainer && !isAppleContainerTarget(cfg.Platform.TargetOS, cfg.Platform.TargetArch) {
		return model.RunnerError{
			Code:        string(model.ErrorCodeAppleContainerUnsupported),
			Message:     "apple-container backend is supported only on darwin/arm64",
			BackendKind: string(cfg.Execution.Backend),
		}
	}
	if cfg.Execution.Backend == model.ExecutionBackendMachine && !isDarwinTarget(cfg.Platform.TargetOS) {
		return model.RunnerError{
			Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
			Message:     "machine backend is currently supported only on macOS",
			BackendKind: string(cfg.Execution.Backend),
		}
	}
	return nil
}

func invalidSchema(message string) error {
	return model.RunnerError{
		Code:    string(model.ErrorCodeConfigInvalidSchema),
		Message: message,
	}
}

func isAppleContainerTarget(targetOS, targetArch string) bool {
	osName := targetOS
	archName := targetArch
	if osName == "" || osName == "auto" {
		osName = runtime.GOOS
	}
	if archName == "" || archName == "auto" {
		archName = runtime.GOARCH
	}
	return osName == "darwin" && archName == "arm64"
}

func isDarwinTarget(targetOS string) bool {
	if targetOS == "" || targetOS == "auto" {
		return runtime.GOOS == "darwin"
	}
	return targetOS == "darwin"
}
