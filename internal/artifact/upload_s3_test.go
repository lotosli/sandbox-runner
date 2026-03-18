//go:build s3

package artifact

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestDefaultUploaderS3UploadsToConfiguredEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	artifactPath := filepath.Join(tmpDir, "results.json")
	if err := os.WriteFile(artifactPath, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/runner-artifacts/runs/dev/run-1/1/results.json" {
			t.Fatalf("path = %s, want /runner-artifacts/runs/dev/run-1/1/results.json", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Fatalf("body = %q, want JSON artifact payload", string(body))
		}
		w.Header().Set("ETag", `"test-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "")

	uploaded, err := DefaultUploader{}.Upload(context.Background(), []model.ArtifactRef{{
		Name: "results",
		Path: artifactPath,
	}}, model.ArtifactsConfig{
		Upload:         true,
		Backend:        model.ArtifactBackendS3,
		Bucket:         "runner-artifacts",
		Endpoint:       server.URL,
		Region:         "us-east-1",
		ForcePathStyle: true,
		ObjectPrefix:   "runs/",
	}, "dev", "run-1", 1)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if len(uploaded) != 1 {
		t.Fatalf("len(uploaded) = %d, want 1", len(uploaded))
	}
	if uploaded[0].URI != "s3://runner-artifacts/runs/dev/run-1/1/results.json" {
		t.Fatalf("URI = %q, want s3://runner-artifacts/runs/dev/run-1/1/results.json", uploaded[0].URI)
	}
}
