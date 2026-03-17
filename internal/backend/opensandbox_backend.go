package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
	osclient "github.com/lotosli/sandbox-runner/internal/opensandbox/client"
)

type OpenSandboxBackend struct {
	client       *osclient.Client
	cfg          model.RunConfig
	pollInterval time.Duration
	mu           sync.Mutex
	execs        map[string]*openSandboxExecSession
}

type openSandboxExecSession struct {
	logs chan LogChunk

	mu     sync.Mutex
	status ExecStatus
	done   chan struct{}
	closed bool
}

func NewOpenSandboxBackend(cfg model.RunConfig) *OpenSandboxBackend {
	pollInterval := time.Duration(cfg.OpenSandbox.PollIntervalMs) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &OpenSandboxBackend{
		client: osclient.New(osclient.Config{
			BaseURL:    cfg.OpenSandbox.BaseURL,
			APIKey:     cfg.OpenSandbox.APIKey,
			Timeout:    30 * time.Second,
			MaxRetries: 0,
		}),
		cfg:          cfg,
		pollInterval: pollInterval,
		execs:        map[string]*openSandboxExecSession{},
	}
}

func (b *OpenSandboxBackend) Kind() model.BackendKind { return model.BackendKindOpenSandbox }

func (b *OpenSandboxBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return model.BackendCapabilities{
		SupportsPauseResume:    true,
		SupportsTTL:            true,
		SupportsFileUpload:     true,
		SupportsFileDownload:   true,
		SupportsBackgroundExec: true,
		SupportsStreamLogs:     true,
		SupportsEndpoints:      true,
		SupportsBridgeNetwork:  true,
		SupportsHostNetwork:    true,
		SupportsCodeInterp:     true,
		SupportsRuntimeProfile: true,
	}, nil
}

func (b *OpenSandboxBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	info := model.RuntimeInfo{
		ProviderKind:     string(model.BackendKindOpenSandbox),
		RuntimeProfile:   string(b.cfg.Runtime.Profile),
		RuntimeClassName: b.cfg.Kata.RuntimeClassName,
		ContainerRuntime: "opensandbox-" + string(b.cfg.OpenSandbox.Runtime),
		HostOS:           runtime.GOOS,
		HostArch:         runtime.GOARCH,
		Available:        true,
	}
	if b.cfg.Runtime.Profile == model.RuntimeProfileKata {
		info.Virtualization = "kata"
		info.CheckedBy = "provider-capabilities"
		info.Detail = "runtime profile request will be delegated to opensandbox provider"
	} else {
		info.Virtualization = "none"
	}
	return info, nil
}

func (b *OpenSandboxBackend) Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error) {
	env := map[string]*string{}
	for key, value := range req.Env {
		v := value
		env[key] = &v
	}

	extensions := map[string]string{}
	if b.cfg.OpenSandbox.Runtime != "" {
		extensions["runtime"] = string(b.cfg.OpenSandbox.Runtime)
	}
	if req.NetworkMode != "" {
		extensions["network_mode"] = req.NetworkMode
	}
	if req.WorkspaceDir != "" {
		extensions["workspace_dir"] = req.WorkspaceDir
	}

	createReq := osclient.OSSandboxCreateRequest{
		Image: osclient.OSImageSpec{
			URI: req.Image,
		},
		Timeout:        maxInt(req.TimeoutSec, b.cfg.OpenSandbox.TTLSec),
		ResourceLimits: osclient.OSResourceLimits{},
		Env:            env,
		Metadata:       req.Metadata,
		Entrypoint:     nonEmptyEntrypoint(req.Entrypoint),
		Extensions:     extensions,
	}
	if createReq.Metadata == nil {
		createReq.Metadata = map[string]string{}
	}
	if b.cfg.Runtime.Profile != "" {
		createReq.Metadata["runtime.profile"] = string(b.cfg.Runtime.Profile)
	}
	if b.cfg.Runtime.Profile == model.RuntimeProfileKata {
		createReq.Metadata["runtime.class"] = b.cfg.Kata.RuntimeClassName
		extensions["runtime.profile"] = string(model.RuntimeProfileKata)
		if b.cfg.Kata.RuntimeClassName != "" {
			extensions["runtime.class"] = b.cfg.Kata.RuntimeClassName
		}
	}
	if req.CPU != "" {
		createReq.ResourceLimits["cpu"] = req.CPU
	}
	if req.Memory != "" {
		createReq.ResourceLimits["memory"] = req.Memory
	}

	info, err := b.client.CreateSandbox(ctx, createReq)
	if err != nil {
		return SandboxInfo{}, wrapProviderError(model.ErrorCodeSandboxCreateFailed, err)
	}
	return mapSandboxInfo(info, b.cfg), nil
}

func (b *OpenSandboxBackend) Start(ctx context.Context, sandboxID string) error {
	timeout := time.Duration(maxInt(b.cfg.OpenSandbox.CreateTimeoutSec, 120)) * time.Second
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		info, err := b.client.GetSandbox(deadlineCtx, sandboxID)
		if err != nil {
			return wrapProviderError(model.ErrorCodeSandboxStartFailed, err)
		}
		switch strings.ToLower(info.Status.State) {
		case "running", "paused":
			return nil
		case "failed", "terminated":
			return model.RunnerError{
				Code:        string(model.ErrorCodeSandboxStartFailed),
				Message:     fmt.Sprintf("sandbox %s entered %s: %s", sandboxID, info.Status.State, info.Status.Message),
				BackendKind: string(model.BackendKindOpenSandbox),
			}
		}

		select {
		case <-deadlineCtx.Done():
			return deadlineCtx.Err()
		case <-ticker.C:
		}
	}
}

func (b *OpenSandboxBackend) Stat(ctx context.Context, sandboxID string) (SandboxStatus, error) {
	info, err := b.client.GetSandbox(ctx, sandboxID)
	if err != nil {
		return SandboxStatus{}, wrapProviderError(model.ErrorCodeSandboxStartFailed, err)
	}
	return SandboxStatus{
		ID:        info.ID,
		Status:    info.Status.State,
		ExpiresAt: info.ExpiresAt,
	}, nil
}

func (b *OpenSandboxBackend) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error) {
	stream, err := b.client.RunCommandStream(ctx, sandboxID, osclient.OSExecRequest{
		Command:    req.Command,
		Cwd:        req.Cwd,
		Background: false,
		TimeoutMs:  req.Timeout.Milliseconds(),
		Envs:       req.Env,
	})
	if err != nil {
		return ExecHandle{}, wrapProviderError(model.ErrorCodeSandboxExecFailed, err)
	}

	session := newOpenSandboxExecSession()
	initCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go b.consumeExecStream(ctx, stream, session, initCh, errCh)

	select {
	case commandID := <-initCh:
		session.setID(commandID)
		b.setExecSession(commandID, session)
		return ExecHandle{
			ExecID:   commandID,
			Status:   "running",
			Provider: "opensandbox",
		}, nil
	case err := <-errCh:
		return ExecHandle{}, wrapProviderError(model.ErrorCodeSandboxExecFailed, err)
	case <-ctx.Done():
		_ = stream.Close()
		return ExecHandle{}, ctx.Err()
	}
}

func (b *OpenSandboxBackend) ExecStatus(ctx context.Context, sandboxID string, execID string) (ExecStatus, error) {
	status, err := b.client.GetCommandStatus(ctx, sandboxID, execID)
	if err != nil {
		if session, ok := b.getExecSession(execID); ok {
			return session.snapshot(execID), nil
		}
		return ExecStatus{}, wrapProviderError(model.ErrorCodeSandboxExecFailed, err)
	}

	out := ExecStatus{
		ID:         defaultString(status.ID, execID),
		Running:    status.Running,
		ExitCode:   status.ExitCode,
		Error:      status.Error,
		StartedAt:  status.StartedAt,
		FinishedAt: status.FinishedAt,
	}
	if session, ok := b.getExecSession(execID); ok {
		snap := session.snapshot(execID)
		if out.ExitCode == nil {
			out.ExitCode = snap.ExitCode
		}
		if out.Error == "" {
			out.Error = snap.Error
		}
		if out.StartedAt.IsZero() {
			out.StartedAt = snap.StartedAt
		}
		if out.FinishedAt == nil {
			out.FinishedAt = snap.FinishedAt
		}
		if !out.Running {
			b.deleteExecSession(execID)
		}
	}
	return out, nil
}

func (b *OpenSandboxBackend) CancelExec(ctx context.Context, sandboxID string, execID string) error {
	return wrapProviderError(model.ErrorCodeSandboxExecFailed, b.client.InterruptCommand(ctx, sandboxID, execID))
}

func (b *OpenSandboxBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error) {
	if session, ok := b.getExecSession(execID); ok {
		return session.logs, nil
	}

	ch := make(chan LogChunk, 16)
	go func() {
		defer close(ch)

		var (
			cursor int64
			ticker = time.NewTicker(b.pollInterval)
		)
		defer ticker.Stop()

		for {
			output, nextCursor, err := b.client.GetBackgroundCommandLogs(ctx, sandboxID, execID, cursor)
			if err == nil && output != "" {
				for _, line := range splitNonEmptyLines(output) {
					select {
					case <-ctx.Done():
						return
					case ch <- LogChunk{Timestamp: time.Now().UTC(), Stream: "stdout", Line: line}:
					}
				}
			}
			cursor = nextCursor

			status, statusErr := b.client.GetCommandStatus(ctx, sandboxID, execID)
			if statusErr == nil && !status.Running {
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ch, nil
}

func (b *OpenSandboxBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	return wrapProviderError(model.ErrorCodeSandboxUploadFailed, b.client.UploadFile(ctx, sandboxID, localPath, remotePath))
}

func (b *OpenSandboxBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	return wrapProviderError(model.ErrorCodeSandboxDownloadFailed, b.client.DownloadFile(ctx, sandboxID, remotePath, localPath))
}

func (b *OpenSandboxBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_, err := b.client.RenewSandbox(ctx, sandboxID, time.Now().UTC().Add(ttl))
	return wrapProviderError(model.ErrorCodeSandboxRenewFailed, err)
}

func (b *OpenSandboxBackend) Pause(ctx context.Context, sandboxID string) error {
	return wrapProviderError(model.ErrorCodeSandboxPauseFailed, b.client.PauseSandbox(ctx, sandboxID))
}

func (b *OpenSandboxBackend) Resume(ctx context.Context, sandboxID string) error {
	return wrapProviderError(model.ErrorCodeSandboxResumeFailed, b.client.ResumeSandbox(ctx, sandboxID))
}

func (b *OpenSandboxBackend) Delete(ctx context.Context, sandboxID string) error {
	return wrapProviderError(model.ErrorCodeSandboxDeleteFailed, b.client.DeleteSandbox(ctx, sandboxID))
}

func (b *OpenSandboxBackend) SyncWorkspaceIn(ctx context.Context, sandboxID, localDir string) error {
	return b.syncInTar(ctx, sandboxID, localDir, b.cfg.OpenSandbox.WorkspaceRoot)
}

func (b *OpenSandboxBackend) SyncWorkspaceOut(ctx context.Context, sandboxID, remoteDir, localDir string) error {
	return b.syncOutTar(ctx, sandboxID, remoteDir, localDir)
}

func (b *OpenSandboxBackend) DownloadArtifact(ctx context.Context, sandboxID, remotePath, localPath string) error {
	return wrapProviderError(model.ErrorCodeSandboxDownloadFailed, b.client.DownloadFile(ctx, sandboxID, remotePath, localPath))
}

func (b *OpenSandboxBackend) SandboxMetadata(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	info, err := b.client.GetSandbox(ctx, sandboxID)
	if err != nil {
		return SandboxInfo{}, wrapProviderError(model.ErrorCodeSandboxStartFailed, err)
	}
	return mapSandboxInfo(info, b.cfg), nil
}

func (b *OpenSandboxBackend) Endpoints(ctx context.Context, sandboxID string, ports []int) ([]model.Endpoint, error) {
	out := make([]model.Endpoint, 0, len(ports))
	for _, port := range ports {
		endpoint, err := b.client.GetSandboxEndpoint(ctx, sandboxID, port, true)
		if err != nil {
			return nil, wrapProviderError(model.ErrorCodeSandboxStartFailed, err)
		}
		out = append(out, model.Endpoint{
			ContainerPort: port,
			URL:           endpoint.Endpoint,
			Headers:       endpoint.Headers,
		})
	}
	return out, nil
}

func (b *OpenSandboxBackend) RunSimpleCommand(ctx context.Context, sandboxID, command, cwd string, env map[string]string, timeout time.Duration) (int, string, error) {
	stream, err := b.client.RunCommandStream(ctx, sandboxID, osclient.OSExecRequest{
		Command:    command,
		Cwd:        cwd,
		Background: false,
		TimeoutMs:  timeout.Milliseconds(),
		Envs:       env,
	})
	if err != nil {
		return 0, "", err
	}
	defer stream.Close()

	var (
		commandID string
		stderrBuf strings.Builder
		exitCode  = 0
	)
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		event, ok := decodeEvent(scanner.Bytes())
		if !ok {
			continue
		}
		switch event.Type {
		case "init":
			commandID = event.Text
		case "stderr", "error":
			if event.Text != "" {
				stderrBuf.WriteString(event.Text)
			}
			if event.Error != nil {
				if event.Error.EValue != "" {
					if parsed, err := strconv.Atoi(event.Error.EValue); err == nil {
						exitCode = parsed
					} else {
						exitCode = 1
					}
				} else {
					exitCode = 1
				}
				if stderrBuf.Len() == 0 {
					stderrBuf.WriteString(event.Error.EName + ":" + event.Error.EValue)
				}
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return exitCode, stderrBuf.String(), scanErr
	}
	if commandID != "" {
		status, err := b.client.GetCommandStatus(ctx, sandboxID, commandID)
		if err == nil && status.ExitCode != nil {
			exitCode = *status.ExitCode
		}
		if err == nil && status.Error != "" && stderrBuf.Len() == 0 {
			stderrBuf.WriteString(status.Error)
		}
	}
	return exitCode, stderrBuf.String(), nil
}

func mapSandboxInfo(info osclient.OSSandboxInfo, cfg model.RunConfig) SandboxInfo {
	return SandboxInfo{
		ID:          info.ID,
		Status:      info.Status.State,
		RuntimeKind: string(cfg.OpenSandbox.Runtime),
		Metadata:    info.Metadata,
		ExpiresAt:   info.ExpiresAt,
	}
}

func decodeEvent(raw []byte) (osclient.OSServerStreamEvent, bool) {
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return osclient.OSServerStreamEvent{}, false
	}
	if strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	}
	var event osclient.OSServerStreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return osclient.OSServerStreamEvent{}, false
	}
	return event, true
}

func splitNonEmptyLines(text string) []string {
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func newOpenSandboxExecSession() *openSandboxExecSession {
	return &openSandboxExecSession{
		logs: make(chan LogChunk, 64),
		status: ExecStatus{
			Running:   true,
			StartedAt: time.Now().UTC(),
		},
		done: make(chan struct{}),
	}
}

func (s *openSandboxExecSession) setID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.ID = id
}

func (s *openSandboxExecSession) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if message != "" {
		s.status.Error = message
	}
}

func (s *openSandboxExecSession) setExitCode(exitCode int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.ExitCode = &exitCode
}

func (s *openSandboxExecSession) finish() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.status.Running = false
	finishedAt := time.Now().UTC()
	s.status.FinishedAt = &finishedAt
	s.mu.Unlock()

	close(s.logs)
	close(s.done)
}

func (s *openSandboxExecSession) snapshot(execID string) ExecStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	if status.ID == "" {
		status.ID = execID
	}
	return status
}

func (s *openSandboxExecSession) emit(ctx context.Context, chunk LogChunk) bool {
	select {
	case <-ctx.Done():
		return false
	case s.logs <- chunk:
		return true
	}
}

func (b *OpenSandboxBackend) consumeExecStream(ctx context.Context, stream io.ReadCloser, session *openSandboxExecSession, initCh chan<- string, errCh chan<- error) {
	defer stream.Close()
	defer session.finish()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	stdoutBuf := &strings.Builder{}
	stderrBuf := &strings.Builder{}
	initSent := false

	for scanner.Scan() {
		event, ok := decodeEvent(scanner.Bytes())
		if !ok {
			continue
		}
		ts := time.Now().UTC()
		if event.Timestamp > 0 {
			ts = time.UnixMilli(event.Timestamp).UTC()
		}
		switch event.Type {
		case "init":
			if event.Text != "" && !initSent {
				initSent = true
				initCh <- event.Text
			}
		case "stdout":
			if !emitBufferedLogChunks(ctx, session, stdoutBuf, "stdout", event.Text, ts, false) {
				return
			}
		case "stderr":
			if !emitBufferedLogChunks(ctx, session, stderrBuf, "stderr", event.Text, ts, false) {
				return
			}
		case "error":
			if event.Error != nil {
				message := joinProviderError(*event.Error)
				session.setError(message)
				if event.Error.EValue != "" {
					if parsed, err := strconv.Atoi(event.Error.EValue); err == nil {
						session.setExitCode(parsed)
					}
				}
				if message != "" && !emitBufferedLogChunks(ctx, session, stderrBuf, "stderr", message+"\n", ts, false) {
					return
				}
				for _, trace := range event.Error.Traceback {
					if trace == "" {
						continue
					}
					if !emitBufferedLogChunks(ctx, session, stderrBuf, "stderr", trace+"\n", ts, false) {
						return
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		if !initSent {
			errCh <- err
			return
		}
		session.setError(err.Error())
	}
	if !initSent {
		errCh <- fmt.Errorf("opensandbox execd stream missing init event")
		return
	}
	_ = emitBufferedLogChunks(ctx, session, stdoutBuf, "stdout", "", time.Now().UTC(), true)
	_ = emitBufferedLogChunks(ctx, session, stderrBuf, "stderr", "", time.Now().UTC(), true)
}

func emitBufferedLogChunks(ctx context.Context, session *openSandboxExecSession, buf *strings.Builder, stream string, text string, ts time.Time, flush bool) bool {
	if text != "" {
		_, _ = buf.WriteString(text)
	}
	remaining := buf.String()
	lines := make([]string, 0)
	for {
		idx := strings.IndexByte(remaining, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(remaining[:idx], "\r")
		if line != "" {
			lines = append(lines, line)
		}
		remaining = remaining[idx+1:]
	}
	buf.Reset()
	if flush {
		remaining = strings.TrimRight(remaining, "\r\n")
		if remaining != "" {
			lines = append(lines, remaining)
			remaining = ""
		}
	}
	_, _ = buf.WriteString(remaining)
	for _, line := range lines {
		if !session.emit(ctx, LogChunk{Timestamp: ts, Stream: stream, Line: line}) {
			return false
		}
	}
	return true
}

func joinProviderError(err osclient.OSExecError) string {
	parts := make([]string, 0, 2)
	if err.EName != "" {
		parts = append(parts, err.EName)
	}
	if err.EValue != "" {
		parts = append(parts, err.EValue)
	}
	return strings.TrimSpace(strings.Join(parts, ": "))
}

func (b *OpenSandboxBackend) setExecSession(execID string, session *openSandboxExecSession) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.execs[execID] = session
}

func (b *OpenSandboxBackend) getExecSession(execID string) (*openSandboxExecSession, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.execs[execID]
	return session, ok
}

func (b *OpenSandboxBackend) deleteExecSession(execID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.execs, execID)
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func wrapProviderError(code model.ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	var providerErr osclient.ProviderError
	if errors.As(err, &providerErr) {
		return model.RunnerError{
			Code:         string(code),
			Message:      providerErr.Error(),
			Retryable:    providerErr.Retryable,
			BackendKind:  string(model.BackendKindOpenSandbox),
			ProviderCode: providerErr.Code,
			Cause:        err,
		}
	}
	return model.RunnerError{
		Code:        string(code),
		Message:     err.Error(),
		BackendKind: string(model.BackendKindOpenSandbox),
		Cause:       err,
	}
}

func nonEmptyEntrypoint(entrypoint []string) []string {
	if len(entrypoint) == 0 {
		return nil
	}
	if len(entrypoint) == 2 && (entrypoint[0] == "/bin/sh" || entrypoint[0] == "sh") && entrypoint[1] == "-lc" {
		return nil
	}
	return entrypoint
}

func maxInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

var _ SandboxBackend = (*OpenSandboxBackend)(nil)
var _ WorkspaceSyncer = (*OpenSandboxBackend)(nil)
var _ MetadataProvider = (*OpenSandboxBackend)(nil)
var _ ArtifactDownloader = (*OpenSandboxBackend)(nil)
var _ ExecStatusProvider = (*OpenSandboxBackend)(nil)
var _ ExecCanceler = (*OpenSandboxBackend)(nil)

func remoteTempPath(root, name string) string {
	return path.Join(root, ".sandbox-runner", name)
}
