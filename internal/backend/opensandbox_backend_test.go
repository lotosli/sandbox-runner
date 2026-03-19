package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestOpenSandboxBackendExecAndStreamLogs(t *testing.T) {
	var statusCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sandboxes/sbx-1/endpoints/44772":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"endpoint": serverURLWithoutScheme(r),
				"headers":  map[string]string{"X-EXECD-ACCESS-TOKEN": "token-1"},
			})
		case "/sandboxes/sbx-1/proxy/44772/command":
			w.Header().Set("Content-Type", "text/event-stream")
			events := []map[string]any{
				{"type": "init", "text": "cmd-1", "timestamp": time.Now().UnixMilli()},
				{"type": "stdout", "text": "hello from stdout\n", "timestamp": time.Now().UnixMilli()},
				{"type": "stderr", "text": "hello from stderr\n", "timestamp": time.Now().UnixMilli()},
				{"type": "execution_complete", "execution_time": int64(25), "timestamp": time.Now().UnixMilli()},
			}
			for _, event := range events {
				data, _ := json.Marshal(event)
				fmt.Fprintln(w, string(data))
			}
		case "/sandboxes/sbx-1/proxy/44772/command/status/cmd-1":
			statusCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "cmd-1",
				"running":    false,
				"exit_code":  0,
				"started_at": time.Now().UTC(),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeLocalOpenSandboxDocker
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeDocker
	cfg.OpenSandbox.NetworkMode = "bridge"

	backend := NewOpenSandboxBackend(cfg)
	handle, err := backend.Exec(context.Background(), "sbx-1", ExecRequest{
		Command: "echo hello",
		Cwd:     "/workspace",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if handle.ExecID != "cmd-1" {
		t.Fatalf("exec id = %s, want cmd-1", handle.ExecID)
	}

	logs, err := backend.StreamLogs(context.Background(), "sbx-1", handle.ExecID)
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
	if chunks[0].Stream != "stdout" || chunks[0].Line != "hello from stdout" {
		t.Fatalf("first chunk = %#v", chunks[0])
	}
	if chunks[1].Stream != "stderr" || chunks[1].Line != "hello from stderr" {
		t.Fatalf("second chunk = %#v", chunks[1])
	}

	status, err := backend.ExecStatus(context.Background(), "sbx-1", handle.ExecID)
	if err != nil {
		t.Fatalf("ExecStatus() error = %v", err)
	}
	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", status.ExitCode)
	}
	if statusCalls == 0 {
		t.Fatalf("expected command status lookup")
	}
}

func TestOpenSandboxBackendMapsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"UNAUTHORIZED","message":"bad api key","status_code":401,"retryable":false}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.OpenSandbox.BaseURL = server.URL

	backend := NewOpenSandboxBackend(cfg)
	_, err := backend.Create(context.Background(), CreateSandboxRequest{Image: "alpine:3.20"})
	if err == nil {
		t.Fatal("Create() error = nil, want mapped RunnerError")
	}

	runnerErr, ok := err.(model.RunnerError)
	if !ok {
		t.Fatalf("error type = %T, want model.RunnerError", err)
	}
	if runnerErr.ProviderCode != "UNAUTHORIZED" {
		t.Fatalf("provider code = %s, want UNAUTHORIZED", runnerErr.ProviderCode)
	}
	if runnerErr.Code != string(model.ErrorCodeSandboxCreateFailed) {
		t.Fatalf("runner code = %s, want %s", runnerErr.Code, model.ErrorCodeSandboxCreateFailed)
	}
}

func TestOpenSandboxBackendCreatePropagatesKataRuntimeMetadata(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sandboxes" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "sbx-kata",
			"status": map[string]any{
				"state": "running",
			},
			"metadata": map[string]string{
				"runtime.profile": "kata",
				"runtime.class":   "kata",
			},
		})
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeSTGOpenSandboxK8s
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeKubernetes
	cfg.Runtime.Profile = model.RuntimeProfileKata
	cfg.Kata.Enabled = true
	cfg.Kata.RuntimeClassName = "kata"

	backend := NewOpenSandboxBackend(cfg)
	_, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:    "run-kata",
		Attempt:  1,
		Image:    "alpine:3.20",
		Metadata: map[string]string{"run_id": "run-kata"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want map", payload["metadata"])
	}
	if metadata["runtime.profile"] != "kata" {
		t.Fatalf("runtime.profile = %v, want kata", metadata["runtime.profile"])
	}
	if metadata["runtime.class"] != "kata" {
		t.Fatalf("runtime.class = %v, want kata", metadata["runtime.class"])
	}

	extensions, ok := payload["extensions"].(map[string]any)
	if !ok {
		t.Fatalf("extensions = %#v, want map", payload["extensions"])
	}
	if extensions["runtime.profile"] != "kata" {
		t.Fatalf("extensions runtime.profile = %v, want kata", extensions["runtime.profile"])
	}
	if extensions["runtime.class"] != "kata" {
		t.Fatalf("extensions runtime.class = %v, want kata", extensions["runtime.class"])
	}
}

func TestOpenSandboxBackendCreatePropagatesFirecrackerRuntimeMetadata(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sandboxes" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "sbx-firecracker",
			"status": map[string]any{
				"state": "running",
			},
			"metadata": map[string]string{
				"runtime.profile": "firecracker",
				"runtime.class":   "sandbox-runner-microvm",
			},
		})
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeSTGOpenSandboxK8s
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeKubernetes
	cfg.Runtime.Profile = model.RuntimeProfileFirecracker
	cfg.Runtime.ClassName = "sandbox-runner-microvm"

	backend := NewOpenSandboxBackend(cfg)
	_, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:    "run-firecracker",
		Attempt:  1,
		Image:    "alpine:3.20",
		Metadata: map[string]string{"run_id": "run-firecracker"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want map", payload["metadata"])
	}
	if metadata["runtime.profile"] != "firecracker" {
		t.Fatalf("runtime.profile = %v, want firecracker", metadata["runtime.profile"])
	}
	if metadata["runtime.class"] != "sandbox-runner-microvm" {
		t.Fatalf("runtime.class = %v, want sandbox-runner-microvm", metadata["runtime.class"])
	}

	extensions, ok := payload["extensions"].(map[string]any)
	if !ok {
		t.Fatalf("extensions = %#v, want map", payload["extensions"])
	}
	if extensions["runtime.profile"] != "firecracker" {
		t.Fatalf("extensions runtime.profile = %v, want firecracker", extensions["runtime.profile"])
	}
	if extensions["runtime.class"] != "sandbox-runner-microvm" {
		t.Fatalf("extensions runtime.class = %v, want sandbox-runner-microvm", extensions["runtime.class"])
	}
}

func TestOpenSandboxBackendCreateExpandsDefaultShellEntrypointToKeepAlive(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sandboxes" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "sbx-shell",
			"status": map[string]any{
				"state": "running",
			},
		})
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Backend.Kind = model.BackendKindOpenSandbox
	cfg.Platform.RunMode = model.RunModeLocalOpenSandboxDocker
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeDocker

	backend := NewOpenSandboxBackend(cfg)
	_, err := backend.Create(context.Background(), CreateSandboxRequest{
		RunID:      "run-shell",
		Attempt:    1,
		Image:      "alpine:3.20",
		Entrypoint: []string{"/bin/sh", "-lc"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entrypoint, ok := payload["entrypoint"].([]any)
	if !ok {
		t.Fatalf("entrypoint = %#v, want array", payload["entrypoint"])
	}
	if len(entrypoint) != 3 {
		t.Fatalf("entrypoint len = %d, want 3", len(entrypoint))
	}
	if entrypoint[0] != "/bin/sh" || entrypoint[1] != "-lc" || entrypoint[2] != openSandboxKeepAliveCommand {
		t.Fatalf("entrypoint = %#v, want expanded keepalive shell", entrypoint)
	}
}

func serverURLWithoutScheme(r *http.Request) string {
	return r.Host + "/sandboxes/sbx-1/proxy/44772"
}
