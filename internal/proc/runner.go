package proc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type Runner struct{}

func NewRunner() Runner { return Runner{} }

func (Runner) Run(ctx context.Context, spec CommandSpec, handler IOHandler) (Result, error) {
	if len(spec.Command) == 0 {
		return Result{}, errors.New("empty command")
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, spec.Command[0], spec.Command[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = mergeEnv(spec.Env)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, err
	}

	started := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		return Result{StartedAt: started, FinishedAt: time.Now().UTC()}, err
	}

	result := Result{StartedAt: started}
	var wg sync.WaitGroup
	var stdoutLines, stderrLines int

	wg.Add(2)
	go func() {
		defer wg.Done()
		stdoutLines = streamPipe(runCtx, stdoutPipe, "stdout", spec, handler)
	}()
	go func() {
		defer wg.Done()
		stderrLines = streamPipe(runCtx, stderrPipe, "stderr", spec, handler)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	finished := time.Now().UTC()
	result.FinishedAt = finished
	result.Duration = finished.Sub(started)
	result.StdoutLines = stdoutLines
	result.StderrLines = stderrLines

	if runCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = -1
		return result, context.DeadlineExceeded
	}

	if waitErr == nil {
		result.ExitCode = 0
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			result.Signal = status.Signal().String()
		}
		return result, nil
	}

	return result, waitErr
}

func mergeEnv(extra map[string]string) []string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		parts := splitEnv(item)
		env[parts[0]] = parts[1]
	}
	for k, v := range extra {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func splitEnv(item string) [2]string {
	for i := 0; i < len(item); i++ {
		if item[i] == '=' {
			return [2]string{item[:i], item[i+1:]}
		}
	}
	return [2]string{item, ""}
}

func streamPipe(ctx context.Context, reader io.Reader, stream string, spec CommandSpec, handler IOHandler) int {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, spec.LogLineMaxBytes)
	maxBuf := spec.LogLineMaxBytes
	if maxBuf < 64*1024 {
		maxBuf = 64 * 1024
	}
	scanner.Buffer(buf, maxBuf)

	lineNo := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return lineNo
		default:
		}
		lineNo++
		line := scanner.Text()
		if spec.LogLineMaxBytes > 0 && len(line) > spec.LogLineMaxBytes {
			line = line[:spec.LogLineMaxBytes]
		}
		if handler != nil {
			_ = handler.OnLog(ctx, model.StructuredLog{
				Timestamp:    time.Now().UTC(),
				RunID:        spec.RunID,
				Attempt:      spec.Attempt,
				Phase:        spec.Phase,
				CommandClass: spec.CommandClass,
				Stream:       stream,
				LineNo:       lineNo,
				Line:         Redact(line),
			})
		}
	}
	return lineNo
}
