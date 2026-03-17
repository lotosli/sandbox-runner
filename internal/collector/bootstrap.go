package collector

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type BootstrapResult struct {
	Enabled    bool
	UsingJSONL bool
	Warnings   []string
	cmd        *exec.Cmd
}

func Bootstrap(ctx context.Context, cfg model.RunConfig) (BootstrapResult, error) {
	mode := cfg.Collector.Mode
	timeout := time.Duration(cfg.Collector.HealthcheckTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}

	if mode == model.CollectorModeSkip {
		return BootstrapResult{Enabled: false, UsingJSONL: true}, nil
	}

	if Healthy(ctx, cfg.Run.OTLPEndpoint, timeout) {
		return BootstrapResult{Enabled: true, UsingJSONL: false}, nil
	}

	if mode == model.CollectorModeRequire {
		return BootstrapResult{}, fmt.Errorf("%s: %s", model.ErrorCodeCollectorUnavailable, cfg.Run.OTLPEndpoint)
	}

	if cfg.Platform.RunMode != model.RunModeLocalDirect {
		return BootstrapResult{Enabled: false, UsingJSONL: true, Warnings: []string{"collector unavailable; falling back to JSONL"}}, nil
	}

	cmd, warnings, err := startLocalCollector(cfg)
	if err != nil {
		return BootstrapResult{Enabled: false, UsingJSONL: true, Warnings: append(warnings, err.Error())}, nil
	}
	return BootstrapResult{Enabled: true, UsingJSONL: false, Warnings: warnings, cmd: cmd}, nil
}

func (r BootstrapResult) Shutdown() error {
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	return r.cmd.Process.Kill()
}

func startLocalCollector(cfg model.RunConfig) (*exec.Cmd, []string, error) {
	candidates := [][]string{
		cfg.Collector.LocalCollectorCommand,
		{"otelcol-contrib", "--config", cfg.Collector.LocalCollectorConfig},
		{"otelcol", "--config", cfg.Collector.LocalCollectorConfig},
	}
	warnings := []string{}
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		cmd := exec.Command(candidate[0], candidate[1:]...)
		if err := cmd.Start(); err == nil {
			time.Sleep(500 * time.Millisecond)
			return cmd, warnings, nil
		} else {
			warnings = append(warnings, fmt.Sprintf("failed to start %q: %v", candidate[0], err))
		}
	}
	return nil, warnings, fmt.Errorf("no local collector command succeeded")
}
