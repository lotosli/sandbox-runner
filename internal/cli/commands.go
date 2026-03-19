package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/artifact"
	"github.com/lotosli/sandbox-runner/internal/kubernetes"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/phase"
	"github.com/lotosli/sandbox-runner/internal/platform"
	"github.com/lotosli/sandbox-runner/pkg/sdk"
)

type RunCommand struct {
	Args        []string
	Opts        GlobalOptions
	JSONSummary bool
}

var runCommandStdout io.Writer = os.Stdout

func (c *RunCommand) Run(ctx context.Context) (int, error) {
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	req.RunConfig = withCommand(req.RunConfig, c.Args)
	req.Target = platform.Detect(req.RunConfig.Platform.RunMode)
	engine := phase.NewEngine()
	result, err := engine.Run(ctx, &req)
	printRunSummary(runCommandStdout, req.RunConfig.Run.ArtifactDir, result, c.JSONSummary)
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
	printRunSummary(runCommandStdout, req.RunConfig.Run.ArtifactDir, result, c.JSONSummary)
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
	req = ensureK8sRequestIdentity(req)
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
	Opts        GlobalOptions
	Namespace   string
	Kubeconfig  string
	KubeContext string
}

func (c *K8sSubmitCommand) Run(ctx context.Context) (int, error) {
	req, err := loadRequest(c.Opts)
	if err != nil {
		return 1, err
	}
	req = ensureK8sRequestIdentity(req)
	builder := sdk.NewJobSpecBuilder()
	spec, err := builder.Build(req, c.Namespace)
	if err != nil {
		return 1, err
	}
	configMap, err := kubernetes.BuildConfigMap(req, c.Namespace)
	if err != nil {
		return 1, err
	}
	contextName := c.KubeContext
	if contextName == "" {
		contextName = req.RunConfig.K8s.Context
	}
	kubeconfig := c.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = req.RunConfig.K8s.Kubeconfig
	}
	submitter, err := sdk.NewSubmitter(kubeconfig, contextName, req.RunConfig.K8s.Provider)
	if err != nil {
		return 1, err
	}
	if _, err := submitter.ApplyConfigMap(ctx, configMap); err != nil {
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
	report := platform.DoctorReport("")
	payload, err := json.MarshalIndent(struct {
		model.VersionInfo
		ExecutionTarget model.ExecutionTarget `json:"execution_target"`
		FeatureGates    model.FeatureSet      `json:"feature_gates"`
		Warnings        []string              `json:"warnings,omitempty"`
	}{
		VersionInfo:     versionInfo,
		ExecutionTarget: report.ExecutionTarget,
		FeatureGates:    report.FeatureGates,
		Warnings:        report.Warnings,
	}, "", "  ")
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

func printRunSummary(w io.Writer, artifactDir string, result *model.RunResult, jsonSummary bool) {
	if w == nil || result == nil {
		return
	}
	summary, err := buildRunSummary(artifactDir, result)
	if err != nil {
		_, _ = fmt.Fprintf(w, "failed to build run summary: %v\n", err)
		return
	}
	if jsonSummary {
		payload, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(w, "failed to encode run summary: %v\n", err)
			return
		}
		_, _ = fmt.Fprintln(w, string(payload))
		return
	}
	_, _ = fmt.Fprintln(w, formatRunSummary(summary))
}

type runSummary struct {
	RunID          string            `json:"run_id"`
	Status         model.RunStatus   `json:"status"`
	Phase          model.Phase       `json:"phase"`
	ExitCode       int               `json:"exit_code"`
	TimedOut       bool              `json:"timed_out"`
	Signal         string            `json:"signal,omitempty"`
	CommandClass   string            `json:"command_class,omitempty"`
	BackendKind    string            `json:"backend_kind,omitempty"`
	ProviderName   string            `json:"provider_name,omitempty"`
	RuntimeProfile string            `json:"runtime_profile,omitempty"`
	ArtifactDir    string            `json:"artifact_dir"`
	Files          map[string]string `json:"files"`
	ReadOrder      []string          `json:"suggested_read_order,omitempty"`
	StdoutTail     []string          `json:"stdout_tail,omitempty"`
	StderrTail     []string          `json:"stderr_tail,omitempty"`
}

func buildRunSummary(artifactDir string, result *model.RunResult) (runSummary, error) {
	stdoutTail, err := readStructuredLogTail(filepath.Join(artifactDir, artifact.StdoutFileName), 3)
	if err != nil {
		return runSummary{}, err
	}
	stderrTail, err := readStructuredLogTail(filepath.Join(artifactDir, artifact.StderrFileName), 3)
	if err != nil {
		return runSummary{}, err
	}
	return runSummary{
		RunID:          result.RunID,
		Status:         result.Status,
		Phase:          result.Phase,
		ExitCode:       result.ExitCode,
		TimedOut:       result.TimedOut,
		Signal:         result.Signal,
		CommandClass:   result.CommandClass,
		BackendKind:    result.BackendKind,
		ProviderName:   result.ProviderName,
		RuntimeProfile: result.RuntimeProfile,
		ArtifactDir:    artifactDir,
		Files:          runSummaryFiles(artifactDir),
		ReadOrder:      suggestedReadOrder(runSummaryFiles(artifactDir)),
		StdoutTail:     stdoutTail,
		StderrTail:     stderrTail,
	}, nil
}

func formatRunSummary(summary runSummary) string {
	lines := []string{
		fmt.Sprintf("run_id: %s", summary.RunID),
		fmt.Sprintf("status: %s", summary.Status),
		fmt.Sprintf("phase: %s", summary.Phase),
		fmt.Sprintf("exit_code: %d", summary.ExitCode),
		fmt.Sprintf("artifact_dir: %s", summary.ArtifactDir),
		fmt.Sprintf("index: %s", summary.Files["index"]),
		fmt.Sprintf("results: %s", summary.Files["results"]),
		fmt.Sprintf("phases: %s", summary.Files["phases"]),
		fmt.Sprintf("replay: %s", summary.Files["replay"]),
		fmt.Sprintf("commands: %s", summary.Files["commands"]),
		fmt.Sprintf("stdout: %s", summary.Files["stdout"]),
		fmt.Sprintf("stderr: %s", summary.Files["stderr"]),
	}
	lines = append(lines, formatSummaryTail("stdout tail", summary.StdoutTail)...)
	lines = append(lines, formatSummaryTail("stderr tail", summary.StderrTail)...)
	return strings.Join(lines, "\n")
}

func formatSummaryTail(label string, lines []string) []string {
	if len(lines) == 0 {
		return []string{label + ": <empty>"}
	}
	out := []string{label + ":"}
	for _, line := range lines {
		out = append(out, "- "+line)
	}
	return out
}

func readStructuredLogTail(path string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	ring := make([]string, 0, limit)
	scanner := bufio.NewScanner(file)
	const maxSummaryLineBytes = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxSummaryLineBytes)
	for scanner.Scan() {
		var log model.StructuredLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			continue
		}
		line := strings.TrimSpace(log.Line)
		if line == "" {
			continue
		}
		if len(ring) == limit {
			copy(ring, ring[1:])
			ring[len(ring)-1] = line
			continue
		}
		ring = append(ring, line)
	}
	if err := scanner.Err(); err != nil {
		return ring, err
	}
	return ring, nil
}

func runSummaryFiles(artifactDir string) map[string]string {
	return map[string]string{
		"index":    filepath.Join(artifactDir, artifact.IndexFileName),
		"results":  resultsPath(artifactDir),
		"phases":   filepath.Join(artifactDir, artifact.PhasesFileName),
		"replay":   replayManifestPath(artifactDir),
		"commands": filepath.Join(artifactDir, artifact.CommandsFileName),
		"stdout":   filepath.Join(artifactDir, artifact.StdoutFileName),
		"stderr":   filepath.Join(artifactDir, artifact.StderrFileName),
		"context":  artifactContextPath(artifactDir),
	}
}

func ensureK8sRequestIdentity(req model.RunRequest) model.RunRequest {
	if strings.TrimSpace(req.RunConfig.Run.RunID) == "" {
		req.RunConfig.Run.RunID = fmt.Sprintf("r-%s", time.Now().UTC().Format("20060102-150405"))
	}
	return req
}

func suggestedReadOrder(files map[string]string) []string {
	order := []string{"index", "results", "phases", "commands", "stdout", "stderr", "replay", "context"}
	out := make([]string, 0, len(order))
	for _, key := range order {
		if files[key] != "" {
			out = append(out, key)
		}
	}
	return out
}
