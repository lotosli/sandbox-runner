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
					Provider:       providerName(spec.RunConfig),
					ExecProviderID: handle.ExecID,
					Stream:         chunk.Stream,
					LineNo:         lineNo,
					Line:           proc.Redact(truncateLine(chunk.Line, spec.LogLineMaxBytes)),
					Attributes: map[string]string{
						"sandbox.backend.kind":  string(spec.RunConfig.Backend.Kind),
						"sandbox.provider.name": providerName(spec.RunConfig),
						"sandbox.runtime.kind":  string(spec.RunConfig.OpenSandbox.Runtime),
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
			"backend_kind":     spec.RunConfig.Backend.Kind,
			"provider":         providerName(spec.RunConfig),
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
	remoteRoot := e.runCfg.OpenSandbox.WorkspaceRoot
	if localDir == "" || workspace == "" || remoteRoot == "" {
		return remoteRoot
	}
	rel, err := filepath.Rel(workspace, localDir)
	if err != nil || rel == "." {
		return remoteRoot
	}
	return path.Join(remoteRoot, filepath.ToSlash(rel))
}

func (e BackendExecutor) resultTarget(sandboxID string) model.ExecutionTarget {
	image := e.runCfg.Sandbox.Image
	if image == "" {
		image = e.runCfg.Run.Image
	}
	return model.ExecutionTarget{
		OS:              "linux",
		Arch:            e.target.Arch,
		Mode:            e.runCfg.Platform.RunMode,
		BackendKind:     string(e.runCfg.Backend.Kind),
		ProviderName:    providerName(e.runCfg),
		RuntimeKind:     string(e.runCfg.OpenSandbox.Runtime),
		NetworkMode:     e.runCfg.OpenSandbox.NetworkMode,
		ContainerID:     sandboxID,
		ContainerImage:  image,
		InKubernetes:    e.runCfg.OpenSandbox.Runtime == model.OpenSandboxRuntimeKubernetes,
		DockerAvailable: e.runCfg.OpenSandbox.Runtime == model.OpenSandboxRuntimeDocker,
	}
}

func providerName(cfg model.RunConfig) string {
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		return "opensandbox"
	}
	return string(cfg.Backend.Kind)
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
