//go:build !s3

package artifact

import (
	"context"
	"fmt"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func uploadS3(ctx context.Context, refs []model.ArtifactRef, cfg model.ArtifactsConfig, deploymentEnv, runID string, attempt int) ([]model.ArtifactRef, error) {
	_ = ctx
	_ = cfg
	_ = deploymentEnv
	_ = runID
	_ = attempt
	return refs, fmt.Errorf("artifact backend s3 is not available in this binary; rebuild with -tags s3")
}
