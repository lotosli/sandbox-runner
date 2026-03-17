package adapter

import (
	"fmt"
	"path/filepath"
	"strings"

	golang "github.com/lotosli/sandbox-runner/internal/lang/go"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type Adapter interface {
	Detect(cmd []string, env map[string]string) bool
	Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error)
	Name() string
}

type Registry struct {
	adapters []Adapter
}

func NewRegistry() Registry {
	return Registry{
		adapters: []Adapter{
			JavaAdapter{},
			PythonAdapter{},
			NodeAdapter{},
			golang.Adapter{},
			ShellAdapter{},
		},
	}
}

func (r Registry) Resolve(language string, cmd []string, env map[string]string) (Adapter, error) {
	if language != "" && language != "auto" {
		for _, adapter := range r.adapters {
			if adapter.Name() == language {
				return adapter, nil
			}
		}
		return nil, fmt.Errorf("unsupported language adapter: %s", language)
	}
	for _, adapter := range r.adapters {
		if adapter.Detect(cmd, env) {
			return adapter, nil
		}
	}
	return ShellAdapter{}, nil
}

type JavaAdapter struct{}

func (JavaAdapter) Detect(cmd []string, env map[string]string) bool {
	_ = env
	if len(cmd) == 0 {
		return false
	}
	return filepath.Base(cmd[0]) == "java"
}

func (JavaAdapter) Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error) {
	env = cloneEnv(env)
	if _, ok := env["JAVA_TOOL_OPTIONS"]; !ok {
		env["JAVA_TOOL_OPTIONS"] = "-javaagent:/opt/otel/opentelemetry-javaagent.jar"
	}
	return cmd, mergeOTelEnv(env, cfg), nil
}

func (JavaAdapter) Name() string { return "java" }

type PythonAdapter struct{}

func (PythonAdapter) Detect(cmd []string, env map[string]string) bool {
	_ = env
	if len(cmd) == 0 {
		return false
	}
	base := filepath.Base(cmd[0])
	return base == "python" || base == "python3" || strings.HasPrefix(base, "python")
}

func (PythonAdapter) Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error) {
	env = mergeOTelEnv(cloneEnv(env), cfg)
	if len(cmd) == 0 {
		return cmd, env, nil
	}
	return append([]string{"opentelemetry-instrument"}, cmd...), env, nil
}

func (PythonAdapter) Name() string { return "python" }

type NodeAdapter struct{}

func (NodeAdapter) Detect(cmd []string, env map[string]string) bool {
	_ = env
	if len(cmd) == 0 {
		return false
	}
	base := filepath.Base(cmd[0])
	return base == "node" || base == "npm" || base == "pnpm"
}

func (NodeAdapter) Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error) {
	env = mergeOTelEnv(cloneEnv(env), cfg)
	if _, ok := env["NODE_OPTIONS"]; !ok {
		env["NODE_OPTIONS"] = "--require @opentelemetry/auto-instrumentations-node/register"
	}
	return cmd, env, nil
}

func (NodeAdapter) Name() string { return "node" }

type ShellAdapter struct{}

func (ShellAdapter) Detect(cmd []string, env map[string]string) bool {
	_ = env
	return true
}

func (ShellAdapter) Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error) {
	return cmd, mergeOTelEnv(cloneEnv(env), cfg), nil
}

func (ShellAdapter) Name() string { return "shell" }

func cloneEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func mergeOTelEnv(env map[string]string, cfg model.RunConfig) map[string]string {
	env["OTEL_SERVICE_NAME"] = cfg.Run.ServiceName
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = cfg.Run.OTLPEndpoint
	env["OTEL_TRACES_EXPORTER"] = "otlp"
	env["OTEL_METRICS_EXPORTER"] = "otlp"
	env["OTEL_LOGS_EXPORTER"] = "otlp"
	resourceAttrs := []string{
		"deployment.environment.name=" + cfg.Run.DeploymentEnvironment,
		"run_id=" + cfg.Run.RunID,
		fmt.Sprintf("attempt=%d", cfg.Run.Attempt),
		"sandbox_id=" + cfg.Run.SandboxID,
	}
	env["OTEL_RESOURCE_ATTRIBUTES"] = strings.Join(resourceAttrs, ",")
	return env
}
