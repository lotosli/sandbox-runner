package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type DevContainerBackend struct {
	cfg       model.RunConfig
	cliPath   string
	mu        sync.Mutex
	sandboxes map[string]devContainerSandbox
	execs     map[string]*devContainerExecSession
}

type devContainerSandbox struct {
	info    SandboxInfo
	summary model.DevContainerArtifact
}

type devContainerExecSession struct {
	logs chan LogChunk

	mu     sync.Mutex
	status ExecStatus
	cmd    *exec.Cmd
	closed bool
}

func NewDevContainerBackend(cfg model.RunConfig) (*DevContainerBackend, error) {
	cliPath, err := exec.LookPath(cfg.DevContainer.CLIPath)
	if err != nil {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerCLINotFound),
			Message:     fmt.Sprintf("devcontainer CLI not found: %s", cfg.DevContainer.CLIPath),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}
	return &DevContainerBackend{
		cfg:       cfg,
		cliPath:   cliPath,
		sandboxes: map[string]devContainerSandbox{},
		execs:     map[string]*devContainerExecSession{},
	}, nil
}

func (b *DevContainerBackend) Kind() model.BackendKind { return model.BackendKindDevContainer }

func (b *DevContainerBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return model.BackendCapabilities{
		SupportsStreamLogs:   true,
		SupportsDevContainer: true,
	}, nil
}

func (b *DevContainerBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	return model.RuntimeInfo{
		ProviderKind:     string(model.BackendKindDevContainer),
		RuntimeProfile:   string(model.RuntimeProfileNative),
		ContainerRuntime: "devcontainer",
		HostOS:           runtime.GOOS,
		HostArch:         runtime.GOARCH,
		Virtualization:   "none",
		Available:        true,
	}, nil
}

func (b *DevContainerBackend) Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error) {
	workspacePath := b.workspacePath()
	if workspacePath == "" {
		workspacePath = req.WorkspaceDir
	}
	if workspacePath == "" {
		return SandboxInfo{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerReadConfigFailed),
			Message:     "workspace path is required for devcontainer backend",
			BackendKind: string(model.BackendKindDevContainer),
		}
	}

	sandboxID := fmt.Sprintf("sbx-devcontainer-%s-%d", req.RunID, req.Attempt)
	readTimeout := time.Duration(maxIntValue(req.TimeoutSec, b.cfg.DevContainer.UpTimeoutSec, 300)) * time.Second
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	output, err := b.runCLI(readCtx, workspacePath, b.readConfigurationArgs(workspacePath)...)
	if err != nil {
		return SandboxInfo{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerReadConfigFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}
	summary := summarizeDevContainerConfig(output, b.cfg, workspacePath)

	upCtx, upCancel := context.WithTimeout(ctx, readTimeout)
	defer upCancel()
	if _, err := b.runCLI(upCtx, workspacePath, b.upArgs(workspacePath, sandboxID)...); err != nil {
		return SandboxInfo{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerUpFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}

	if b.cfg.DevContainer.RunUserCommands && !b.cfg.DevContainer.SkipPostCreate {
		userCtx, userCancel := context.WithTimeout(ctx, readTimeout)
		defer userCancel()
		if _, err := b.runCLI(userCtx, workspacePath, b.runUserCommandsArgs(workspacePath)...); err != nil {
			return SandboxInfo{}, model.RunnerError{
				Code:        string(model.ErrorCodeDevContainerUpFailed),
				Message:     err.Error(),
				BackendKind: string(model.BackendKindDevContainer),
				Cause:       err,
			}
		}
	}

	info := SandboxInfo{
		ID:          sandboxID,
		Status:      "running",
		RuntimeKind: "devcontainer",
		Metadata: map[string]string{
			"workspace_folder": summary.WorkspaceFolder,
			"config_path":      summary.ConfigPath,
			"runtime.profile":  string(model.RuntimeProfileNative),
		},
	}

	b.mu.Lock()
	b.sandboxes[sandboxID] = devContainerSandbox{info: info, summary: summary}
	b.mu.Unlock()
	return info, nil
}

func (b *DevContainerBackend) Start(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (b *DevContainerBackend) Stat(ctx context.Context, sandboxID string) (SandboxStatus, error) {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	if sandbox, ok := b.sandboxes[sandboxID]; ok {
		return SandboxStatus{ID: sandbox.info.ID, Status: sandbox.info.Status}, nil
	}
	return SandboxStatus{ID: sandboxID, Status: "unknown"}, nil
}

func (b *DevContainerBackend) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error) {
	sandbox, ok := b.lookupSandbox(sandboxID)
	if !ok {
		return ExecHandle{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     fmt.Sprintf("unknown devcontainer sandbox: %s", sandboxID),
			BackendKind: string(model.BackendKindDevContainer),
		}
	}

	execID := fmt.Sprintf("%s-exec-%d", sandboxID, time.Now().UTC().UnixNano())
	session := &devContainerExecSession{
		logs: make(chan LogChunk, 128),
		status: ExecStatus{
			ID:        execID,
			Running:   true,
			StartedAt: time.Now().UTC(),
		},
	}

	args := b.execArgs(sandbox, req)
	cmd := exec.CommandContext(ctx, b.cliPath, args...)
	cmd.Dir = sandbox.summary.LocalWorkspacePath
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ExecHandle{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ExecHandle{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return ExecHandle{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamLogs(ctx, stdout, "stdout", session)
	}()
	go func() {
		defer wg.Done()
		streamLogs(ctx, stderr, "stderr", session)
	}()
	go func() {
		err := cmd.Wait()
		wg.Wait()
		session.finish(err)
	}()

	b.mu.Lock()
	b.execs[execID] = session
	b.mu.Unlock()
	return ExecHandle{
		ExecID:   execID,
		Status:   "running",
		Provider: "devcontainer",
	}, nil
}

func (b *DevContainerBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error) {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     fmt.Sprintf("unknown devcontainer exec session: %s", execID),
			BackendKind: string(model.BackendKindDevContainer),
		}
	}
	return session.logs, nil
}

func (b *DevContainerBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	_ = ctx
	_ = sandboxID
	_ = localPath
	_ = remotePath
	return nil
}

func (b *DevContainerBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	_ = ctx
	_ = sandboxID
	_ = remotePath
	_ = localPath
	return nil
}

func (b *DevContainerBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_ = ctx
	_ = sandboxID
	_ = ttl
	return errUnsupported
}

func (b *DevContainerBackend) Pause(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *DevContainerBackend) Resume(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *DevContainerBackend) Delete(ctx context.Context, sandboxID string) error {
	sandbox, ok := b.lookupSandbox(sandboxID)
	if !ok {
		return nil
	}
	if strings.EqualFold(b.cfg.DevContainer.CleanupMode, "keep") {
		return nil
	}
	_, err := b.runCLI(ctx, sandbox.summary.LocalWorkspacePath, b.downArgs(sandbox.summary.LocalWorkspacePath)...)
	if err != nil {
		return model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerDownFailed),
			Message:     err.Error(),
			BackendKind: string(model.BackendKindDevContainer),
			Cause:       err,
		}
	}
	return nil
}

func (b *DevContainerBackend) SyncWorkspaceIn(ctx context.Context, sandboxID, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = localDir
	return nil
}

func (b *DevContainerBackend) SyncWorkspaceOut(ctx context.Context, sandboxID, remoteDir, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = remoteDir
	_ = localDir
	return nil
}

func (b *DevContainerBackend) SandboxMetadata(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	_ = ctx
	sandbox, ok := b.lookupSandbox(sandboxID)
	if !ok {
		return SandboxInfo{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     fmt.Sprintf("unknown devcontainer sandbox: %s", sandboxID),
			BackendKind: string(model.BackendKindDevContainer),
		}
	}
	return sandbox.info, nil
}

func (b *DevContainerBackend) Endpoints(ctx context.Context, sandboxID string, ports []int) ([]model.Endpoint, error) {
	_ = ctx
	_ = sandboxID
	_ = ports
	return nil, nil
}

func (b *DevContainerBackend) DevContainerMetadata(ctx context.Context, sandboxID string) (model.DevContainerArtifact, error) {
	_ = ctx
	sandbox, ok := b.lookupSandbox(sandboxID)
	if !ok {
		return model.DevContainerArtifact{}, model.RunnerError{
			Code:        string(model.ErrorCodeDevContainerExecFailed),
			Message:     fmt.Sprintf("unknown devcontainer sandbox: %s", sandboxID),
			BackendKind: string(model.BackendKindDevContainer),
		}
	}
	return sandbox.summary, nil
}

func (b *DevContainerBackend) ExecStatus(ctx context.Context, sandboxID string, execID string) (ExecStatus, error) {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return ExecStatus{ID: execID, Running: false}, nil
	}
	status := session.snapshot()
	if !status.Running {
		b.deleteExec(execID)
	}
	return status, nil
}

func (b *DevContainerBackend) CancelExec(ctx context.Context, sandboxID string, execID string) error {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.cmd == nil || session.cmd.Process == nil {
		return nil
	}
	return session.cmd.Process.Kill()
}

func (b *DevContainerBackend) workspacePath() string {
	if b.cfg.DevContainer.WorkspaceFolder != "" {
		return b.cfg.DevContainer.WorkspaceFolder
	}
	return b.cfg.Run.WorkspaceDir
}

func (b *DevContainerBackend) lookupSandbox(sandboxID string) (devContainerSandbox, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sandbox, ok := b.sandboxes[sandboxID]
	return sandbox, ok
}

func (b *DevContainerBackend) deleteExec(execID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.execs, execID)
}

func (b *DevContainerBackend) runCLI(ctx context.Context, workspacePath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.cliPath, args...)
	if workspacePath != "" {
		cmd.Dir = workspacePath
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return nil, errors.New(text)
	}
	return output, nil
}

func (b *DevContainerBackend) commonArgs(workspacePath string) []string {
	args := []string{"--workspace-folder", workspacePath}
	if b.cfg.DevContainer.ConfigPath != "" {
		args = append(args, "--config", b.cfg.DevContainer.ConfigPath)
	}
	if b.cfg.DevContainer.LogLevel != "" {
		args = append(args, "--log-level", b.cfg.DevContainer.LogLevel)
	}
	if b.cfg.DevContainer.MountWorkspaceGitRoot {
		args = append(args, "--mount-workspace-git-root")
	}
	return args
}

func (b *DevContainerBackend) readConfigurationArgs(workspacePath string) []string {
	return append([]string{"read-configuration"}, b.commonArgs(workspacePath)...)
}

func (b *DevContainerBackend) upArgs(workspacePath, sandboxID string) []string {
	args := append([]string{"up"}, b.commonArgs(workspacePath)...)
	if b.cfg.DevContainer.RemoveExistingContainer {
		args = append(args, "--remove-existing-container")
	}
	if b.cfg.DevContainer.SkipPostCreate {
		args = append(args, "--skip-post-create")
	}
	for _, label := range b.idLabels(sandboxID) {
		args = append(args, "--id-label", label)
	}
	return args
}

func (b *DevContainerBackend) runUserCommandsArgs(workspacePath string) []string {
	return append([]string{"run-user-commands"}, b.commonArgs(workspacePath)...)
}

func (b *DevContainerBackend) downArgs(workspacePath string) []string {
	return append([]string{"down"}, b.commonArgs(workspacePath)...)
}

func (b *DevContainerBackend) execArgs(sandbox devContainerSandbox, req ExecRequest) []string {
	args := append([]string{"exec"}, b.commonArgs(sandbox.summary.LocalWorkspacePath)...)
	script := req.Command
	if translated := b.translateCwd(sandbox, req.Cwd); translated != "" {
		script = "cd " + devcontainerShellQuote(translated) + " && " + req.Command
	}
	args = append(args, "--", "/usr/bin/env")
	keys := make([]string, 0, len(req.Env))
	for key := range req.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, key+"="+req.Env[key])
	}
	args = append(args, "/bin/sh", "-lc", script)
	return args
}

func (b *DevContainerBackend) idLabels(sandboxID string) []string {
	prefix := strings.TrimSpace(b.cfg.DevContainer.IDLabelPrefix)
	if prefix == "" {
		prefix = "ai-sandbox-runner"
	}
	return []string{
		fmt.Sprintf("%s.run_id=%s", prefix, b.cfg.Run.RunID),
		fmt.Sprintf("%s.sandbox_id=%s", prefix, sandboxID),
	}
}

func streamLogs(ctx context.Context, reader io.Reader, stream string, session *devContainerExecSession) {
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

func (s *devContainerExecSession) finish(err error) {
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

func (s *devContainerExecSession) snapshot() ExecStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func summarizeDevContainerConfig(output []byte, cfg model.RunConfig, workspacePath string) model.DevContainerArtifact {
	summary := model.DevContainerArtifact{
		CLIPath:            cfg.DevContainer.CLIPath,
		ConfigPath:         cfg.DevContainer.ConfigPath,
		LocalWorkspacePath: workspacePath,
	}
	payload := strings.TrimSpace(string(output))
	if payload == "" {
		return summary
	}

	root := map[string]any{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return summary
	}
	if value, ok := lookupValue(root, "workspaceFolder"); ok {
		summary.WorkspaceFolder = stringify(value)
	}
	if value, ok := lookupValue(root, "postCreateCommand"); ok {
		summary.HasPostCreate = hasMeaningfulValue(value)
	}
	if value, ok := lookupValue(root, "postStartCommand"); ok {
		summary.HasPostStart = hasMeaningfulValue(value)
	}
	if value, ok := lookupValue(root, "features"); ok {
		summary.Features = stringifyFeatures(value)
	}
	if summary.WorkspaceFolder == "" {
		summary.WorkspaceFolder = cfg.OpenSandbox.WorkspaceRoot
		if summary.WorkspaceFolder == "" {
			summary.WorkspaceFolder = "/workspace"
		}
	}
	return summary
}

func (b *DevContainerBackend) translateCwd(sandbox devContainerSandbox, cwd string) string {
	if cwd == "" {
		return ""
	}
	pseudoRoot := strings.TrimSpace(b.cfg.OpenSandbox.WorkspaceRoot)
	if pseudoRoot == "" {
		pseudoRoot = "/workspace"
	}
	targetRoot := sandbox.summary.WorkspaceFolder
	if targetRoot == "" {
		targetRoot = pseudoRoot
	}
	if cwd == pseudoRoot {
		return targetRoot
	}
	if strings.HasPrefix(cwd, pseudoRoot+"/") {
		suffix := strings.TrimPrefix(cwd, pseudoRoot+"/")
		return path.Join(targetRoot, suffix)
	}
	return cwd
}

func lookupValue(root map[string]any, key string) (any, bool) {
	if value, ok := root[key]; ok {
		return value, true
	}
	for _, value := range root {
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if found, ok := lookupValue(nested, key); ok {
			return found, true
		}
	}
	return nil, false
}

func hasMeaningfulValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return value != nil
	}
}

func stringify(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func stringifyFeatures(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringify(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func maxIntValue(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func devcontainerShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

var _ SandboxBackend = (*DevContainerBackend)(nil)
var _ WorkspaceSyncer = (*DevContainerBackend)(nil)
var _ MetadataProvider = (*DevContainerBackend)(nil)
var _ DevContainerMetadataProvider = (*DevContainerBackend)(nil)
var _ ExecStatusProvider = (*DevContainerBackend)(nil)
var _ ExecCanceler = (*DevContainerBackend)(nil)
