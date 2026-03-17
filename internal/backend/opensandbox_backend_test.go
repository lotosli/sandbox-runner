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

func serverURLWithoutScheme(r *http.Request) string {
	return r.Host + "/sandboxes/sbx-1/proxy/44772"
}
