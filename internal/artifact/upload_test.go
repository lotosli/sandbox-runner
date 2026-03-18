package artifact

import (
	"context"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestDefaultUploaderLocalReturnsFilePaths(t *testing.T) {
	refs := []model.ArtifactRef{{Name: "results", Path: "/tmp/results.json"}}

	uploaded, err := DefaultUploader{}.Upload(context.Background(), refs, model.ArtifactsConfig{
		Upload:  true,
		Backend: model.ArtifactBackendLocal,
	}, "dev", "run-1", 1)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if uploaded[0].URI != refs[0].Path {
		t.Fatalf("URI = %q, want %q", uploaded[0].URI, refs[0].Path)
	}
}
