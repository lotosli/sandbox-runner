package config

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func ValidateRunConfig(cfg model.RunConfig) error {
	if cfg.Run.ServiceName == "" {
		return errors.New("run.service_name is required")
	}
	if cfg.Run.Attempt <= 0 {
		return errors.New("run.attempt must be positive")
	}
	if cfg.Run.SandboxID == "" && !backendCreatesSandbox(cfg.Backend.Kind) {
		return errors.New("run.sandbox_id is required")
	}
	if cfg.Run.WorkspaceDir == "" {
		return errors.New("run.workspace_dir is required")
	}
	if cfg.Run.ArtifactDir == "" {
		return errors.New("run.artifact_dir is required")
	}
	if cfg.Run.Language == "" {
		return errors.New("run.language is required")
	}
	if cfg.Platform.RunMode == "" {
		return errors.New("platform.run_mode is required")
	}
	switch cfg.Platform.RunMode {
	case model.RunModeLocalDirect, model.RunModeLocalDocker, model.RunModeLocalDevContainer, model.RunModeLocalAppleContainer, model.RunModeLocalOrbStackMachine, model.RunModeSTGLinux, model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
	default:
		return fmt.Errorf("unsupported platform.run_mode: %s", cfg.Platform.RunMode)
	}
	if cfg.Platform.RunMode == model.RunModeLocalDocker && cfg.Run.Image == "" {
		return errors.New("run.image is required for local_docker mode")
	}
	if cfg.Backend.Kind == "" {
		return errors.New("backend.kind is required")
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDirect, model.BackendKindDocker, model.BackendKindDevContainer, model.BackendKindAppleContainer, model.BackendKindOrbStackMachine, model.BackendKindK8s, model.BackendKindOpenSandbox:
	default:
		return fmt.Errorf("unsupported backend.kind: %s", cfg.Backend.Kind)
	}
	if cfg.Runtime.Profile == "" {
		return errors.New("runtime.profile is required")
	}
	switch cfg.Runtime.Profile {
	case model.RuntimeProfileNative, model.RuntimeProfileKata, model.RuntimeProfileAppleContainer, model.RuntimeProfileOrbStackDocker, model.RuntimeProfileOrbStackK8s, model.RuntimeProfileOrbStackMachine:
	default:
		return fmt.Errorf("unsupported runtime.profile: %s", cfg.Runtime.Profile)
	}
	if err := validateBackendRunMode(cfg); err != nil {
		return err
	}
	if err := validateBackendRuntime(cfg); err != nil {
		return err
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		if cfg.OpenSandbox.BaseURL == "" {
			return errors.New("opensandbox.base_url is required for opensandbox backend")
		}
		if cfg.Sandbox.Image == "" && cfg.Run.Image == "" {
			return errors.New("sandbox.image or run.image is required for opensandbox backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindDevContainer {
		if cfg.DevContainer.CLIPath == "" {
			return errors.New("devcontainer.cli_path is required for devcontainer backend")
		}
		if cfg.DevContainer.WorkspaceFolder == "" && cfg.Run.WorkspaceDir == "" {
			return errors.New("devcontainer.workspace_folder or run.workspace_dir is required for devcontainer backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindAppleContainer {
		if cfg.AppleContainer.Binary == "" {
			return errors.New("apple_container.binary is required for apple-container backend")
		}
		if cfg.Run.Image == "" && cfg.AppleContainer.Image == "" {
			return errors.New("run.image or apple_container.image is required for apple-container backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		if cfg.OrbStack.OrbCtlBinary == "" && cfg.OrbStack.OrbBinary == "" {
			return errors.New("orbstack.orbctl_binary or orbstack.orb_binary is required for orbstack-machine backend")
		}
		if cfg.OrbStack.MachineName == "" {
			return errors.New("orbstack.machine_name is required for orbstack-machine backend")
		}
	}
	if cfg.Backend.Kind == model.BackendKindDocker {
		switch cfg.Docker.Provider {
		case model.DockerProviderDocker, model.DockerProviderOrbStack:
		default:
			return fmt.Errorf("unsupported docker.provider: %s", cfg.Docker.Provider)
		}
	}
	if cfg.Backend.Kind == model.BackendKindK8s {
		switch cfg.K8s.Provider {
		case model.K8sProviderRemote, model.K8sProviderOrbStackLocal:
		default:
			return fmt.Errorf("unsupported k8s.provider: %s", cfg.K8s.Provider)
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
		return errors.New("tools.allow must not be empty")
	}
	if len(cfg.Tools.DenyPatterns) == 0 {
		return errors.New("tools.deny_patterns must not be empty")
	}
	if cfg.Resources.TimeoutSecDefault <= 0 {
		return errors.New("resources.timeout_sec_default must be positive")
	}
	return nil
}

func validateBackendRunMode(cfg model.RunConfig) error {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		if cfg.Platform.RunMode != model.RunModeLocalDirect {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDirect)
		}
	case model.BackendKindDocker:
		if cfg.Platform.RunMode != model.RunModeLocalDocker {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDocker)
		}
	case model.BackendKindDevContainer:
		if cfg.Platform.RunMode != model.RunModeLocalDevContainer {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalDevContainer)
		}
	case model.BackendKindK8s:
		if cfg.Platform.RunMode != model.RunModeSTGLinux {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeSTGLinux)
		}
	case model.BackendKindAppleContainer:
		if cfg.Platform.RunMode != model.RunModeLocalAppleContainer {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalAppleContainer)
		}
	case model.BackendKindOrbStackMachine:
		if cfg.Platform.RunMode != model.RunModeLocalOrbStackMachine {
			return fmt.Errorf("backend.kind=%s requires platform.run_mode=%s", cfg.Backend.Kind, model.RunModeLocalOrbStackMachine)
		}
	case model.BackendKindOpenSandbox:
		switch cfg.Platform.RunMode {
		case model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
		default:
			return fmt.Errorf("backend.kind=%s requires opensandbox run_mode", cfg.Backend.Kind)
		}
	}
	return nil
}

func validateBackendRuntime(cfg model.RunConfig) error {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect, model.BackendKindDevContainer:
		if cfg.Runtime.Profile != model.RuntimeProfileNative {
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     fmt.Sprintf("%s backend does not support runtime.profile=%s", cfg.Backend.Kind, cfg.Runtime.Profile),
				BackendKind: string(cfg.Backend.Kind),
			}
		}
	case model.BackendKindDocker:
		switch cfg.Runtime.Profile {
		case model.RuntimeProfileNative:
		case model.RuntimeProfileOrbStackDocker:
			if cfg.Docker.Provider != model.DockerProviderOrbStack {
				return model.RunnerError{
					Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
					Message:     "runtime.profile=orbstack-docker requires docker.provider=orbstack",
					BackendKind: string(cfg.Backend.Kind),
				}
			}
		default:
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     fmt.Sprintf("docker backend does not support runtime.profile=%s", cfg.Runtime.Profile),
				BackendKind: string(cfg.Backend.Kind),
			}
		}
	case model.BackendKindAppleContainer:
		if cfg.Runtime.Profile != model.RuntimeProfileAppleContainer {
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     "apple-container backend requires runtime.profile=apple-container",
				BackendKind: string(cfg.Backend.Kind),
			}
		}
		if !isAppleContainerTarget(cfg.Platform.TargetOS, cfg.Platform.TargetArch) {
			return model.RunnerError{
				Code:        string(model.ErrorCodeAppleContainerUnsupported),
				Message:     "apple-container backend is supported only on darwin/arm64",
				BackendKind: string(cfg.Backend.Kind),
			}
		}
	case model.BackendKindOrbStackMachine:
		if cfg.Runtime.Profile != model.RuntimeProfileOrbStackMachine {
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     "orbstack-machine backend requires runtime.profile=orbstack-machine",
				BackendKind: string(cfg.Backend.Kind),
			}
		}
		if !isDarwinTarget(cfg.Platform.TargetOS) {
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     "orbstack-machine backend is supported only on macOS",
				BackendKind: string(cfg.Backend.Kind),
			}
		}
	case model.BackendKindK8s, model.BackendKindOpenSandbox:
		switch cfg.Runtime.Profile {
		case model.RuntimeProfileNative:
		case model.RuntimeProfileKata:
			if cfg.Runtime.Profile == model.RuntimeProfileKata && cfg.Kata.RuntimeClassName == "" {
				return model.RunnerError{
					Code:        string(model.ErrorCodeKataRuntimeClassNotFound),
					Message:     "kata.runtime_class_name is required when runtime.profile=kata",
					BackendKind: string(cfg.Backend.Kind),
				}
			}
		case model.RuntimeProfileOrbStackK8s:
			if cfg.Backend.Kind != model.BackendKindK8s || cfg.K8s.Provider != model.K8sProviderOrbStackLocal {
				return model.RunnerError{
					Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
					Message:     "runtime.profile=orbstack-k8s requires backend.kind=k8s and k8s.provider=orbstack-local",
					BackendKind: string(cfg.Backend.Kind),
				}
			}
		default:
			return model.RunnerError{
				Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
				Message:     fmt.Sprintf("%s backend does not support runtime.profile=%s", cfg.Backend.Kind, cfg.Runtime.Profile),
				BackendKind: string(cfg.Backend.Kind),
			}
		}
	}
	return nil
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
