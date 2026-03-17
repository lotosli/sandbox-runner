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
	return cfg
}

func normalizeRunConfig(cfg model.RunConfig) model.RunConfig {
	cfg.Run.WorkspaceDir = cleanPath(cfg.Run.WorkspaceDir)
	cfg.Run.ArtifactDir = cleanPath(cfg.Run.ArtifactDir)
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
	if cfg.Sandbox.Image == "" {
		cfg.Sandbox.Image = cfg.Run.Image
	}
	if cfg.Run.Image == "" {
		cfg.Run.Image = cfg.Sandbox.Image
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
