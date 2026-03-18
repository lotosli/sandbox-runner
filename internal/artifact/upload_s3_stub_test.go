//go:build !s3

package artifact

import (
	"context"
	"strings"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestDefaultUploaderS3RequiresS3BuildTag(t *testing.T) {
	refs := []model.ArtifactRef{{Name: "results", Path: "/tmp/results.json"}}

	_, err := DefaultUploader{}.Upload(context.Background(), refs, model.ArtifactsConfig{
		Upload:  true,
		Backend: model.ArtifactBackendS3,
	}, "dev", "run-1", 1)
	if err == nil {
		t.Fatal("Upload() error = nil, want missing s3 build tag error")
	}
	if !strings.Contains(err.Error(), "rebuild with -tags s3") {
		t.Fatalf("Upload() error = %v, want s3 build tag hint", err)
	}
}
