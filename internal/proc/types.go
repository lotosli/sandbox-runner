package proc

import (
	"context"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type IOHandler interface {
	OnLog(ctx context.Context, log model.StructuredLog) error
}

type CommandSpec struct {
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
}
