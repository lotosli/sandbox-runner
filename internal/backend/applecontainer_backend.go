package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type AppleContainerBackend struct {
	cfg       model.RunConfig
	binary    string
	mu        sync.Mutex
	sandboxes map[string]appleContainerRecord
	execs     map[string]*localExecSession
}

type appleContainerRecord struct {
	info  SandboxInfo
	image string
}

func NewAppleContainerBackend(cfg model.RunConfig) (*AppleContainerBackend, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeAppleContainerUnsupported),
			Message:     "apple-container backend is supported only on darwin/arm64",
			BackendKind: string(model.BackendKindAppleContainer),
		}
	}
	binary, err := exec.LookPath(cfg.AppleContainer.Binary)
	if err != nil {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeAppleContainerBinaryNotFound),
			Message:     fmt.Sprintf("apple container CLI not found: %s", cfg.AppleContainer.Binary),
			BackendKind: string(model.BackendKindAppleContainer),
			Cause:       err,
		}
	}
	return &AppleContainerBackend{
		cfg:       cfg,
		binary:    binary,
		sandboxes: map[string]appleContainerRecord{},
		execs:     map[string]*localExecSession{},
	}, nil
}

func (b *AppleContainerBackend) Kind() model.BackendKind { return model.BackendKindAppleContainer }

func (b *AppleContainerBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return model.BackendCapabilities{
		SupportsFileUpload:     true,
		SupportsFileDownload:   true,
		SupportsStreamLogs:     true,
		SupportsRuntimeProfile: true,
		SupportsOCIImage:       true,
		SupportsVMIsolation:    true,
	}, nil
}

func (b *AppleContainerBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	return model.RuntimeInfo{
		ProviderKind:     string(model.BackendKindAppleContainer),
		BackendProvider:  "apple-container",
		RuntimeProfile:   string(model.RuntimeProfileAppleContainer),
		ContainerRuntime: "apple-container",
		HostOS:           runtime.GOOS,
		HostArch:         runtime.GOARCH,
		Virtualization:   "apple-container",
		LocalPlatform:    "macos",
		Available:        true,
	}, nil
}

func (b *AppleContainerBackend) Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error) {
	workspace, err := filepath.Abs(req.WorkspaceDir)
	if err != nil {
		return SandboxInfo{}, wrapAppleContainerErr(model.ErrorCodeAppleContainerCreateFailed, err)
	}
	image := req.Image
	if image == "" {
		image = b.cfg.AppleContainer.Image
	}
	if image == "" {
		return SandboxInfo{}, model.RunnerError{
			Code:        string(model.ErrorCodeAppleContainerCreateFailed),
			Message:     "apple-container image is required",
			BackendKind: string(model.BackendKindAppleContainer),
		}
	}
	sandboxID := fmt.Sprintf("ac-%s-%d", req.RunID, req.Attempt)
	args := []string{
		"create",
		"--name", sandboxID,
		"--workdir", b.workspaceRoot(),
		"--volume", workspace + ":" + b.workspaceRoot(),
	}
	for _, entry := range sortedEnv(req.Env) {
		args = append(args, "-e", entry)
	}
	if req.CPU != "" {
		args = append(args, "--cpus", req.CPU)
	}
	if req.Memory != "" {
		args = append(args, "--memory", req.Memory)
	}
	for _, label := range b.labels(req) {
		args = append(args, "--label", label)
	}
	args = append(args, image, "/bin/sh", "-lc", "trap 'exit 0' TERM INT; while true; do sleep 3600; done")
	if _, err := b.runCLI(ctx, "", args...); err != nil {
		return SandboxInfo{}, wrapAppleContainerErr(model.ErrorCodeAppleContainerCreateFailed, err)
	}

	info := SandboxInfo{
		ID:          sandboxID,
		Status:      "created",
		RuntimeKind: "apple-container",
		Metadata: map[string]string{
			"image":             image,
			"workspace_root":    b.workspaceRoot(),
			"runtime.profile":   string(model.RuntimeProfileAppleContainer),
			"backend.provider":  "apple-container",
			"local.platform":    "macos",
			"container_runtime": "apple-container",
		},
	}
	b.mu.Lock()
	b.sandboxes[sandboxID] = appleContainerRecord{info: info, image: image}
	b.mu.Unlock()
	return info, nil
}

func (b *AppleContainerBackend) Start(ctx context.Context, sandboxID string) error {
	_, err := b.runCLI(ctx, "", "start", sandboxID)
	return wrapAppleContainerErr(model.ErrorCodeSandboxStartFailed, err)
}

func (b *AppleContainerBackend) Stat(ctx context.Context, sandboxID string) (SandboxStatus, error) {
	info, err := b.inspect(ctx, sandboxID)
	if err != nil {
		return SandboxStatus{}, err
	}
	return SandboxStatus{ID: info.ID, Status: info.Status, ExpiresAt: info.ExpiresAt}, nil
}

func (b *AppleContainerBackend) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error) {
	execID := fmt.Sprintf("%s-exec-%d", sandboxID, time.Now().UTC().UnixNano())
	session := newLocalExecSession(execID)
	args := []string{"exec"}
	for _, entry := range sortedEnv(req.Env) {
		args = append(args, "-e", entry)
	}
	if cwd := b.translateCwd(req.Cwd); cwd != "" {
		args = append(args, "--workdir", cwd)
	}
	args = append(args, sandboxID, "/bin/sh", "-lc", req.Command)
	cmd := exec.CommandContext(ctx, b.binary, args...)
	if err := session.start(ctx, cmd); err != nil {
		return ExecHandle{}, wrapAppleContainerErr(model.ErrorCodeAppleContainerExecFailed, err)
	}
	b.mu.Lock()
	b.execs[execID] = session
	b.mu.Unlock()
	return ExecHandle{ExecID: execID, Status: "running", Provider: "apple-container"}, nil
}

func (b *AppleContainerBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error) {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeAppleContainerExecFailed),
			Message:     fmt.Sprintf("unknown apple-container exec session: %s", execID),
			BackendKind: string(model.BackendKindAppleContainer),
		}
	}
	return session.logs, nil
}

func (b *AppleContainerBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	_ = ctx
	_ = sandboxID
	_ = localPath
	_ = remotePath
	return nil
}

func (b *AppleContainerBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	_ = ctx
	_ = sandboxID
	_ = remotePath
	_ = localPath
	return nil
}

func (b *AppleContainerBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_ = ctx
	_ = sandboxID
	_ = ttl
	return errUnsupported
}

func (b *AppleContainerBackend) Pause(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *AppleContainerBackend) Resume(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *AppleContainerBackend) Delete(ctx context.Context, sandboxID string) error {
	_, err := b.runCLI(ctx, "", "delete", sandboxID)
	return wrapAppleContainerErr(model.ErrorCodeAppleContainerDeleteFailed, err)
}

func (b *AppleContainerBackend) SyncWorkspaceIn(ctx context.Context, sandboxID, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = localDir
	return nil
}

func (b *AppleContainerBackend) SyncWorkspaceOut(ctx context.Context, sandboxID, remoteDir, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = remoteDir
	_ = localDir
	return nil
}

func (b *AppleContainerBackend) SandboxMetadata(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return b.inspect(ctx, sandboxID)
}

func (b *AppleContainerBackend) Endpoints(ctx context.Context, sandboxID string, ports []int) ([]model.Endpoint, error) {
	_ = ctx
	_ = sandboxID
	_ = ports
	return nil, nil
}

func (b *AppleContainerBackend) ExecStatus(ctx context.Context, sandboxID string, execID string) (ExecStatus, error) {
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

func (b *AppleContainerBackend) CancelExec(ctx context.Context, sandboxID string, execID string) error {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil
	}
	return session.cancel()
}

func (b *AppleContainerBackend) inspect(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	output, err := b.runCLI(ctx, "", "inspect", sandboxID)
	if err != nil {
		if sandbox, ok := b.lookupSandbox(sandboxID); ok {
			return sandbox.info, nil
		}
		return SandboxInfo{}, wrapAppleContainerErr(model.ErrorCodeAppleContainerExecFailed, err)
	}
	if info, ok := parseAppleContainerInspect(output); ok {
		if info.ID == "" {
			info.ID = sandboxID
		}
		if sandbox, ok := b.lookupSandbox(sandboxID); ok {
			if info.Metadata == nil {
				info.Metadata = map[string]string{}
			}
			if info.Metadata["image"] == "" {
				info.Metadata["image"] = sandbox.image
			}
		}
		return info, nil
	}
	if sandbox, ok := b.lookupSandbox(sandboxID); ok {
		return sandbox.info, nil
	}
	return SandboxInfo{ID: sandboxID, Status: "unknown"}, nil
}

func (b *AppleContainerBackend) workspaceRoot() string {
	if b.cfg.AppleContainer.WorkspaceRoot != "" {
		return b.cfg.AppleContainer.WorkspaceRoot
	}
	return "/workspace"
}

func (b *AppleContainerBackend) translateCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	if cwd == b.workspaceRoot() {
		return cwd
	}
	localRoot, err := filepath.Abs(b.cfg.Run.WorkspaceDir)
	if err != nil {
		return cwd
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	rel, err := filepath.Rel(localRoot, absCwd)
	if err != nil || rel == "." {
		return b.workspaceRoot()
	}
	return path.Join(b.workspaceRoot(), filepath.ToSlash(rel))
}

func (b *AppleContainerBackend) labels(req CreateSandboxRequest) []string {
	labels := []string{
		"run_id=" + req.RunID,
		fmt.Sprintf("attempt=%d", req.Attempt),
		"backend.provider=apple-container",
	}
	sort.Strings(labels)
	return labels
}

func (b *AppleContainerBackend) runCLI(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.binary, args...)
	if dir != "" {
		cmd.Dir = dir
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

func (b *AppleContainerBackend) lookupSandbox(sandboxID string) (appleContainerRecord, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	record, ok := b.sandboxes[sandboxID]
	return record, ok
}

func (b *AppleContainerBackend) deleteExec(execID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.execs, execID)
}

func sortedEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func parseAppleContainerInspect(output []byte) (SandboxInfo, bool) {
	var payload any
	if err := json.Unmarshal(output, &payload); err == nil {
		switch typed := payload.(type) {
		case []any:
			if len(typed) == 0 {
				return SandboxInfo{}, false
			}
			if item, ok := typed[0].(map[string]any); ok {
				return appleSandboxInfoFromMap(item), true
			}
		case map[string]any:
			return appleSandboxInfoFromMap(typed), true
		}
	}
	return SandboxInfo{}, false
}

func appleSandboxInfoFromMap(item map[string]any) SandboxInfo {
	info := SandboxInfo{
		ID:       firstString(item, "id", "ID", "name", "Name"),
		Status:   firstString(item, "status", "Status", "state", "State"),
		Metadata: map[string]string{},
	}
	image := firstString(item, "image", "Image")
	if image != "" {
		info.Metadata["image"] = image
	}
	if info.Status == "" {
		if nested, ok := item["state"].(map[string]any); ok {
			info.Status = firstString(nested, "status", "Status")
		}
	}
	return info
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := item[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		}
	}
	return ""
}

func wrapAppleContainerErr(code model.ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return model.RunnerError{
		Code:        string(code),
		Message:     err.Error(),
		BackendKind: string(model.BackendKindAppleContainer),
		Cause:       err,
	}
}

var _ SandboxBackend = (*AppleContainerBackend)(nil)
var _ WorkspaceSyncer = (*AppleContainerBackend)(nil)
var _ MetadataProvider = (*AppleContainerBackend)(nil)
var _ ExecStatusProvider = (*AppleContainerBackend)(nil)
var _ ExecCanceler = (*AppleContainerBackend)(nil)
