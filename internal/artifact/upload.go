package artifact

import (
	"context"
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type Uploader interface {
	Upload(ctx context.Context, refs []model.ArtifactRef, cfg model.ArtifactsConfig, deploymentEnv, runID string, attempt int) ([]model.ArtifactRef, error)
}

type DefaultUploader struct{}

func (DefaultUploader) Upload(ctx context.Context, refs []model.ArtifactRef, cfg model.ArtifactsConfig, deploymentEnv, runID string, attempt int) ([]model.ArtifactRef, error) {
	if !cfg.Upload {
		return refs, nil
	}
	switch cfg.Backend {
	case model.ArtifactBackendLocal:
		for i := range refs {
			refs[i].URI = refs[i].Path
		}
		return refs, nil
	case model.ArtifactBackendS3:
		return uploadS3(ctx, refs, cfg, deploymentEnv, runID, attempt)
	default:
		return refs, fmt.Errorf("unsupported artifact backend: %s", cfg.Backend)
	}
}
