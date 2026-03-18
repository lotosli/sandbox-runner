package backend

import (
	"strings"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestParseAppleContainerInspect(t *testing.T) {
	output := []byte(`{"id":"ac-123","status":"running","image":"ghcr.io/example/app:latest"}`)
	info, ok := parseAppleContainerInspect(output)
	if !ok {
		t.Fatal("parseAppleContainerInspect() ok = false, want true")
	}
	if info.ID != "ac-123" {
		t.Fatalf("info.ID = %q, want ac-123", info.ID)
	}
	if info.Status != "running" {
		t.Fatalf("info.Status = %q, want running", info.Status)
	}
	if info.Metadata["image"] != "ghcr.io/example/app:latest" {
		t.Fatalf("image = %q, want ghcr.io/example/app:latest", info.Metadata["image"])
	}
}

func TestParseAppleContainerInspectEnv(t *testing.T) {
	output := []byte(`{"configuration":{"initProcess":{"environment":["PATH=/usr/local/go/bin:/usr/bin","GOPATH=/go"]}}}`)
	env := parseAppleContainerInspectEnv(output)
	if env["PATH"] != "/usr/local/go/bin:/usr/bin" {
		t.Fatalf("PATH = %q, want /usr/local/go/bin:/usr/bin", env["PATH"])
	}
	if env["GOPATH"] != "/go" {
		t.Fatalf("GOPATH = %q, want /go", env["GOPATH"])
	}
}

func TestParseOrbStackMachineList(t *testing.T) {
	output := []byte(`[{"name":"ai-runner-dev","status":"running","distro":"ubuntu"}]`)
	info, ok := parseOrbStackMachineList(output, "ai-runner-dev")
	if !ok {
		t.Fatal("parseOrbStackMachineList() ok = false, want true")
	}
	if info.ID != "ai-runner-dev" {
		t.Fatalf("info.ID = %q, want ai-runner-dev", info.ID)
	}
	if info.Metadata["distro"] != "ubuntu" {
		t.Fatalf("distro = %q, want ubuntu", info.Metadata["distro"])
	}
}

func TestOrbStackExecEnvIncludesORBENV(t *testing.T) {
	env := orbstackExecEnv([]string{"PATH=/usr/bin"}, map[string]string{
		"ZZZ": "1",
		"AAA": "2",
	})
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "AAA=2") || !strings.Contains(joined, "ZZZ=1") {
		t.Fatalf("env = %v, want extra env variables", env)
	}
	if !strings.Contains(joined, "ORBENV=AAA:ZZZ") {
		t.Fatalf("env = %v, want sorted ORBENV list", env)
	}
}

func TestLocalBackendRuntimeInfoUsesOrbStackProviderMetadata(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindDocker
	cfg.Platform.RunMode = model.RunModeLocalDocker
	cfg.Docker.Provider = model.DockerProviderOrbStack
	cfg.Runtime.Profile = model.RuntimeProfileOrbStackDocker

	info, err := NewLocalBackend(model.BackendKindDocker, cfg).RuntimeInfo(nil)
	if err != nil {
		t.Fatalf("RuntimeInfo() error = %v", err)
	}
	if info.BackendProvider != "orbstack" {
		t.Fatalf("BackendProvider = %q, want orbstack", info.BackendProvider)
	}
	if info.LocalPlatform != "orbstack" {
		t.Fatalf("LocalPlatform = %q, want orbstack", info.LocalPlatform)
	}
}
