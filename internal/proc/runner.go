package proc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	env := mergedEnv(spec.Env)
	commandPath, err := resolveCommandPath(spec.Command[0], env, spec.Dir)
	if err != nil {
		now := time.Now().UTC()
		return Result{StartedAt: now, FinishedAt: now}, err
	}

	cmd := exec.CommandContext(runCtx, commandPath, spec.Command[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = envList(env)

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

func mergedEnv(extra map[string]string) map[string]string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		parts := splitEnv(item)
		env[parts[0]] = parts[1]
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func envList(env map[string]string) []string {
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

func resolveCommandPath(name string, env map[string]string, dir string) (string, error) {
	if name == "" {
		return "", exec.ErrNotFound
	}
	if strings.ContainsRune(name, os.PathSeparator) || filepath.IsAbs(name) {
		return name, nil
	}
	pathValue := env["PATH"]
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	if runtime.GOOS == "windows" {
		return resolveCommandPathWindows(name, pathValue, dir, env["PATHEXT"])
	}
	for _, entry := range filepath.SplitList(pathValue) {
		if entry == "" {
			entry = "."
		}
		base := entry
		if !filepath.IsAbs(base) && dir != "" {
			base = filepath.Join(dir, base)
		}
		candidate := filepath.Join(base, name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func ResolveCommandPath(name string, dir string, extraEnv map[string]string) (string, error) {
	return resolveCommandPath(name, mergedEnv(extraEnv), dir)
}

func resolveCommandPathWindows(name, pathValue, dir, pathExt string) (string, error) {
	extensions := []string{""}
	if filepath.Ext(name) == "" {
		if pathExt == "" {
			pathExt = ".COM;.EXE;.BAT;.CMD"
		}
		for _, ext := range strings.Split(pathExt, ";") {
			if ext == "" {
				continue
			}
			extensions = append(extensions, ext)
		}
	}
	for _, entry := range filepath.SplitList(pathValue) {
		if entry == "" {
			entry = "."
		}
		base := entry
		if !filepath.IsAbs(base) && dir != "" {
			base = filepath.Join(dir, base)
		}
		for _, ext := range extensions {
			candidate := filepath.Join(base, name)
			if ext != "" && !strings.HasSuffix(strings.ToLower(candidate), strings.ToLower(ext)) {
				candidate += ext
			}
			if isExistingFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
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
