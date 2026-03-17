package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lotosli/sandbox-runner/internal/artifact"
	"github.com/lotosli/sandbox-runner/internal/kubernetes"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/phase"
	"github.com/lotosli/sandbox-runner/internal/platform"
	"github.com/lotosli/sandbox-runner/pkg/sdk"
)

type RunCommand struct {
	Args []string
	Opts GlobalOptions
}

func (c *RunCommand) Run(ctx context.Context) (int, error) {
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	req.RunConfig = withCommand(req.RunConfig, c.Args)
	req.Target = platform.Detect(req.RunConfig.Platform.RunMode)
	engine := phase.NewEngine()
	result, err := engine.Run(ctx, &req)
	if err != nil {
		return phase.ExitCodeForResult(result, err), err
	}
	return result.ExitCode, nil
}

type DockerCommand struct {
	RunCommand
	Image                  string
	ContainerExecutionMode model.ContainerExecutionMode
}

func (c *DockerCommand) Run(ctx context.Context) (int, error) {
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	req.RunConfig = withCommand(req.RunConfig, c.Args)
	req.RunConfig.Platform.RunMode = model.RunModeLocalDocker
	req.RunConfig.Backend.Kind = model.BackendKindDocker
	req.RunConfig.Platform.ContainerExecutionMode = c.ContainerExecutionMode
	if c.Image != "" {
		req.RunConfig.Run.Image = c.Image
	}
	req.Target = platform.Detect(req.RunConfig.Platform.RunMode)
	engine := phase.NewEngine()
	result, err := engine.Run(ctx, &req)
	if err != nil {
		return phase.ExitCodeForResult(result, err), err
	}
	return result.ExitCode, nil
}

type ValidateCommand struct {
	Opts GlobalOptions
}

func (c *ValidateCommand) Run(ctx context.Context) (int, error) {
	_ = ctx
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	payload, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return 1, err
	}
	fmt.Println(string(payload))
	return 0, nil
}

type ReplayCommand struct {
	ArtifactDir string
}

func (c *ReplayCommand) Run(ctx context.Context) (int, error) {
	_ = ctx
	manifestPath := filepath.Join(c.ArtifactDir, "replay.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return 1, err
	}
	fmt.Println(string(data))
	return 0, nil
}

type DoctorCommand struct {
	Image string
}

func (c *DoctorCommand) Run(ctx context.Context) (int, error) {
	_ = ctx
	report := platform.DoctorReport(c.Image)
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return 1, err
	}
	fmt.Println(string(payload))
	return 0, nil
}

type K8sRenderCommand struct {
	Opts      GlobalOptions
	Namespace string
}

func (c *K8sRenderCommand) Run(ctx context.Context) (int, error) {
	_ = ctx
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	builder := sdk.NewJobSpecBuilder()
	spec, err := builder.Build(req, c.Namespace)
	if err != nil {
		return 1, err
	}
	out, err := kubernetes.RenderJobYAML(spec)
	if err != nil {
		return 1, err
	}
	fmt.Print(out)
	return 0, nil
}

type K8sSubmitCommand struct {
	Opts       GlobalOptions
	Namespace  string
	Kubeconfig string
}

func (c *K8sSubmitCommand) Run(ctx context.Context) (int, error) {
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	builder := sdk.NewJobSpecBuilder()
	spec, err := builder.Build(req, c.Namespace)
	if err != nil {
		return 1, err
	}
	submitter, err := sdk.NewSubmitter(c.Kubeconfig)
	if err != nil {
		return 1, err
	}
	result, err := submitter.SubmitJob(ctx, spec)
	if err != nil {
		return 1, err
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return 1, err
	}
	fmt.Println(string(payload))
	return 0, nil
}

type VersionCommand struct{}

func (VersionCommand) Run(ctx context.Context) (int, error) {
	_ = ctx
	payload, err := json.MarshalIndent(versionInfo, "", "  ")
	if err != nil {
		return 1, err
	}
	fmt.Println(string(payload))
	return 0, nil
}

func replayManifestPath(dir string) string {
	return filepath.Join(dir, "replay.json")
}

func resultsPath(dir string) string {
	return filepath.Join(dir, "results.json")
}

func artifactContextPath(dir string) string {
	return filepath.Join(dir, artifact.ContextFileName)
}
