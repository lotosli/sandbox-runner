package phase

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/adapter"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestBuildPhaseEnvExpandsRunExtraEnvAndPreservesSecrets(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SANDBOX_SAMPLE_NAME", "sandbox")

	cfg := config.DefaultRunConfig()
	cfg.Run.ExtraEnv = map[string]string{
		"PATH":       ".sample-bin:$PATH",
		"SAMPLE_TAG": "$SANDBOX_SAMPLE_NAME",
	}
	cfg.Go.ExtraEnv = map[string]string{
		"GOFLAGS": "-mod=readonly",
	}

	env := buildPhaseEnv(cfg, true, map[string]string{
		"SECRET_VALUE": "token-$PATH",
		"SAMPLE_TAG":   "from-secret",
	})

	if env["PATH"] != ".sample-bin:/usr/bin:/bin" {
		t.Fatalf("PATH = %q, want .sample-bin:/usr/bin:/bin", env["PATH"])
	}
	if env["SAMPLE_TAG"] != "from-secret" {
		t.Fatalf("SAMPLE_TAG = %q, want from-secret", env["SAMPLE_TAG"])
	}
	if env["SECRET_VALUE"] != "token-$PATH" {
		t.Fatalf("SECRET_VALUE = %q, want token-$PATH", env["SECRET_VALUE"])
	}
	if env["GOFLAGS"] != "-mod=readonly" {
		t.Fatalf("GOFLAGS = %q, want -mod=readonly", env["GOFLAGS"])
	}
}

func TestOpenSandboxBootstrapEnvIncludesAdapterEnv(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Run.Language = "node"
	cfg.Run.ServiceName = "sandbox-runner-node-opensandbox-sample"
	cfg.Run.OTLPEndpoint = "http://127.0.0.1:4318"
	cfg.Run.DeploymentEnvironment = "local"
	cfg.Run.Command = []string{"node", "app.js", "execute"}

	engine := Engine{registry: adapter.NewRegistry()}
	env := engine.openSandboxBootstrapEnv(cfg)

	if env["OTEL_SERVICE_NAME"] != "sandbox-runner-node-opensandbox-sample" {
		t.Fatalf("OTEL_SERVICE_NAME = %q, want sandbox-runner-node-opensandbox-sample", env["OTEL_SERVICE_NAME"])
	}
	if _, ok := env["NODE_OPTIONS"]; ok {
		t.Fatalf("NODE_OPTIONS should not be pre-injected into opensandbox bootstrap env, got %q", env["NODE_OPTIONS"])
	}
}
