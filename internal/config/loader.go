package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lotosli/sandbox-runner/internal/model"
	"gopkg.in/yaml.v3"
)

func LoadRunConfig(path string) (model.RunConfig, error) {
	cfg := DefaultRunConfig()
	if path == "" {
		cfg = applyRunEnvOverrides(cfg)
		cfg = normalizeRunConfig(cfg)
		return cfg, ValidateRunConfig(cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return model.RunConfig{}, fmt.Errorf("read run config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return model.RunConfig{}, fmt.Errorf("parse run config: %w", err)
	}
	cfg = applyRunEnvOverrides(cfg)
	cfg = normalizeRunConfig(cfg)
	return cfg, ValidateRunConfig(cfg)
}

func LoadPolicyConfig(path string) (model.PolicyConfig, error) {
	cfg := DefaultPolicyConfig()
	if path == "" {
		return cfg, ValidatePolicyConfig(cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return model.PolicyConfig{}, fmt.Errorf("read policy config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return model.PolicyConfig{}, fmt.Errorf("parse policy config: %w", err)
	}
	return cfg, ValidatePolicyConfig(cfg)
}

func applyRunEnvOverrides(cfg model.RunConfig) model.RunConfig {
	if v := os.Getenv("RUN_ID"); v != "" {
		cfg.Run.RunID = v
	}
	if v := os.Getenv("ATTEMPT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Run.Attempt)
	}
	if v := os.Getenv("SANDBOX_ID"); v != "" {
		cfg.Run.SandboxID = v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.Run.OTLPEndpoint = v
	}
	if v := os.Getenv("SANDBOX_RUN_MODE"); v != "" {
		cfg.Platform.RunMode = model.RunMode(v)
	}
	if v := os.Getenv("SANDBOX_BACKEND_KIND"); v != "" {
		cfg.Backend.Kind = model.BackendKind(v)
	}
	if v := os.Getenv("SANDBOX_RUNTIME_PROFILE"); v != "" {
		cfg.Runtime.Profile = model.RuntimeProfile(v)
	}
	if v := os.Getenv("DOCKER_PROVIDER"); v != "" {
		cfg.Docker.Provider = model.DockerProvider(v)
	}
	if v := os.Getenv("DOCKER_CONTEXT"); v != "" {
		cfg.Docker.Context = v
	}
	if v := os.Getenv("K8S_PROVIDER"); v != "" {
		cfg.K8s.Provider = model.K8sProvider(v)
	}
	if v := os.Getenv("KUBECONFIG"); v != "" {
		cfg.K8s.Kubeconfig = v
	}
	if v := os.Getenv("K8S_CONTEXT"); v != "" {
		cfg.K8s.Context = v
	}
	if v := os.Getenv("OPENSANDBOX_BASE_URL"); v != "" {
		cfg.OpenSandbox.BaseURL = v
	}
	if v := os.Getenv("OPENSANDBOX_API_KEY"); v != "" {
		cfg.OpenSandbox.APIKey = v
	}
	if v := os.Getenv("OPENSANDBOX_RUNTIME"); v != "" {
		cfg.OpenSandbox.Runtime = model.OpenSandboxRuntime(v)
	}
	if v := os.Getenv("OPENSANDBOX_NETWORK_MODE"); v != "" {
		cfg.OpenSandbox.NetworkMode = v
	}
	if v := os.Getenv("DEVCONTAINER_CLI_PATH"); v != "" {
		cfg.DevContainer.CLIPath = v
	}
	if v := os.Getenv("APPLE_CONTAINER_BINARY"); v != "" {
		cfg.AppleContainer.Binary = v
	}
	if v := os.Getenv("ORBSTACK_ORB_BINARY"); v != "" {
		cfg.OrbStack.OrbBinary = v
	}
	if v := os.Getenv("ORBSTACK_ORBCTL_BINARY"); v != "" {
		cfg.OrbStack.OrbCtlBinary = v
	}
	return cfg
}

func normalizeRunConfig(cfg model.RunConfig) model.RunConfig {
	cfg.Run.WorkspaceDir = cleanPath(cfg.Run.WorkspaceDir)
	cfg.Run.ArtifactDir = cleanPath(cfg.Run.ArtifactDir)
	cfg.DevContainer.ConfigPath = cleanPath(cfg.DevContainer.ConfigPath)
	cfg.DevContainer.WorkspaceFolder = cleanPath(cfg.DevContainer.WorkspaceFolder)
	cfg.K8s.Kubeconfig = cleanPath(cfg.K8s.Kubeconfig)
	inferredKind := inferBackendKind(cfg.Platform.RunMode)
	if cfg.Backend.Kind == "" || cfg.Backend.Kind == model.BackendKindDirect && inferredKind != model.BackendKindDirect {
		cfg.Backend.Kind = inferredKind
	}
	if cfg.Platform.RunMode == model.RunModeLocalDocker && cfg.Backend.Kind == model.BackendKindDirect {
		cfg.Backend.Kind = inferredKind
	}
	if cfg.Platform.RunMode == model.RunModeSTGLinux && cfg.Backend.Kind == model.BackendKindDirect {
		cfg.Backend.Kind = inferBackendKind(cfg.Platform.RunMode)
	}
	if cfg.Runtime.Profile == "" {
		if cfg.Kata.Enabled {
			cfg.Runtime.Profile = model.RuntimeProfileKata
		} else {
			cfg.Runtime.Profile = model.RuntimeProfileNative
		}
	}
	if cfg.Backend.Kind == model.BackendKindAppleContainer && cfg.Runtime.Profile == model.RuntimeProfileNative {
		cfg.Runtime.Profile = model.RuntimeProfileAppleContainer
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine && cfg.Runtime.Profile == model.RuntimeProfileNative {
		cfg.Runtime.Profile = model.RuntimeProfileOrbStackMachine
	}
	if cfg.Backend.Kind == model.BackendKindDocker && cfg.Docker.Provider == model.DockerProviderOrbStack && cfg.Runtime.Profile == model.RuntimeProfileNative {
		cfg.Runtime.Profile = model.RuntimeProfileOrbStackDocker
	}
	if cfg.Backend.Kind == model.BackendKindK8s && cfg.K8s.Provider == model.K8sProviderOrbStackLocal && cfg.Runtime.Profile == model.RuntimeProfileNative {
		cfg.Runtime.Profile = model.RuntimeProfileOrbStackK8s
	}
	if cfg.Runtime.Profile == model.RuntimeProfileKata {
		cfg.Kata.Enabled = true
	}
	if cfg.DevContainer.WorkspaceFolder == "" {
		cfg.DevContainer.WorkspaceFolder = cfg.Run.WorkspaceDir
	}
	if cfg.AppleContainer.WorkspaceRoot == "" {
		cfg.AppleContainer.WorkspaceRoot = "/workspace"
	}
	if cfg.OrbStack.MachineWorkspaceRoot == "" {
		cfg.OrbStack.MachineWorkspaceRoot = cfg.Run.WorkspaceDir
	}
	if cfg.K8s.Namespace == "" {
		cfg.K8s.Namespace = "ai-sandbox-runner-runs"
	}
	if cfg.Docker.Provider == "" {
		cfg.Docker.Provider = model.DockerProviderDocker
	}
	if cfg.Docker.Binary == "" {
		cfg.Docker.Binary = "docker"
	}
	if cfg.Docker.ComposeBinary == "" {
		cfg.Docker.ComposeBinary = "docker"
	}
	if cfg.K8s.Provider == "" {
		cfg.K8s.Provider = model.K8sProviderRemote
	}
	if cfg.Docker.Provider == model.DockerProviderOrbStack {
		if cfg.OrbStack.DockerContext == "" {
			cfg.OrbStack.DockerContext = "orbstack"
		}
		if cfg.Docker.Context == "" || cfg.Docker.Context == "default" {
			cfg.Docker.Context = cfg.OrbStack.DockerContext
		}
	}
	if cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
		if cfg.OrbStack.KubeContext == "" {
			cfg.OrbStack.KubeContext = "orbstack"
		}
		if cfg.K8s.Context == "" {
			cfg.K8s.Context = cfg.OrbStack.KubeContext
		}
	}
	if cfg.Sandbox.Image == "" {
		cfg.Sandbox.Image = cfg.Run.Image
	}
	if cfg.Run.Image == "" {
		cfg.Run.Image = cfg.Sandbox.Image
	}
	if cfg.Backend.Kind == model.BackendKindAppleContainer && cfg.Run.Image == "" {
		cfg.Run.Image = cfg.AppleContainer.Image
		cfg.Sandbox.Image = cfg.AppleContainer.Image
	}
	if cfg.OpenSandbox.WorkspaceRoot == "" {
		cfg.OpenSandbox.WorkspaceRoot = "/workspace"
	}
	return cfg
}

func inferBackendKind(mode model.RunMode) model.BackendKind {
	switch mode {
	case model.RunModeLocalDocker:
		return model.BackendKindDocker
	case model.RunModeLocalDevContainer:
		return model.BackendKindDevContainer
	case model.RunModeLocalAppleContainer:
		return model.BackendKindAppleContainer
	case model.RunModeLocalOrbStackMachine:
		return model.BackendKindOrbStackMachine
	case model.RunModeSTGLinux:
		return model.BackendKindK8s
	case model.RunModeLocalOpenSandboxDocker, model.RunModeSTGOpenSandboxK8s:
		return model.BackendKindOpenSandbox
	default:
		return model.BackendKindDirect
	}
}

func cleanPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return filepath.Clean(path)
}
