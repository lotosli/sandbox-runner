//go:build s3

package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func uploadS3(ctx context.Context, refs []model.ArtifactRef, cfg model.ArtifactsConfig, deploymentEnv, runID string, attempt int) ([]model.ArtifactRef, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return refs, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.ForcePathStyle
	})
	uploader := manager.NewUploader(client)
	for i := range refs {
		path := refs[i].Path
		if !filepath.IsAbs(path) {
			path = filepath.Clean(path)
		}
		file, err := os.Open(path)
		if err != nil {
			return refs, err
		}
		key := fmt.Sprintf("%s%s/%s/%d/%s", cfg.ObjectPrefix, deploymentEnv, runID, attempt, filepath.Base(path))
		_, err = uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(cfg.Bucket),
			Key:    aws.String(key),
			Body:   file,
		})
		_ = file.Close()
		if err != nil {
			return refs, err
		}
		refs[i].URI = fmt.Sprintf("s3://%s/%s", cfg.Bucket, key)
	}
	return refs, nil
}

func loadAWSConfig(ctx context.Context, cfg model.ArtifactsConfig) (aws.Config, error) {
	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}
	if cfg.Endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
			if service == s3.ServiceID {
				return aws.Endpoint{
					URL:           cfg.Endpoint,
					SigningRegion: cfg.Region,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		})
		opts = append(opts, awscfg.WithEndpointResolverWithOptions(resolver))
	}
	if key := os.Getenv("AWS_ACCESS_KEY_ID"); key != "" {
		opts = append(opts, awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			key,
			os.Getenv("AWS_SECRET_ACCESS_KEY"),
			os.Getenv("AWS_SESSION_TOKEN"),
		)))
	}
	return awscfg.LoadDefaultConfig(ctx, opts...)
}
