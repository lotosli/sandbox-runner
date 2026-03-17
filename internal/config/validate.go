package config

import (
	"errors"
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func ValidateRunConfig(cfg model.RunConfig) error {
	if cfg.Run.ServiceName == "" {
		return errors.New("run.service_name is required")
	}
	if cfg.Run.Attempt <= 0 {
		return errors.New("run.attempt must be positive")
	}
	if cfg.Run.SandboxID == "" && cfg.Backend.Kind != model.BackendKindOpenSandbox {
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
	case model.RunModeLocalDirect, model.RunModeLocalDocker, model.RunModeSTGLinux, model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
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
	case model.BackendKindDirect, model.BackendKindDocker, model.BackendKindK8s, model.BackendKindOpenSandbox:
	default:
		return fmt.Errorf("unsupported backend.kind: %s", cfg.Backend.Kind)
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		if cfg.OpenSandbox.BaseURL == "" {
			return errors.New("opensandbox.base_url is required for opensandbox backend")
		}
		if cfg.Sandbox.Image == "" && cfg.Run.Image == "" {
			return errors.New("sandbox.image or run.image is required for opensandbox backend")
		}
	}
	return nil
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
