package backend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestDevContainerBackendCreateExecAndDelete(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	logPath := filepath.Join(dir, "devcontainer.log")
	scriptPath := filepath.Join(dir, "devcontainer")
	script := `#!/bin/sh
set -eu
cmd="$1"
shift
log_path="${DEVCONTAINER_TEST_LOG}"
printf '%s %s\n' "$cmd" "$*" >> "$log_path"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --workspace-folder)
      workspace="$2"
      shift 2
      ;;
    --config)
      config="$2"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    *)
      shift
      ;;
  esac
done
case "$cmd" in
  read-configuration)
    printf '{"workspaceFolder":"/workspaces/test","postCreateCommand":"go mod download","features":{"ghcr.io/devcontainers/features/go:1":{}}}\n'
    ;;
  up)
    printf '{"outcome":"ok"}\n'
    ;;
  run-user-commands)
    printf '{"outcome":"ok"}\n'
    ;;
  exec)
    exec "$@"
    ;;
  down)
    printf '{"outcome":"ok"}\n'
    ;;
  *)
    echo "unexpected command: $cmd" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("DEVCONTAINER_TEST_LOG", logPath)

	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.Runtime.Profile = model.RuntimeProfileNative
	cfg.DevContainer.CLIPath = scriptPath
	cfg.DevContainer.WorkspaceFolder = workspace
	cfg.Run.WorkspaceDir = workspace
	cfg.Run.RunID = "run-devcontainer"
	cfg.Run.Attempt = 2
	cfg.Run.SandboxID = ""

	backend, err := NewDevContainerBackend(cfg)
	if err != nil {
		t.Fatalf("NewDevContainerBackend() error = %v", err)
	}

	info, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		WorkspaceDir: workspace,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if info.ID != "sbx-devcontainer-run-devcontainer-2" {
		t.Fatalf("sandbox id = %s, want sbx-devcontainer-run-devcontainer-2", info.ID)
	}

	summary, err := backend.DevContainerMetadata(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("DevContainerMetadata() error = %v", err)
	}
	if summary.WorkspaceFolder != "/workspaces/test" {
		t.Fatalf("workspace folder = %s, want /workspaces/test", summary.WorkspaceFolder)
	}
	if !summary.HasPostCreate {
		t.Fatal("expected postCreateCommand to be detected")
	}
	if len(summary.Features) != 1 || summary.Features[0] != "ghcr.io/devcontainers/features/go:1" {
		t.Fatalf("features = %v, want devcontainer feature key", summary.Features)
	}

	handle, err := backend.Exec(context.Background(), info.ID, ExecRequest{
		Command: `printf 'hello from stdout\n'; printf 'hello from stderr\n' 1>&2`,
		Env:     map[string]string{"FOO": "bar"},
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	logs, err := backend.StreamLogs(context.Background(), info.ID, handle.ExecID)
	if err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}

	var chunks []LogChunk
	for chunk := range logs {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("log chunks = %d, want 2", len(chunks))
	}
	streams := map[string]string{}
	for _, chunk := range chunks {
		streams[chunk.Stream] = chunk.Line
	}
	if streams["stdout"] != "hello from stdout" {
		t.Fatalf("stdout line = %q, want %q", streams["stdout"], "hello from stdout")
	}
	if streams["stderr"] != "hello from stderr" {
		t.Fatalf("stderr line = %q, want %q", streams["stderr"], "hello from stderr")
	}

	status, err := backend.ExecStatus(context.Background(), info.ID, handle.ExecID)
	if err != nil {
		t.Fatalf("ExecStatus() error = %v", err)
	}
	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", status.ExitCode)
	}

	if err := backend.Delete(context.Background(), info.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, fragment := range []string{"read-configuration", "up", "run-user-commands", "exec", "down"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("log %q does not contain %q", text, fragment)
		}
	}
}

func TestDevContainerBackendKeepModeSkipsDown(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "devcontainer.log")
	scriptPath := filepath.Join(dir, "devcontainer")
	script := `#!/bin/sh
set -eu
printf '%s %s\n' "$1" "$*" >> "${DEVCONTAINER_TEST_LOG}"
case "$1" in
  read-configuration)
    printf '{"workspaceFolder":"/workspaces/test"}\n'
    ;;
  up)
    printf '{}\n'
    ;;
  run-user-commands)
    printf '{}\n'
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("DEVCONTAINER_TEST_LOG", logPath)

	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.DevContainer.CLIPath = scriptPath
	cfg.DevContainer.WorkspaceFolder = dir
	cfg.DevContainer.CleanupMode = "keep"
	cfg.Run.WorkspaceDir = dir
	cfg.Run.RunID = "run-keep"
	cfg.Run.Attempt = 1
	cfg.Run.SandboxID = ""

	backend, err := NewDevContainerBackend(cfg)
	if err != nil {
		t.Fatalf("NewDevContainerBackend() error = %v", err)
	}

	info, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		WorkspaceDir: dir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := backend.Delete(context.Background(), info.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "down") {
		t.Fatalf("expected keep cleanup mode to skip down, log=%q", string(data))
	}
}

func TestDevContainerBackendDeleteFallsBackToDockerRemoveWhenDownUnsupported(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "devcontainer.log")
	dockerLogPath := filepath.Join(dir, "docker.log")
	devcontainerPath := filepath.Join(dir, "devcontainer")
	dockerPath := filepath.Join(dir, "docker")
	devcontainerScript := `#!/bin/sh
set -eu
printf '%s %s\n' "$1" "$*" >> "${DEVCONTAINER_TEST_LOG}"
case "$1" in
  read-configuration)
    printf '{"workspaceFolder":"/workspaces/test"}\n'
    ;;
  up)
    printf '{"containerId":"ctr-devcontainer-1"}\n'
    ;;
  down)
    echo 'Unknown arguments: workspace-folder, config, log-level, mount-workspace-git-root, down' >&2
    exit 1
    ;;
  *)
    printf '{}\n'
    ;;
esac
`
	dockerScript := `#!/bin/sh
set -eu
printf '%s %s\n' "$1" "$*" >> "${DOCKER_TEST_LOG}"
`
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerScript), 0o755); err != nil {
		t.Fatalf("WriteFile(devcontainer) error = %v", err)
	}
	if err := os.WriteFile(dockerPath, []byte(dockerScript), 0o755); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}
	t.Setenv("DEVCONTAINER_TEST_LOG", logPath)
	t.Setenv("DOCKER_TEST_LOG", dockerLogPath)

	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.DevContainer.CLIPath = devcontainerPath
	cfg.DevContainer.WorkspaceFolder = dir
	cfg.Run.WorkspaceDir = dir
	cfg.Run.RunID = "run-fallback"
	cfg.Run.Attempt = 1
	cfg.Run.SandboxID = ""
	cfg.Docker.Binary = dockerPath

	backend, err := NewDevContainerBackend(cfg)
	if err != nil {
		t.Fatalf("NewDevContainerBackend() error = %v", err)
	}
	info, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		WorkspaceDir: dir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := backend.Delete(context.Background(), info.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	dockerLog, err := os.ReadFile(dockerLogPath)
	if err != nil {
		t.Fatalf("ReadFile(docker log) error = %v", err)
	}
	if !strings.Contains(string(dockerLog), "rm -f ctr-devcontainer-1") {
		t.Fatalf("docker cleanup log = %q, want rm -f ctr-devcontainer-1", string(dockerLog))
	}
}

func TestDevContainerBackendExecTranslatesCwdToResolvedWorkspaceFolder(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	subdir := filepath.Join(workspace, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	scriptPath := filepath.Join(dir, "devcontainer")
	script := `#!/bin/sh
set -eu
cmd="$1"
shift
while [ "$#" -gt 0 ]; do
  case "$1" in
    --workspace-folder)
      workspace="$2"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    *)
      shift
      ;;
  esac
done
case "$cmd" in
  read-configuration)
    printf '{"workspaceFolder":"` + workspace + `"}\n'
    ;;
  up|run-user-commands)
    printf '{}\n'
    ;;
  exec)
    exec "$@"
    ;;
  *)
    printf '{}\n'
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.DevContainer.CLIPath = scriptPath
	cfg.DevContainer.WorkspaceFolder = workspace
	cfg.Run.WorkspaceDir = workspace
	cfg.Run.RunID = "run-cwd"
	cfg.Run.Attempt = 1
	cfg.Run.SandboxID = ""

	backend, err := NewDevContainerBackend(cfg)
	if err != nil {
		t.Fatalf("NewDevContainerBackend() error = %v", err)
	}
	info, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		WorkspaceDir: workspace,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	handle, err := backend.Exec(context.Background(), info.ID, ExecRequest{
		Command: "pwd",
		Cwd:     "/workspace/nested",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	logs, err := backend.StreamLogs(context.Background(), info.ID, handle.ExecID)
	if err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}

	var stdout []string
	for chunk := range logs {
		if chunk.Stream == "stdout" {
			stdout = append(stdout, chunk.Line)
		}
	}
	if len(stdout) == 0 {
		t.Fatal("expected stdout from pwd")
	}
	if stdout[len(stdout)-1] != subdir {
		t.Fatalf("pwd = %q, want %q", stdout[len(stdout)-1], subdir)
	}
}

func TestSummarizeDevContainerConfigParsesJSONAfterBanner(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.DevContainer.CLIPath = "devcontainer"
	cfg.DevContainer.ConfigPath = "/workspace/.devcontainer/devcontainer.json"

	output := []byte("[2026-03-18T17:57:17.171Z] @devcontainers/cli 0.84.1\n" +
		`{"configuration":{"postCreateCommand":"go mod download","features":{"ghcr.io/devcontainers/features/go:1":{}}},"workspace":{"workspaceFolder":"/workspace"}}`)

	summary := summarizeDevContainerConfig(output, cfg, "/tmp/workspace")
	if summary.WorkspaceFolder != "/workspace" {
		t.Fatalf("workspace folder = %q, want /workspace", summary.WorkspaceFolder)
	}
	if !summary.HasPostCreate {
		t.Fatal("expected postCreateCommand to be detected from trailing JSON line")
	}
	if len(summary.Features) != 1 || summary.Features[0] != "ghcr.io/devcontainers/features/go:1" {
		t.Fatalf("features = %v, want devcontainer feature key", summary.Features)
	}
}

func TestMergeDevContainerUpSummaryParsesJSONAfterBanner(t *testing.T) {
	summary := mergeDevContainerUpSummary(model.DevContainerArtifact{}, []byte("[2026-03-18T17:56:46.743Z] @devcontainers/cli 0.84.1\n"+`{"outcome":"success","containerId":"ctr-123"}`))
	if summary.ContainerID != "ctr-123" {
		t.Fatalf("container id = %q, want ctr-123", summary.ContainerID)
	}
}
