package executor

import (
	"context"

	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/proc"
)

type DirectExecutor struct {
	runner proc.Runner
	target model.ExecutionTarget
}

func NewDirectExecutor(target model.ExecutionTarget) DirectExecutor {
	return DirectExecutor{
		runner: proc.NewRunner(),
		target: target,
	}
}

func (e DirectExecutor) Run(ctx context.Context, spec Spec, handler proc.IOHandler) (Result, error) {
	res, err := e.runner.Run(ctx, proc.CommandSpec{
		Phase:           spec.Phase,
		Command:         spec.Command,
		Env:             spec.Env,
		Dir:             spec.Dir,
		Timeout:         spec.Timeout,
		RunID:           spec.RunID,
		Attempt:         spec.Attempt,
		CommandClass:    spec.CommandClass,
		ArtifactDir:     spec.ArtifactDir,
		LogLineMaxBytes: spec.LogLineMaxBytes,
	}, handler)
	return Result{
		ExitCode:    res.ExitCode,
		Signal:      res.Signal,
		TimedOut:    res.TimedOut,
		StartedAt:   res.StartedAt,
		FinishedAt:  res.FinishedAt,
		Duration:    res.Duration,
		StdoutLines: res.StdoutLines,
		StderrLines: res.StderrLines,
		Target:      e.target,
		Metadata:    map[string]any{},
	}, err
}
