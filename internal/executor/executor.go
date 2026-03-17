package executor

import (
	"context"
	"time"

	"github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/proc"
)

type Spec struct {
	Phase           model.Phase
	Command         []string
	Env             map[string]string
	Dir             string
	Timeout         time.Duration
	RunID           string
	Attempt         int
	CommandClass    string
	ArtifactDir     string
	LogLineMaxBytes int
	RunConfig       model.RunConfig
	Target          model.ExecutionTarget
}

type Result struct {
	ExitCode    int
	Signal      string
	TimedOut    bool
	StartedAt   time.Time
	FinishedAt  time.Time
	Duration    time.Duration
	StdoutLines int
	StderrLines int
	Target      model.ExecutionTarget
	Metadata    map[string]any
}

type Executor interface {
	Run(ctx context.Context, spec Spec, handler proc.IOHandler) (Result, error)
}

func New(runCfg model.RunConfig, target model.ExecutionTarget, backendImpl backend.SandboxBackend) (Executor, error) {
	switch runCfg.Backend.Kind {
	case model.BackendKindOpenSandbox, model.BackendKindDevContainer, model.BackendKindAppleContainer, model.BackendKindOrbStackMachine:
		return NewBackendExecutor(runCfg, target, backendImpl)
	}
	if runCfg.Platform.RunMode == model.RunModeLocalDocker {
		return NewDockerExecutor(runCfg)
	}
	return NewDirectExecutor(target), nil
}
