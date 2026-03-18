package opensandbox_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backendpkg "github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestOpenSandboxServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping OpenSandbox integration test in short mode")
	}
	if _, err := exec.LookPath("opensandbox-server"); err != nil {
		t.Skipf("opensandbox-server not installed: %v", err)
	}
	if err := dockerReady(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sandbox.toml")
	if output, err := exec.Command("opensandbox-server", "init-config", configPath, "--example", "docker", "--force").CombinedOutput(); err != nil {
		t.Fatalf("init-config failed: %v\n%s", err, output)
	}

	port, err := freeTCPPort()
	if err != nil {
		t.Fatalf("freeTCPPort() error = %v", err)
	}
	if err := rewriteServerPort(configPath, port); err != nil {
		t.Fatalf("rewriteServerPort() error = %v", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command("opensandbox-server", "--config", configPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start opensandbox-server: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	if err := waitForServerReady(baseURL, 20*time.Second); err != nil {
		t.Fatalf("opensandbox-server did not become ready: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "README.txt"), []byte("hello opensandbox"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeLocalOpenSandboxDocker
	cfg.OpenSandbox.BaseURL = baseURL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeDocker
	cfg.OpenSandbox.CleanupMode = model.OpenSandboxCleanupDelete
	cfg.OpenSandbox.WorkspaceRoot = "/workspace"
	cfg.Run.WorkspaceDir = workspaceDir
	cfg.Sandbox.Image = "debian:bookworm-slim"

	backend := backendpkg.NewOpenSandboxBackend(cfg)
	info, err := backend.Create(context.Background(), backendpkg.CreateSandboxRequest{
		RunID:        "r-integration",
		Attempt:      1,
		Image:        cfg.Sandbox.Image,
		Entrypoint:   []string{"/bin/sh", "-lc"},
		WorkspaceDir: cfg.OpenSandbox.WorkspaceRoot,
		TimeoutSec:   180,
	})
	if err != nil {
		t.Fatalf("Create() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if err := backend.Start(context.Background(), info.ID); err != nil {
		t.Fatalf("Start() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if err := backend.SyncWorkspaceIn(context.Background(), info.ID, workspaceDir); err != nil {
		t.Fatalf("SyncWorkspaceIn() error = %v", err)
	}

	exitCode, stderrText, err := backend.RunSimpleCommand(context.Background(), info.ID, "pwd", cfg.OpenSandbox.WorkspaceRoot, nil, 30*time.Second)
	if err != nil {
		if isOpenSandboxProxyLimitation(err) {
			t.Skipf("opensandbox local docker fixture cannot proxy exec traffic from this host: %v", err)
		}
		t.Fatalf("RunSimpleCommand(pwd) error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("pwd exit code = %d, stderr = %s", exitCode, stderrText)
	}

	exitCode, stderrText, err = backend.RunSimpleCommand(context.Background(), info.ID, "mkdir -p /workspace/.sandbox-runner && echo hello > /workspace/.sandbox-runner/hello.txt", cfg.OpenSandbox.WorkspaceRoot, nil, 30*time.Second)
	if err != nil {
		if isOpenSandboxProxyLimitation(err) {
			t.Skipf("opensandbox local docker fixture cannot proxy exec traffic from this host: %v", err)
		}
		t.Fatalf("RunSimpleCommand(write artifact) error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("artifact write exit code = %d, stderr = %s", exitCode, stderrText)
	}

	outDir := filepath.Join(tmpDir, "artifacts")
	if err := backend.SyncWorkspaceOut(context.Background(), info.ID, "/workspace/.sandbox-runner", outDir); err != nil {
		t.Fatalf("SyncWorkspaceOut() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("artifact content = %q, want %q", strings.TrimSpace(string(data)), "hello")
	}

	if err := backend.Delete(context.Background(), info.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func dockerReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run()
}

func freeTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func rewriteServerPort(path string, port int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.Replace(string(data), "port = 8080", fmt.Sprintf("port = %d", port), 1)
	return os.WriteFile(path, []byte(updated), 0o644)
}

func waitForServerReady(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/v1/sandboxes?page=1&pageSize=1")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", baseURL)
}

func isOpenSandboxProxyLimitation(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "Could not connect to the backend sandbox endpoint") ||
		strings.Contains(message, "NETWORK_MODE_ENDPOINT_UNAVAILABLE") ||
		strings.Contains(message, "has no assigned IP address")
}
