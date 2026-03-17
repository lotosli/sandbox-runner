package backend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	backendpkg "github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestOpenSandboxBackendContract(t *testing.T) {
	files := map[string][]byte{}
	deleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "sbx-1",
				"status":   map[string]any{"state": "pending"},
				"metadata": map[string]string{"run_id": "r-1"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/sbx-1/endpoints/44772":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"endpoint": contractServerURLWithoutScheme(r),
				"headers":  map[string]string{"X-EXECD-ACCESS-TOKEN": "token-1"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-1/proxy/44772/command":
			w.Header().Set("Content-Type", "text/event-stream")
			events := []map[string]any{
				{"type": "init", "text": "cmd-1", "timestamp": time.Now().UnixMilli()},
				{"type": "stdout", "text": "hello contract\n", "timestamp": time.Now().UnixMilli()},
				{"type": "execution_complete", "execution_time": int64(10), "timestamp": time.Now().UnixMilli()},
			}
			for _, event := range events {
				data, _ := json.Marshal(event)
				fmt.Fprintln(w, string(data))
			}
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sbx-1/proxy/44772/command/status/cmd-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":        "cmd-1",
				"running":   false,
				"exit_code": 0,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-1/proxy/44772/files/upload":
			remotePath, content := readUploadedFile(t, r)
			files[remotePath] = content
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sbx-1/proxy/44772/files/download":
			remotePath := r.URL.Query().Get("path")
			payload, ok := files[remotePath]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(payload)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx-1":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
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

	backend := backendpkg.NewOpenSandboxBackend(cfg)
	info, err := backend.Create(context.Background(), backendpkg.CreateSandboxRequest{
		RunID:   "r-1",
		Attempt: 1,
		Image:   "alpine:3.20",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if info.ID != "sbx-1" {
		t.Fatalf("sandbox id = %s, want sbx-1", info.ID)
	}

	handle, err := backend.Exec(context.Background(), info.ID, backendpkg.ExecRequest{
		Command: "echo hello",
		Cwd:     "/workspace",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	logs, err := backend.StreamLogs(context.Background(), info.ID, handle.ExecID)
	if err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	var lines []string
	for item := range logs {
		lines = append(lines, item.Line)
	}
	if len(lines) != 1 || lines[0] != "hello contract" {
		t.Fatalf("logs = %v, want [hello contract]", lines)
	}

	localUpload := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(localUpload, []byte("hello upload"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	remotePath := "/workspace/hello.txt"
	if err := backend.Upload(context.Background(), info.ID, localUpload, remotePath); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	localDownload := filepath.Join(t.TempDir(), "download.txt")
	if err := backend.Download(context.Background(), info.ID, remotePath, localDownload); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	data, err := os.ReadFile(localDownload)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello upload" {
		t.Fatalf("downloaded content = %q, want %q", string(data), "hello upload")
	}

	if err := backend.Delete(context.Background(), info.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("expected sandbox delete to be called")
	}
}

func readUploadedFile(t *testing.T, r *http.Request) (string, []byte) {
	t.Helper()
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("ParseMultipartForm() error = %v", err)
	}
	metaHeaders := r.MultipartForm.File["metadata"]
	fileHeaders := r.MultipartForm.File["file"]
	if len(metaHeaders) != 1 || len(fileHeaders) != 1 {
		t.Fatalf("unexpected upload parts: metadata=%d file=%d", len(metaHeaders), len(fileHeaders))
	}

	metaFile, err := openMultipartFile(metaHeaders[0])
	if err != nil {
		t.Fatalf("open metadata error = %v", err)
	}
	defer metaFile.Close()
	metaBytes, err := io.ReadAll(metaFile)
	if err != nil {
		t.Fatalf("read metadata error = %v", err)
	}
	var metadata struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(metaBytes, &metadata); err != nil {
		t.Fatalf("unmarshal metadata error = %v", err)
	}

	filePart, err := openMultipartFile(fileHeaders[0])
	if err != nil {
		t.Fatalf("open file error = %v", err)
	}
	defer filePart.Close()
	content, err := io.ReadAll(filePart)
	if err != nil {
		t.Fatalf("read file error = %v", err)
	}
	return metadata.Path, content
}

func openMultipartFile(header *multipart.FileHeader) (multipart.File, error) {
	return header.Open()
}

func contractServerURLWithoutScheme(r *http.Request) string {
	return r.Host + "/sandboxes/sbx-1/proxy/44772"
}
