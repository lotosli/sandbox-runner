package config

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestNormalizeRunConfigMapsExecutionK8sProviders(t *testing.T) {
	tests := []struct {
		name            string
		provider        model.ProviderKind
		wantK8sProvider model.K8sProvider
		wantK8sContext  string
	}{
		{name: "orbstack", provider: model.ProviderOrbStack, wantK8sProvider: model.K8sProviderOrbStackLocal, wantK8sContext: "orbstack"},
		{name: "minikube", provider: model.ProviderMinikube, wantK8sProvider: model.K8sProviderMinikube, wantK8sContext: "minikube"},
		{name: "k3s", provider: model.ProviderK3s, wantK8sProvider: model.K8sProviderK3s},
		{name: "microk8s", provider: model.ProviderMicroK8s, wantK8sProvider: model.K8sProviderMicroK8s},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultRunConfig()
			cfg.Execution = model.ExecutionConfig{
				Backend:        model.ExecutionBackendK8s,
				Provider:       tt.provider,
				RuntimeProfile: model.ExecutionRuntimeProfileDefault,
			}
			cfg.Platform.RunMode = model.RunModeSTGLinux

			cfg = NormalizeRunConfig(cfg)
			if cfg.Backend.Kind != model.BackendKindK8s {
				t.Fatalf("backend.kind = %s, want %s", cfg.Backend.Kind, model.BackendKindK8s)
			}
			if cfg.K8s.Provider != tt.wantK8sProvider {
				t.Fatalf("k8s.provider = %s, want %s", cfg.K8s.Provider, tt.wantK8sProvider)
			}
			if cfg.K8s.Context != tt.wantK8sContext {
				t.Fatalf("k8s.context = %q, want %q", cfg.K8s.Context, tt.wantK8sContext)
			}
		})
	}
}

func TestNormalizeRunConfigDerivesExecutionProviderFromLegacyK8sProvider(t *testing.T) {
	tests := []struct {
		name           string
		k8sProvider    model.K8sProvider
		wantExecution  model.ProviderKind
		wantK8sContext string
	}{
		{name: "remote", k8sProvider: model.K8sProviderRemote, wantExecution: model.ProviderNative},
		{name: "orbstack", k8sProvider: model.K8sProviderOrbStackLocal, wantExecution: model.ProviderOrbStack, wantK8sContext: "orbstack"},
		{name: "minikube", k8sProvider: model.K8sProviderMinikube, wantExecution: model.ProviderMinikube, wantK8sContext: "minikube"},
		{name: "k3s", k8sProvider: model.K8sProviderK3s, wantExecution: model.ProviderK3s},
		{name: "microk8s", k8sProvider: model.K8sProviderMicroK8s, wantExecution: model.ProviderMicroK8s},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultRunConfig()
			cfg.Platform.RunMode = model.RunModeSTGLinux
			cfg.Backend.Kind = model.BackendKindK8s
			cfg.Execution = model.ExecutionConfig{}
			cfg.K8s.Provider = tt.k8sProvider

			cfg = NormalizeRunConfig(cfg)
			if cfg.Execution.Backend != model.ExecutionBackendK8s {
				t.Fatalf("execution.backend = %s, want %s", cfg.Execution.Backend, model.ExecutionBackendK8s)
			}
			if cfg.Execution.Provider != tt.wantExecution {
				t.Fatalf("execution.provider = %s, want %s", cfg.Execution.Provider, tt.wantExecution)
			}
			if cfg.K8s.Context != tt.wantK8sContext {
				t.Fatalf("k8s.context = %q, want %q", cfg.K8s.Context, tt.wantK8sContext)
			}
		})
	}
}

func TestNormalizeRunConfigNormalizesMicroVMAliasToFirecracker(t *testing.T) {
	cfg := DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderK3s,
		RuntimeProfile: model.ExecutionRuntimeProfile("microvm"),
	}
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Runtime.Profile = model.RuntimeProfile("microvm")
	cfg.Runtime.ClassName = "sandbox-runner-microvm"

	cfg = NormalizeRunConfig(cfg)
	if cfg.Execution.RuntimeProfile != model.ExecutionRuntimeProfileFirecracker {
		t.Fatalf("execution.runtime_profile = %s, want %s", cfg.Execution.RuntimeProfile, model.ExecutionRuntimeProfileFirecracker)
	}
	if cfg.Runtime.Profile != model.RuntimeProfileFirecracker {
		t.Fatalf("runtime.profile = %s, want %s", cfg.Runtime.Profile, model.RuntimeProfileFirecracker)
	}
	if cfg.Kata.RuntimeClassName != "sandbox-runner-microvm" {
		t.Fatalf("kata.runtime_class_name = %q, want sandbox-runner-microvm", cfg.Kata.RuntimeClassName)
	}
}

func TestNormalizeRunConfigKeepsLegacyRuntimeClassForConditionalRuntime(t *testing.T) {
	cfg := DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderMinikube,
		RuntimeProfile: model.ExecutionRuntimeProfileFirecracker,
	}
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Runtime.Profile = model.RuntimeProfileFirecracker
	cfg.Runtime.ClassName = ""
	cfg.Kata.RuntimeClassName = "sandbox-runner-microvm"

	cfg = NormalizeRunConfig(cfg)
	if cfg.Runtime.ClassName != "sandbox-runner-microvm" {
		t.Fatalf("runtime.class_name = %q, want sandbox-runner-microvm", cfg.Runtime.ClassName)
	}
}
