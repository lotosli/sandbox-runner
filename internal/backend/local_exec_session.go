package backend

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

type localExecSession struct {
	logs chan LogChunk

	mu     sync.Mutex
	status ExecStatus
	cmd    *exec.Cmd
	closed bool
}

func newLocalExecSession(execID string) *localExecSession {
	return &localExecSession{
		logs: make(chan LogChunk, 128),
		status: ExecStatus{
			ID:        execID,
			Running:   true,
			StartedAt: time.Now().UTC(),
		},
	}
}

func (s *localExecSession) start(ctx context.Context, cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	s.cmd = cmd
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamSessionLogs(ctx, stdout, "stdout", s)
	}()
	go func() {
		defer wg.Done()
		streamSessionLogs(ctx, stderr, "stderr", s)
	}()
	go func() {
		err := cmd.Wait()
		wg.Wait()
		s.finish(err)
	}()
	return nil
}

func (s *localExecSession) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.status.Running = false
	finished := time.Now().UTC()
	s.status.FinishedAt = &finished
	if err == nil {
		code := 0
		s.status.ExitCode = &code
		close(s.logs)
		return
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code := exitErr.ExitCode()
		s.status.ExitCode = &code
	case errors.Is(err, context.DeadlineExceeded):
		code := 124
		s.status.ExitCode = &code
	default:
		s.status.Error = err.Error()
		code := 1
		s.status.ExitCode = &code
	}
	close(s.logs)
}

func (s *localExecSession) snapshot() ExecStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *localExecSession) cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}

func streamSessionLogs(ctx context.Context, reader io.Reader, stream string, session *localExecSession) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		case session.logs <- LogChunk{
			Timestamp: time.Now().UTC(),
			Stream:    stream,
			Line:      scanner.Text(),
		}:
		}
	}
}
