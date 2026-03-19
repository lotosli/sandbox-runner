package executor

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/proc"
)

type BackendExecutor struct {
	backend backend.SandboxBackend
	runCfg  model.RunConfig
	target  model.ExecutionTarget
}

func NewBackendExecutor(runCfg model.RunConfig, target model.ExecutionTarget, backendImpl backend.SandboxBackend) (BackendExecutor, error) {
	if backendImpl == nil {
		return BackendExecutor{}, fmt.Errorf("backend executor requires a backend implementation")
	}
	return BackendExecutor{
		backend: backendImpl,
		runCfg:  runCfg,
		target:  target,
	}, nil
}

func (e BackendExecutor) Run(ctx context.Context, spec Spec, handler proc.IOHandler) (Result, error) {
	started := time.Now().UTC()
	sandboxID := spec.RunConfig.Run.SandboxID
	if sandboxID == "" {
		return Result{}, fmt.Errorf("run.sandbox_id is required for backend execution")
	}

	handle, err := e.backend.Exec(ctx, sandboxID, backend.ExecRequest{
		Command:    shellJoin(spec.Command),
		Cwd:        e.remoteCwd(spec.Dir),
		Env:        spec.Env,
		Background: false,
		Timeout:    spec.Timeout,
		Class:      spec.CommandClass,
	})
	if err != nil {
		return Result{}, err
	}

	logs, err := e.backend.StreamLogs(ctx, sandboxID, handle.ExecID)
	if err != nil {
		return Result{}, err
	}

	stdoutLines := 0
	stderrLines := 0
	interrupted := false

loop:
	for {
		select {
		case <-ctx.Done():
			if !interrupted {
				interrupted = true
				if canceler, ok := e.backend.(backend.ExecCanceler); ok {
					_ = canceler.CancelExec(context.Background(), sandboxID, handle.ExecID)
				}
			}
			break loop
		case chunk, ok := <-logs:
			if !ok {
				break loop
			}
			lineNo := 0
			switch chunk.Stream {
			case "stderr":
				stderrLines++
				lineNo = stderrLines
			default:
				stdoutLines++
				lineNo = stdoutLines
			}
			if handler != nil {
				_ = handler.OnLog(ctx, model.StructuredLog{
					Timestamp:      chunk.Timestamp,
					RunID:          spec.RunID,
					Attempt:        spec.Attempt,
					Phase:          spec.Phase,
					CommandClass:   spec.CommandClass,
					CommandID:      handle.ExecID,
					Provider:       backendProviderName(spec.RunConfig),
					ExecProviderID: handle.ExecID,
					Stream:         chunk.Stream,
					LineNo:         lineNo,
					Line:           proc.Redact(truncateLine(chunk.Line, spec.LogLineMaxBytes)),
					Attributes: map[string]string{
						"execution.backend":             string(spec.RunConfig.Execution.Backend),
						"execution.provider":            string(spec.RunConfig.Execution.Provider),
						"execution.runtime_profile":     string(spec.RunConfig.Execution.RuntimeProfile),
						"execution.compatibility_level": spec.RunConfig.Metadata["execution.compatibility_level"],
						"backend.kind":                  string(spec.RunConfig.Backend.Kind),
						"backend.provider":              backendProviderName(spec.RunConfig),
						"runtime.profile":               string(spec.RunConfig.Runtime.Profile),
						"sandbox.backend.kind":          string(spec.RunConfig.Backend.Kind),
						"sandbox.provider.name":         providerName(spec.RunConfig),
						"sandbox.runtime.kind":          string(spec.RunConfig.OpenSandbox.Runtime),
						"sandbox.runtime.profile":       string(spec.RunConfig.Runtime.Profile),
						"sandbox.runtime.class":         spec.RunConfig.Kata.RuntimeClassName,
					},
				})
			}
		}
	}

	statusErr := error(nil)
	exitCode := 0
	providerError := ""
	if statusProvider, ok := e.backend.(backend.ExecStatusProvider); ok {
		status, err := statusProvider.ExecStatus(context.Background(), sandboxID, handle.ExecID)
		if err != nil {
			statusErr = err
		} else {
			if status.ExitCode != nil {
				exitCode = *status.ExitCode
			}
			providerError = status.Error
		}
	}

	finished := time.Now().UTC()
	result := Result{
		ExitCode:    exitCode,
		TimedOut:    errors.Is(ctx.Err(), context.DeadlineExceeded),
		StartedAt:   started,
		FinishedAt:  finished,
		Duration:    finished.Sub(started),
		StdoutLines: stdoutLines,
		StderrLines: stderrLines,
		Target:      e.resultTarget(sandboxID),
		Metadata: map[string]any{
			"backend_kind":     spec.RunConfig.Execution.Backend,
			"provider":         providerName(spec.RunConfig),
			"backend_provider": backendProviderName(spec.RunConfig),
			"exec_provider_id": handle.ExecID,
		},
	}
	if result.TimedOut {
		result.Signal = "SIGTERM"
		result.Metadata["backend.cancel.best_effort"] = true
	}
	if providerError != "" {
		result.Metadata["provider_error"] = providerError
	}
	if statusErr != nil {
		result.Metadata["status_error"] = statusErr.Error()
	}
	if result.TimedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func (e BackendExecutor) remoteCwd(localDir string) string {
	workspace := e.runCfg.Run.WorkspaceDir
	remoteRoot := backendWorkspaceRoot(e.runCfg)
	if localDir == "" || workspace == "" || remoteRoot == "" {
		if e.runCfg.Backend.Kind == model.BackendKindDevContainer {
			return ""
		}
		return remoteRoot
	}
	rel, err := filepath.Rel(workspace, localDir)
	if err != nil || rel == "." {
		if e.runCfg.Backend.Kind == model.BackendKindDevContainer {
			return ""
		}
		return remoteRoot
	}
	if e.runCfg.Backend.Kind == model.BackendKindDevContainer {
		return path.Join("/workspace", filepath.ToSlash(rel))
	}
	return path.Join(remoteRoot, filepath.ToSlash(rel))
}

func (e BackendExecutor) resultTarget(sandboxID string) model.ExecutionTarget {
	image := e.runCfg.Sandbox.Image
	if image == "" {
		image = e.runCfg.Run.Image
	}
	containerID := sandboxID
	if e.runCfg.Backend.Kind == model.BackendKindDevContainer && e.runCfg.Metadata["devcontainer.container_id"] != "" {
		containerID = e.runCfg.Metadata["devcontainer.container_id"]
	}
	target := model.ExecutionTarget{
		OS:                 "linux",
		Arch:               e.target.Arch,
		Mode:               e.runCfg.Platform.RunMode,
		BackendKind:        string(e.runCfg.Execution.Backend),
		ProviderName:       providerName(e.runCfg),
		BackendProvider:    backendProviderName(e.runCfg),
		RuntimeProfile:     string(e.runCfg.Execution.RuntimeProfile),
		RuntimeClassName:   e.runCfg.Kata.RuntimeClassName,
		ContainerID:        containerID,
		ContainerImage:     image,
		Execution:          e.runCfg.Execution,
		CompatibilityLevel: model.SupportLevel(e.runCfg.Metadata["execution.compatibility_level"]),
	}
	switch e.runCfg.Backend.Kind {
	case model.BackendKindDevContainer:
		target.RuntimeKind = "devcontainer"
		target.Virtualization = "none"
	case model.BackendKindOpenSandbox:
		target.RuntimeKind = string(e.runCfg.OpenSandbox.Runtime)
		target.NetworkMode = e.runCfg.OpenSandbox.NetworkMode
		target.InKubernetes = e.runCfg.OpenSandbox.Runtime == model.OpenSandboxRuntimeKubernetes
		target.DockerAvailable = e.runCfg.OpenSandbox.Runtime == model.OpenSandboxRuntimeDocker
		if e.runCfg.Runtime.Profile == model.RuntimeProfileKata {
			target.Virtualization = "kata"
		} else {
			target.Virtualization = "none"
		}
	case model.BackendKindAppleContainer:
		target.RuntimeKind = "apple-container"
		target.Virtualization = "apple-container"
		target.LocalPlatform = "macos"
	case model.BackendKindOrbStackMachine:
		target.RuntimeKind = "orbstack-machine"
		target.Virtualization = "vm"
		target.LocalPlatform = "orbstack"
		target.MachineName = e.runCfg.OrbStack.MachineName
	}
	return target
}

func providerName(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	return backendProviderName(cfg)
}

func backendProviderName(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "native"
	case model.BackendKindK8s:
		return string(model.ExecutionProviderForK8sProvider(cfg.K8s.Provider))
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	case model.BackendKindDirect:
		return "native"
	default:
		return string(cfg.Backend.Kind)
	}
}

func backendWorkspaceRoot(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindDevContainer:
		if cfg.DevContainer.WorkspaceFolder != "" {
			return cfg.DevContainer.WorkspaceFolder
		}
		return "/workspace"
	case model.BackendKindAppleContainer:
		return cfg.AppleContainer.WorkspaceRoot
	case model.BackendKindOrbStackMachine:
		return cfg.OrbStack.MachineWorkspaceRoot
	case model.BackendKindOpenSandbox:
		return cfg.OpenSandbox.WorkspaceRoot
	default:
		return ""
	}
}

func truncateLine(line string, limit int) string {
	if limit > 0 && len(line) > limit {
		return line[:limit]
	}
	return line
}

func shellJoin(command []string) string {
	parts := make([]string, 0, len(command))
	for _, arg := range command {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
