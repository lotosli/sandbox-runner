package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type Command interface {
	Run(ctx context.Context) (int, error)
}

type App struct {
	Command Command
}

func (a App) Run(ctx context.Context) (int, error) {
	return a.Command.Run(ctx)
}

type GlobalOptions struct {
	ConfigPath string
	PolicyPath string
}

var (
	versionValue    = "dev"
	gitSHAValue     = "unknown"
	buildTimeValue  = "unknown"
	buildTargetOS   = runtime.GOOS
	buildTargetArch = runtime.GOARCH
)

var versionInfo = model.VersionInfo{
	Version:    versionValue,
	GitSHA:     gitSHAValue,
	BuildTime:  buildTimeValue,
	TargetOS:   buildTargetOS,
	TargetArch: buildTargetArch,
}

func Parse(args []string) (App, error) {
	if len(args) == 0 {
		return App{Command: &RunCommand{Args: []string{}, Opts: GlobalOptions{}}}, nil
	}

	switch args[0] {
	case "run":
		return App{Command: parseRunCommand(args[1:])}, nil
	case "docker":
		return App{Command: parseDockerCommand(args[1:])}, nil
	case "validate":
		return App{Command: parseValidateCommand(args[1:])}, nil
	case "replay":
		return App{Command: parseReplayCommand(args[1:])}, nil
	case "doctor":
		return App{Command: parseDoctorCommand(args[1:])}, nil
	case "version", "--version", "-version":
		return App{Command: VersionCommand{}}, nil
	case "k8s":
		return parseK8sCommand(args[1:])
	default:
		return App{Command: parseRunCommand(args)}, nil
	}
}

func parseRunCommand(args []string) *RunCommand {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	opts := GlobalOptions{}
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to run config")
	fs.StringVar(&opts.PolicyPath, "policy", "", "Path to policy config")
	_ = fs.Parse(args)
	return &RunCommand{
		Args: fs.Args(),
		Opts: opts,
	}
}

func parseDockerCommand(args []string) *DockerCommand {
	fs := flag.NewFlagSet("docker", flag.ContinueOnError)
	opts := GlobalOptions{}
	image := fs.String("image", "", "Docker image")
	mode := fs.String("mode", string(model.ContainerExecutionHostRunner), "Docker execution mode")
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to run config")
	fs.StringVar(&opts.PolicyPath, "policy", "", "Path to policy config")
	_ = fs.Parse(args)
	return &DockerCommand{
		RunCommand: RunCommand{
			Args: fs.Args(),
			Opts: opts,
		},
		Image:                  *image,
		ContainerExecutionMode: model.ContainerExecutionMode(*mode),
	}
}

func parseValidateCommand(args []string) *ValidateCommand {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	opts := GlobalOptions{}
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to run config")
	fs.StringVar(&opts.PolicyPath, "policy", "", "Path to policy config")
	_ = fs.Parse(args)
	return &ValidateCommand{Opts: opts}
}

func parseReplayCommand(args []string) *ReplayCommand {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	artifactDir := fs.String("artifact-dir", ".sandbox-runner", "Artifact dir")
	_ = fs.Parse(args)
	return &ReplayCommand{ArtifactDir: *artifactDir}
}

func parseDoctorCommand(args []string) *DoctorCommand {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	image := fs.String("image", "", "Optional Docker image for validation")
	_ = fs.Parse(args)
	return &DoctorCommand{Image: *image}
}

func parseK8sCommand(args []string) (App, error) {
	if len(args) == 0 {
		return App{}, errors.New("missing k8s subcommand")
	}
	switch args[0] {
	case "render-job":
		return App{Command: parseK8sRenderCommand(args[1:])}, nil
	case "submit-job":
		return App{Command: parseK8sSubmitCommand(args[1:])}, nil
	default:
		return App{}, fmt.Errorf("unsupported k8s subcommand: %s", args[0])
	}
}

func parseK8sRenderCommand(args []string) *K8sRenderCommand {
	fs := flag.NewFlagSet("k8s render-job", flag.ContinueOnError)
	opts := GlobalOptions{}
	namespace := fs.String("namespace", "ai-sandbox-runner-runs", "Kubernetes namespace")
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to run config")
	fs.StringVar(&opts.PolicyPath, "policy", "", "Path to policy config")
	_ = fs.Parse(args)
	return &K8sRenderCommand{Opts: opts, Namespace: *namespace}
}

func parseK8sSubmitCommand(args []string) *K8sSubmitCommand {
	fs := flag.NewFlagSet("k8s submit-job", flag.ContinueOnError)
	opts := GlobalOptions{}
	namespace := fs.String("namespace", "ai-sandbox-runner-runs", "Kubernetes namespace")
	kubeconfig := fs.String("kubeconfig", "", "Kubeconfig path")
	kubeContext := fs.String("context", "", "Kubernetes context")
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to run config")
	fs.StringVar(&opts.PolicyPath, "policy", "", "Path to policy config")
	_ = fs.Parse(args)
	return &K8sSubmitCommand{Opts: opts, Namespace: *namespace, Kubeconfig: *kubeconfig, KubeContext: *kubeContext}
}

func loadRequest(opts GlobalOptions) (model.RunRequest, error) {
	runCfg, err := config.LoadRunConfig(opts.ConfigPath)
	if err != nil {
		return model.RunRequest{}, err
	}
	policy, err := config.LoadPolicyConfig(opts.PolicyPath)
	if err != nil {
		return model.RunRequest{}, err
	}
	return model.RunRequest{
		ConfigPath: opts.ConfigPath,
		PolicyPath: opts.PolicyPath,
		RunConfig:  runCfg,
		Policy:     policy,
		Version:    versionInfo,
	}, nil
}

func withCommand(cfg model.RunConfig, args []string) model.RunConfig {
	if len(args) > 0 {
		cfg.Run.Command = args
	}
	return cfg
}

func printYAMLPlaceholder(value any) error {
	_, err := fmt.Fprintf(os.Stdout, "%v\n", value)
	return err
}
