package executor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type fakeBackend struct {
	handle    backend.ExecHandle
	logs      []backend.LogChunk
	status    backend.ExecStatus
	cancelled bool
}

func (f *fakeBackend) Kind() model.BackendKind { return model.BackendKindOpenSandbox }

func (f *fakeBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return model.BackendCapabilities{SupportsStreamLogs: true}, nil
}

func (f *fakeBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	return model.RuntimeInfo{
		ProviderKind:     string(model.BackendKindOpenSandbox),
		RuntimeProfile:   string(model.RuntimeProfileNative),
		ContainerRuntime: "opensandbox-docker",
		HostOS:           "linux",
		HostArch:         "arm64",
		Virtualization:   "none",
		Available:        true,
	}, nil
}

func (f *fakeBackend) Create(ctx context.Context, req backend.CreateSandboxRequest) (backend.SandboxInfo, error) {
	_ = ctx
	_ = req
	return backend.SandboxInfo{}, nil
}

func (f *fakeBackend) Start(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (f *fakeBackend) Stat(ctx context.Context, sandboxID string) (backend.SandboxStatus, error) {
	_ = ctx
	return backend.SandboxStatus{ID: sandboxID}, nil
}

func (f *fakeBackend) Exec(ctx context.Context, sandboxID string, req backend.ExecRequest) (backend.ExecHandle, error) {
	_ = ctx
	_ = sandboxID
	_ = req
	return f.handle, nil
}

func (f *fakeBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan backend.LogChunk, error) {
	_ = ctx
	_ = sandboxID
	_ = execID
	ch := make(chan backend.LogChunk, len(f.logs))
	for _, item := range f.logs {
		ch <- item
	}
	close(ch)
	return ch, nil
}

func (f *fakeBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	_ = ctx
	_ = sandboxID
	_ = localPath
	_ = remotePath
	return nil
}

func (f *fakeBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	_ = ctx
	_ = sandboxID
	_ = remotePath
	_ = localPath
	return nil
}

func (f *fakeBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_ = ctx
	_ = sandboxID
	_ = ttl
	return nil
}

func (f *fakeBackend) Pause(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (f *fakeBackend) Resume(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (f *fakeBackend) Delete(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (f *fakeBackend) ExecStatus(ctx context.Context, sandboxID string, execID string) (backend.ExecStatus, error) {
	_ = ctx
	_ = sandboxID
	_ = execID
	return f.status, nil
}

func (f *fakeBackend) CancelExec(ctx context.Context, sandboxID string, execID string) error {
	_ = ctx
	_ = sandboxID
	_ = execID
	f.cancelled = true
	return nil
}

func TestBackendExecutorRun(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeLocalOpenSandboxDocker
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeDocker
	cfg.OpenSandbox.WorkspaceRoot = "/workspace"
	cfg.Run.SandboxID = "sbx-1"
	cfg.Run.WorkspaceDir = t.TempDir()

	exec, err := NewBackendExecutor(cfg, model.ExecutionTarget{Arch: "arm64"}, &fakeBackend{
		handle: backend.ExecHandle{ExecID: "cmd-1", Provider: "opensandbox"},
		logs: []backend.LogChunk{
			{Timestamp: time.Now().UTC(), Stream: "stdout", Line: "hello from stdout"},
			{Timestamp: time.Now().UTC(), Stream: "stderr", Line: "hello from stderr"},
		},
		status: backend.ExecStatus{ID: "cmd-1", Running: false, ExitCode: intPtr(0)},
	})
	if err != nil {
		t.Fatalf("NewBackendExecutor() error = %v", err)
	}

	handler := &captureLogs{}
	result, err := exec.Run(context.Background(), Spec{
		Phase:           model.PhaseExecute,
		Command:         []string{"echo", "hello"},
		Env:             map[string]string{"FOO": "bar"},
		Dir:             cfg.Run.WorkspaceDir,
		Timeout:         2 * time.Second,
		RunID:           "r-1",
		Attempt:         1,
		CommandClass:    "test.run",
		ArtifactDir:     t.TempDir(),
		LogLineMaxBytes: 8192,
		RunConfig:       cfg,
		Target:          model.ExecutionTarget{Arch: "arm64"},
	}, handler)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.StdoutLines != 1 || result.StderrLines != 1 {
		t.Fatalf("stdout/stderr lines = %d/%d, want 1/1", result.StdoutLines, result.StderrLines)
	}
	if got := result.Metadata["exec_provider_id"]; got != "cmd-1" {
		t.Fatalf("exec_provider_id = %v, want cmd-1", got)
	}
	if len(handler.logs) != 2 {
		t.Fatalf("captured logs = %d, want 2", len(handler.logs))
	}
	if handler.logs[0].Provider != "opensandbox" {
		t.Fatalf("provider = %s, want opensandbox", handler.logs[0].Provider)
	}
}

func TestBackendExecutorRemoteCwdAppleContainer(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindAppleContainer
	cfg.Platform.RunMode = model.RunModeLocalAppleContainer
	cfg.Runtime.Profile = model.RuntimeProfileAppleContainer
	cfg.Run.WorkspaceDir = t.TempDir()
	cfg.AppleContainer.WorkspaceRoot = "/workspace"

	exec, err := NewBackendExecutor(cfg, model.ExecutionTarget{Arch: "arm64"}, &fakeBackend{})
	if err != nil {
		t.Fatalf("NewBackendExecutor() error = %v", err)
	}

	subdir := filepath.Join(cfg.Run.WorkspaceDir, "sub", "pkg")
	got := exec.remoteCwd(subdir)
	if got != "/workspace/sub/pkg" {
		t.Fatalf("remoteCwd() = %q, want /workspace/sub/pkg", got)
	}
}

func TestBackendExecutorResultTargetOrbStackMachine(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOrbStackMachine
	cfg.Platform.RunMode = model.RunModeLocalOrbStackMachine
	cfg.Runtime.Profile = model.RuntimeProfileOrbStackMachine
	cfg.OrbStack.MachineName = "ai-runner-dev"

	exec, err := NewBackendExecutor(cfg, model.ExecutionTarget{Arch: "arm64"}, &fakeBackend{})
	if err != nil {
		t.Fatalf("NewBackendExecutor() error = %v", err)
	}

	target := exec.resultTarget("machine-sandbox")
	if target.BackendProvider != "orbstack" {
		t.Fatalf("BackendProvider = %q, want orbstack", target.BackendProvider)
	}
	if target.LocalPlatform != "orbstack" {
		t.Fatalf("LocalPlatform = %q, want orbstack", target.LocalPlatform)
	}
	if target.MachineName != "ai-runner-dev" {
		t.Fatalf("MachineName = %q, want ai-runner-dev", target.MachineName)
	}
	if target.RuntimeKind != "orbstack-machine" {
		t.Fatalf("RuntimeKind = %q, want orbstack-machine", target.RuntimeKind)
	}
}

func intPtr(value int) *int { return &value }
