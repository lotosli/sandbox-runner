package examples

import (
	"os"
	"path/filepath"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8syaml "sigs.k8s.io/yaml"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
	"gopkg.in/yaml.v3"
)

type k8sExampleProvider struct {
	name      string
	execution model.ProviderKind
	legacy    model.K8sProvider
	context   string
}

func TestK8sProviderLanguageSamplesRenderJobs(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")

	providers := []k8sExampleProvider{
		{name: "minikube", execution: model.ProviderMinikube, legacy: model.K8sProviderMinikube, context: "minikube"},
		{name: "k3s", execution: model.ProviderK3s, legacy: model.K8sProviderK3s},
		{name: "microk8s", execution: model.ProviderMicroK8s, legacy: model.K8sProviderMicroK8s},
	}

	for _, sample := range backendLanguageSamples(root) {
		for _, provider := range providers {
			t.Run(sample.name+"/"+provider.name, func(t *testing.T) {
				workDir := t.TempDir()
				copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
				copyTree(t, sample.dir, copiedDir)

				cfgPath, cfg := writeK8sExampleConfig(t, copiedDir, provider)
				runCommand(t, root, binary, "validate", "--config", cfgPath, "--policy", policyPath)
				rendered := runCommand(t, root, binary, "k8s", "render-job", "--config", cfgPath, "--policy", policyPath)

				var job batchv1.Job
				if err := k8syaml.Unmarshal([]byte(rendered), &job); err != nil {
					t.Fatalf("Unmarshal(rendered job) error = %v\n%s", err, rendered)
				}
				if job.Labels["backend_provider"] != provider.name {
					t.Fatalf("backend_provider label = %q, want %s", job.Labels["backend_provider"], provider.name)
				}
				container := job.Spec.Template.Spec.Containers[0]
				if container.Image != cfg.Run.Image {
					t.Fatalf("image = %q, want %q", container.Image, cfg.Run.Image)
				}
				wantPrefix := []string{"/usr/local/bin/sandbox-runner", "run", "--config", "/etc/sandbox/run.yaml", "--policy", "/etc/sandbox/policy.yaml", "--"}
				for i, want := range wantPrefix {
					if len(container.Command) <= i || container.Command[i] != want {
						t.Fatalf("container command prefix = %#v, want %q at index %d", container.Command, want, i)
					}
				}
				for i, want := range cfg.Run.Command {
					gotIndex := len(wantPrefix) + i
					if len(container.Command) <= gotIndex || container.Command[gotIndex] != want {
						t.Fatalf("container command = %#v, want %q at index %d", container.Command, want, gotIndex)
					}
				}
				if envValue(container.Env, "BACKEND_PROVIDER") != provider.name {
					t.Fatalf("BACKEND_PROVIDER env = %q, want %s", envValue(container.Env, "BACKEND_PROVIDER"), provider.name)
				}
			})
		}
	}
}

func writeK8sExampleConfig(t *testing.T, dir string, provider k8sExampleProvider) (string, model.RunConfig) {
	t.Helper()
	cfgPath := filepath.Join(dir, "run.docker.sample.yaml")
	cfg, err := config.LoadRunConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadRunConfig(%s) error = %v", cfgPath, err)
	}
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       provider.execution,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Runtime.Profile = model.RuntimeProfileNative
	cfg.K8s.Provider = provider.legacy
	cfg.K8s.Context = provider.context
	cfg.K8s.Namespace = "ai-sandbox-runner-runs"
	cfg.Run.Image = "ghcr.io/lotosli/sandbox-runner:latest"
	cfg.Sandbox.Image = cfg.Run.Image
	cfg.Run.SandboxID = "k8s-" + provider.name + "-" + filepath.Base(dir)
	cfg.Run.ServiceName = cfg.Run.ServiceName + "-" + provider.name

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	outputPath := filepath.Join(dir, "run.k8s."+provider.name+".yaml")
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", outputPath, err)
	}
	return outputPath, cfg
}

func envValue(envs []corev1.EnvVar, name string) string {
	for _, item := range envs {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
