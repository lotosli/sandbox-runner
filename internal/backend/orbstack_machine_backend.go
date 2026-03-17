package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type OrbStackMachineBackend struct {
	cfg       model.RunConfig
	orbBinary string
	ctlBinary string
	mu        sync.Mutex
	sandboxes map[string]orbstackMachineRecord
	execs     map[string]*localExecSession
}

type orbstackMachineRecord struct {
	info SandboxInfo
}

func NewOrbStackMachineBackend(cfg model.RunConfig) (*OrbStackMachineBackend, error) {
	if runtime.GOOS != "darwin" {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeRuntimeProfileUnsupported),
			Message:     "orbstack-machine backend is supported only on macOS",
			BackendKind: string(model.BackendKindOrbStackMachine),
		}
	}
	ctlBinary, ctlErr := exec.LookPath(cfg.OrbStack.OrbCtlBinary)
	orbBinary, orbErr := exec.LookPath(cfg.OrbStack.OrbBinary)
	if ctlErr != nil && orbErr != nil {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeOrbStackBinaryNotFound),
			Message:     fmt.Sprintf("orbstack CLI not found: %s / %s", cfg.OrbStack.OrbCtlBinary, cfg.OrbStack.OrbBinary),
			BackendKind: string(model.BackendKindOrbStackMachine),
		}
	}
	if ctlBinary == "" {
		ctlBinary = orbBinary
	}
	if orbBinary == "" {
		orbBinary = ctlBinary
	}
	return &OrbStackMachineBackend{
		cfg:       cfg,
		orbBinary: orbBinary,
		ctlBinary: ctlBinary,
		sandboxes: map[string]orbstackMachineRecord{},
		execs:     map[string]*localExecSession{},
	}, nil
}

func (b *OrbStackMachineBackend) Kind() model.BackendKind { return model.BackendKindOrbStackMachine }

func (b *OrbStackMachineBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return model.BackendCapabilities{
		SupportsFileUpload:     true,
		SupportsFileDownload:   true,
		SupportsStreamLogs:     true,
		SupportsRuntimeProfile: true,
		SupportsMachineExec:    true,
		SupportsVMIsolation:    true,
	}, nil
}

func (b *OrbStackMachineBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	return model.RuntimeInfo{
		ProviderKind:     string(model.BackendKindOrbStackMachine),
		BackendProvider:  "orbstack",
		RuntimeProfile:   string(model.RuntimeProfileOrbStackMachine),
		ContainerRuntime: "orbstack-machine",
		HostOS:           runtime.GOOS,
		HostArch:         runtime.GOARCH,
		Virtualization:   "vm",
		LocalPlatform:    "orbstack",
		MachineName:      b.cfg.OrbStack.MachineName,
		Available:        true,
	}, nil
}

func (b *OrbStackMachineBackend) Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error) {
	machineName := b.cfg.OrbStack.MachineName
	if machineName == "" {
		machineName = fmt.Sprintf("ai-runner-%s-%d", req.RunID, req.Attempt)
	}
	info, err := b.machineInfo(ctx, machineName)
	if err != nil {
		if !b.cfg.OrbStack.MachineAutoCreate {
			return SandboxInfo{}, wrapOrbStackErr(model.ErrorCodeOrbStackMachineNotFound, err)
		}
		args := []string{"create"}
		if b.cfg.OrbStack.MachineDistro != "" {
			args = append(args, b.cfg.OrbStack.MachineDistro)
		} else {
			args = append(args, "ubuntu")
		}
		args = append(args, machineName)
		if _, err := b.runCtl(ctx, args...); err != nil {
			return SandboxInfo{}, wrapOrbStackErr(model.ErrorCodeOrbStackMachineCreateFailed, err)
		}
		info, err = b.machineInfo(ctx, machineName)
		if err != nil {
			return SandboxInfo{}, wrapOrbStackErr(model.ErrorCodeOrbStackMachineCreateFailed, err)
		}
	}
	if err := b.Start(ctx, machineName); err != nil {
		return SandboxInfo{}, err
	}
	info.Metadata["workspace_root"] = b.machineWorkspaceRoot()
	info.Metadata["runtime.profile"] = string(model.RuntimeProfileOrbStackMachine)
	info.Metadata["backend.provider"] = "orbstack"
	info.Metadata["local.platform"] = "orbstack"
	b.mu.Lock()
	b.sandboxes[machineName] = orbstackMachineRecord{info: info}
	b.mu.Unlock()
	return info, nil
}

func (b *OrbStackMachineBackend) Start(ctx context.Context, sandboxID string) error {
	_, err := b.runCtl(ctx, "start", sandboxID)
	if err != nil {
		text := err.Error()
		if strings.Contains(strings.ToLower(text), "already running") || strings.Contains(strings.ToLower(text), "running") {
			return nil
		}
	}
	return wrapOrbStackErr(model.ErrorCodeSandboxStartFailed, err)
}

func (b *OrbStackMachineBackend) Stat(ctx context.Context, sandboxID string) (SandboxStatus, error) {
	info, err := b.machineInfo(ctx, sandboxID)
	if err != nil {
		return SandboxStatus{}, err
	}
	return SandboxStatus{ID: sandboxID, Status: info.Status}, nil
}

func (b *OrbStackMachineBackend) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error) {
	execID := fmt.Sprintf("%s-exec-%d", sandboxID, time.Now().UTC().UnixNano())
	session := newLocalExecSession(execID)
	args := []string{"run", "-m", sandboxID, "-p"}
	if cwd := b.translateCwd(req.Cwd); cwd != "" {
		args = append(args, "-w", cwd)
	}
	args = append(args, "/bin/sh", "-lc", req.Command)
	cmd := exec.CommandContext(ctx, b.ctlBinary, args...)
	cmd.Env = orbstackExecEnv(os.Environ(), req.Env)
	if err := session.start(ctx, cmd); err != nil {
		return ExecHandle{}, wrapOrbStackErr(model.ErrorCodeOrbStackExecFailed, err)
	}
	b.mu.Lock()
	b.execs[execID] = session
	b.mu.Unlock()
	return ExecHandle{ExecID: execID, Status: "running", Provider: "orbstack"}, nil
}

func (b *OrbStackMachineBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error) {
	_ = ctx
	_ = sandboxID
	b.mu.Lock()
	session, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil, model.RunnerError{
			Code:        string(model.ErrorCodeOrbStackExecFailed),
			Message:     fmt.Sprintf("unknown orbstack exec session: %s", execID),
			BackendKind: string(model.BackendKindOrbStackMachine),
		}
	}
	return session.logs, nil
}

func (b *OrbStackMachineBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	if pathUnderRoot(localPath, b.cfg.Run.WorkspaceDir) {
		return nil
	}
	args := []string{"push", "-m", sandboxID, localPath}
	if remotePath != "" {
		args = append(args, remotePath)
	}
	_, err := b.runCtl(ctx, args...)
	return wrapOrbStackErr(model.ErrorCodeOrbStackCopyFailed, err)
}

func (b *OrbStackMachineBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	if pathUnderRoot(localPath, b.cfg.Run.WorkspaceDir) {
		return nil
	}
	args := []string{"pull", "-m", sandboxID, remotePath}
	if localPath != "" {
		args = append(args, localPath)
	}
	_, err := b.runCtl(ctx, args...)
	return wrapOrbStackErr(model.ErrorCodeOrbStackCopyFailed, err)
}

func (b *OrbStackMachineBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_ = ctx
	_ = sandboxID
	_ = ttl
	return errUnsupported
}

func (b *OrbStackMachineBackend) Pause(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *OrbStackMachineBackend) Resume(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *OrbStackMachineBackend) Delete(ctx context.Context, sandboxID string) error {
	switch strings.ToLower(b.cfg.OrbStack.MachineCleanupMode) {
	case "stop":
		_, err := b.runCtl(ctx, "stop", sandboxID)
		return wrapOrbStackErr(model.ErrorCodeOrbStackStopFailed, err)
	case "delete":
		_, err := b.runCtl(ctx, "delete", "-f", sandboxID)
		return wrapOrbStackErr(model.ErrorCodeOrbStackDeleteFailed, err)
	default:
		return nil
	}
}

func (b *OrbStackMachineBackend) SyncWorkspaceIn(ctx context.Context, sandboxID, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = localDir
	return nil
}

func (b *OrbStackMachineBackend) SyncWorkspaceOut(ctx context.Context, sandboxID, remoteDir, localDir string) error {
	_ = ctx
	_ = sandboxID
	_ = remoteDir
	_ = localDir
	return nil
}

func (b *OrbStackMachineBackend) SandboxMetadata(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return b.machineInfo(ctx, sandboxID)
}

func (b *OrbStackMachineBackend) Endpoints(ctx context.Context, sandboxID string, ports []int) ([]model.Endpoint, error) {
	_ = ctx
	_ = sandboxID
	_ = ports
	return nil, nil
}

func (b *OrbStackMachineBackend) ExecStatus(ctx context.Context, sandboxID string, execID string) (ExecStatus, error) {
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

func (b *OrbStackMachineBackend) CancelExec(ctx context.Context, sandboxID string, execID string) error {
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

func (b *OrbStackMachineBackend) machineInfo(ctx context.Context, machineName string) (SandboxInfo, error) {
	output, err := b.runCtl(ctx, "info", "-f", "json", machineName)
	if err == nil {
		if info, ok := parseOrbStackMachineInfo(output); ok {
			if info.ID == "" {
				info.ID = machineName
			}
			return info, nil
		}
	}
	listOutput, listErr := b.runCtl(ctx, "list", "-f", "json")
	if listErr != nil {
		if err != nil {
			return SandboxInfo{}, wrapOrbStackErr(model.ErrorCodeOrbStackMachineNotFound, err)
		}
		return SandboxInfo{}, wrapOrbStackErr(model.ErrorCodeOrbStackMachineNotFound, listErr)
	}
	if info, ok := parseOrbStackMachineList(listOutput, machineName); ok {
		return info, nil
	}
	if record, ok := b.lookupSandbox(machineName); ok {
		return record.info, nil
	}
	return SandboxInfo{}, model.RunnerError{
		Code:        string(model.ErrorCodeOrbStackMachineNotFound),
		Message:     fmt.Sprintf("orbstack machine not found: %s", machineName),
		BackendKind: string(model.BackendKindOrbStackMachine),
	}
}

func (b *OrbStackMachineBackend) translateCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		return abs
	}
	return cwd
}

func (b *OrbStackMachineBackend) machineWorkspaceRoot() string {
	if b.cfg.OrbStack.MachineWorkspaceRoot != "" {
		return b.cfg.OrbStack.MachineWorkspaceRoot
	}
	return b.cfg.Run.WorkspaceDir
}

func (b *OrbStackMachineBackend) runCtl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.ctlBinary, args...)
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

func (b *OrbStackMachineBackend) lookupSandbox(id string) (orbstackMachineRecord, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	record, ok := b.sandboxes[id]
	return record, ok
}

func (b *OrbStackMachineBackend) deleteExec(execID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.execs, execID)
}

func parseOrbStackMachineInfo(output []byte) (SandboxInfo, bool) {
	payload := map[string]any{}
	if err := json.Unmarshal(output, &payload); err != nil {
		return SandboxInfo{}, false
	}
	return orbStackSandboxInfoFromMap(payload), true
}

func parseOrbStackMachineList(output []byte, machineName string) (SandboxInfo, bool) {
	var list []map[string]any
	if err := json.Unmarshal(output, &list); err != nil {
		return SandboxInfo{}, false
	}
	for _, item := range list {
		if firstString(item, "name", "Name", "id", "ID") == machineName {
			return orbStackSandboxInfoFromMap(item), true
		}
	}
	return SandboxInfo{}, false
}

func orbStackSandboxInfoFromMap(item map[string]any) SandboxInfo {
	info := SandboxInfo{
		ID:          firstString(item, "name", "Name", "id", "ID"),
		Status:      firstString(item, "status", "Status", "state", "State"),
		RuntimeKind: "orbstack-machine",
		Metadata: map[string]string{
			"machine_name":      firstString(item, "name", "Name"),
			"distro":            firstString(item, "distro", "Distro", "distribution", "Distribution"),
			"backend.provider":  "orbstack",
			"local.platform":    "orbstack",
			"runtime.profile":   string(model.RuntimeProfileOrbStackMachine),
			"container_runtime": "orbstack-machine",
		},
	}
	return info
}

func pathUnderRoot(pathValue, root string) bool {
	if pathValue == "" || root == "" {
		return false
	}
	absPath, err := filepath.Abs(pathValue)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func orbstackExecEnv(base []string, extra map[string]string) []string {
	env := append([]string{}, base...)
	if len(extra) == 0 {
		return env
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	visible := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+extra[key])
		visible = append(visible, key)
	}
	env = append(env, "ORBENV="+strings.Join(visible, ":"))
	return env
}

func wrapOrbStackErr(code model.ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return model.RunnerError{
		Code:        string(code),
		Message:     err.Error(),
		BackendKind: string(model.BackendKindOrbStackMachine),
		Cause:       err,
	}
}

var _ SandboxBackend = (*OrbStackMachineBackend)(nil)
var _ WorkspaceSyncer = (*OrbStackMachineBackend)(nil)
var _ MetadataProvider = (*OrbStackMachineBackend)(nil)
var _ ExecStatusProvider = (*OrbStackMachineBackend)(nil)
var _ ExecCanceler = (*OrbStackMachineBackend)(nil)
