package kubernetes

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestBuildJobSetsRuntimeClassNameForKata(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Runtime.Profile = model.RuntimeProfileKata
	cfg.Kata.RuntimeClassName = "kata"
	cfg.Run.RunID = "run-kata"
	cfg.Run.SandboxID = "sbx-kata"

	job := BuildJob(model.RunRequest{RunConfig: cfg}, "sandbox-system")
	if job.Spec.Template.Spec.RuntimeClassName == nil {
		t.Fatal("runtimeClassName = nil, want kata")
	}
	if *job.Spec.Template.Spec.RuntimeClassName != "kata" {
		t.Fatalf("runtimeClassName = %s, want kata", *job.Spec.Template.Spec.RuntimeClassName)
	}
}

func TestBuildJobLeavesRuntimeClassNameEmptyForNative(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Runtime.Profile = model.RuntimeProfileNative
	cfg.Run.RunID = "run-native"
	cfg.Run.SandboxID = "sbx-native"

	job := BuildJob(model.RunRequest{RunConfig: cfg}, "sandbox-system")
	if job.Spec.Template.Spec.RuntimeClassName != nil {
		t.Fatalf("runtimeClassName = %v, want nil", *job.Spec.Template.Spec.RuntimeClassName)
	}
}

func TestBuildJobAnnotatesOrbStackProvider(t *testing.T) {
	tests := []struct {
		name             string
		execution        model.ProviderKind
		legacyProvider   model.K8sProvider
		runtimeProfile   model.RuntimeProfile
		wantProviderName string
	}{
		{name: "orbstack", execution: model.ProviderOrbStack, legacyProvider: model.K8sProviderOrbStackLocal, runtimeProfile: model.RuntimeProfileOrbStackK8s, wantProviderName: "orbstack"},
		{name: "minikube", execution: model.ProviderMinikube, legacyProvider: model.K8sProviderMinikube, runtimeProfile: model.RuntimeProfileNative, wantProviderName: "minikube"},
		{name: "k3s", execution: model.ProviderK3s, legacyProvider: model.K8sProviderK3s, runtimeProfile: model.RuntimeProfileNative, wantProviderName: "k3s"},
		{name: "microk8s", execution: model.ProviderMicroK8s, legacyProvider: model.K8sProviderMicroK8s, runtimeProfile: model.RuntimeProfileNative, wantProviderName: "microk8s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultRunConfig()
			cfg.Platform.RunMode = model.RunModeSTGLinux
			cfg.Backend.Kind = model.BackendKindK8s
			cfg.Execution = model.ExecutionConfig{
				Backend:        model.ExecutionBackendK8s,
				Provider:       tt.execution,
				RuntimeProfile: model.ExecutionRuntimeProfileDefault,
			}
			cfg.K8s.Provider = tt.legacyProvider
			cfg.Runtime.Profile = tt.runtimeProfile
			cfg.Run.RunID = "run-" + tt.name
			cfg.Run.SandboxID = "sbx-" + tt.name

			job := BuildJob(model.RunRequest{RunConfig: cfg}, "sandbox-system")
			if job.Labels["backend_provider"] != tt.wantProviderName {
				t.Fatalf("backend_provider label = %q, want %s", job.Labels["backend_provider"], tt.wantProviderName)
			}
			got := ""
			for _, item := range job.Spec.Template.Spec.Containers[0].Env {
				if item.Name == "BACKEND_PROVIDER" {
					got = item.Value
					break
				}
			}
			if got != tt.wantProviderName {
				t.Fatalf("BACKEND_PROVIDER env = %q, want %s", got, tt.wantProviderName)
			}
		})
	}
}
