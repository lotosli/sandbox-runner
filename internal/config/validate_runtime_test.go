package config

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestValidateRunConfigRejectsUnsupportedRuntimeCombinations(t *testing.T) {
	tests := []struct {
		name string
		cfg  model.RunConfig
	}{
		{
			name: "direct kata",
			cfg: func() model.RunConfig {
				cfg := DefaultRunConfig()
				cfg.Runtime.Profile = model.RuntimeProfileKata
				cfg.Kata.Enabled = true
				return cfg
			}(),
		},
		{
			name: "devcontainer kata",
			cfg: func() model.RunConfig {
				cfg := DefaultRunConfig()
				cfg.Platform.RunMode = model.RunModeLocalDevContainer
				cfg.Backend.Kind = model.BackendKindDevContainer
				cfg.Run.SandboxID = ""
				cfg.Runtime.Profile = model.RuntimeProfileKata
				cfg.Kata.Enabled = true
				return cfg
			}(),
		},
		{
			name: "docker kata",
			cfg: func() model.RunConfig {
				cfg := DefaultRunConfig()
				cfg.Platform.RunMode = model.RunModeLocalDocker
				cfg.Backend.Kind = model.BackendKindDocker
				cfg.Run.Image = "alpine:3.20"
				cfg.Runtime.Profile = model.RuntimeProfileKata
				cfg.Kata.Enabled = true
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRunConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateRunConfig() error = nil, want unsupported runtime profile error")
			}
		})
	}
}

func TestValidateRunConfigAllowsDevContainerNative(t *testing.T) {
	cfg := DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.Run.SandboxID = ""
	cfg.Runtime.Profile = model.RuntimeProfileNative

	if err := ValidateRunConfig(cfg); err != nil {
		t.Fatalf("ValidateRunConfig() error = %v", err)
	}
}

func TestValidateRunConfigAllowsAppleContainerOnDarwinArm64Target(t *testing.T) {
	cfg := DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalAppleContainer
	cfg.Platform.TargetOS = "darwin"
	cfg.Platform.TargetArch = "arm64"
	cfg.Backend.Kind = model.BackendKindAppleContainer
	cfg.Runtime.Profile = model.RuntimeProfileAppleContainer
	cfg.Run.SandboxID = ""
	cfg.Run.Image = "ghcr.io/apple/container:latest"

	if err := ValidateRunConfig(cfg); err != nil {
		t.Fatalf("ValidateRunConfig() error = %v", err)
	}
}

func TestValidateRunConfigAllowsOrbStackProfiles(t *testing.T) {
	dockerCfg := DefaultRunConfig()
	dockerCfg.Platform.RunMode = model.RunModeLocalDocker
	dockerCfg.Backend.Kind = model.BackendKindDocker
	dockerCfg.Docker.Provider = model.DockerProviderOrbStack
	dockerCfg.Runtime.Profile = model.RuntimeProfileOrbStackDocker
	dockerCfg.Run.Image = "alpine:3.20"
	if err := ValidateRunConfig(dockerCfg); err != nil {
		t.Fatalf("ValidateRunConfig(dockerCfg) error = %v", err)
	}

	k8sCfg := DefaultRunConfig()
	k8sCfg.Platform.RunMode = model.RunModeSTGLinux
	k8sCfg.Backend.Kind = model.BackendKindK8s
	k8sCfg.K8s.Provider = model.K8sProviderOrbStackLocal
	k8sCfg.Runtime.Profile = model.RuntimeProfileOrbStackK8s
	if err := ValidateRunConfig(k8sCfg); err != nil {
		t.Fatalf("ValidateRunConfig(k8sCfg) error = %v", err)
	}
}

func TestValidateRunConfigAllowsMicroVMAliasForK8s(t *testing.T) {
	cfg := DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderK3s,
		RuntimeProfile: model.ExecutionRuntimeProfile("microvm"),
	}
	cfg.K8s.Provider = model.K8sProviderK3s
	cfg.Runtime.Profile = model.RuntimeProfile("microvm")
	cfg.Runtime.ClassName = "sandbox-runner-microvm"

	if err := ValidateRunConfig(cfg); err != nil {
		t.Fatalf("ValidateRunConfig() error = %v", err)
	}
}
