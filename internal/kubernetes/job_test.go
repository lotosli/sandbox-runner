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
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.K8s.Provider = model.K8sProviderOrbStackLocal
	cfg.Runtime.Profile = model.RuntimeProfileOrbStackK8s
	cfg.Run.RunID = "run-orbstack"
	cfg.Run.SandboxID = "sbx-orbstack"

	job := BuildJob(model.RunRequest{RunConfig: cfg}, "sandbox-system")
	if job.Labels["backend_provider"] != "orbstack" {
		t.Fatalf("backend_provider label = %q, want orbstack", job.Labels["backend_provider"])
	}
	got := ""
	for _, item := range job.Spec.Template.Spec.Containers[0].Env {
		if item.Name == "BACKEND_PROVIDER" {
			got = item.Value
			break
		}
	}
	if got != "orbstack" {
		t.Fatalf("BACKEND_PROVIDER env = %q, want orbstack", got)
	}
}
