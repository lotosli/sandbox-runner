package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/proc"
	mobystdcopy "github.com/moby/moby/api/pkg/stdcopy"
	mobycontainer "github.com/moby/moby/api/types/container"
	mobymount "github.com/moby/moby/api/types/mount"
	mobyclient "github.com/moby/moby/client"
)

type dockerAPI interface {
	ImagePull(ctx context.Context, ref string, options mobyclient.ImagePullOptions) (mobyclient.ImagePullResponse, error)
	ImageInspect(ctx context.Context, image string, opts ...mobyclient.ImageInspectOption) (mobyclient.ImageInspectResult, error)
	ContainerCreate(ctx context.Context, options mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, container string, options mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error)
	ContainerLogs(ctx context.Context, container string, options mobyclient.ContainerLogsOptions) (mobyclient.ContainerLogsResult, error)
	ContainerWait(ctx context.Context, container string, options mobyclient.ContainerWaitOptions) mobyclient.ContainerWaitResult
	ContainerKill(ctx context.Context, container string, options mobyclient.ContainerKillOptions) (mobyclient.ContainerKillResult, error)
	ContainerInspect(ctx context.Context, container string, options mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error)
	ContainerRemove(ctx context.Context, container string, options mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error)
	Ping(ctx context.Context, options mobyclient.PingOptions) (mobyclient.PingResult, error)
	Close() error
}

type DockerExecutor struct {
	runCfg model.RunConfig
	client dockerAPI
}

func NewDockerExecutor(runCfg model.RunConfig) (DockerExecutor, error) {
	cli, err := mobyclient.New(mobyclient.FromEnv, mobyclient.WithAPIVersionNegotiation())
	if err != nil {
		return DockerExecutor{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, mobyclient.PingOptions{}); err != nil {
		_ = cli.Close()
		return DockerExecutor{}, fmt.Errorf("docker daemon unavailable: %w", err)
	}
	return DockerExecutor{runCfg: runCfg, client: cli}, nil
}

func (e DockerExecutor) Run(ctx context.Context, spec Spec, handler proc.IOHandler) (Result, error) {
	started := time.Now().UTC()
	workspace, err := filepath.Abs(spec.Dir)
	if err != nil {
		return Result{}, err
	}

	if err := e.pullImage(ctx, spec.RunConfig.Run.Image); err != nil {
		return Result{}, err
	}

	containerCfg := &mobycontainer.Config{
		Image:        spec.RunConfig.Run.Image,
		Cmd:          spec.Command,
		Env:          envSlice(spec.Env),
		WorkingDir:   "/workspace",
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}
	hostCfg := &mobycontainer.HostConfig{
		AutoRemove: false,
		Mounts: []mobymount.Mount{
			{
				Type:   mobymount.TypeBind,
				Source: workspace,
				Target: "/workspace",
			},
		},
	}

	created, err := e.client.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config:     containerCfg,
		HostConfig: hostCfg,
		Image:      spec.RunConfig.Run.Image,
	})
	if err != nil {
		return Result{}, err
	}
	containerID := created.ID
	defer func() {
		_, _ = e.client.ContainerRemove(context.Background(), containerID, mobyclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	}()

	if _, err := e.client.ContainerStart(ctx, containerID, mobyclient.ContainerStartOptions{}); err != nil {
		return Result{}, err
	}

	logCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		logCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	logReader, err := e.client.ContainerLogs(logCtx, containerID, mobyclient.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "all",
	})
	if err != nil {
		return Result{}, err
	}
	defer logReader.Close()

	stdoutWriter := &dockerLineWriter{ctx: logCtx, stream: "stdout", spec: spec, handler: handler}
	stderrWriter := &dockerLineWriter{ctx: logCtx, stream: "stderr", spec: spec, handler: handler}
	copyErrCh := make(chan error, 1)
	go func() {
		_, err := mobystdcopy.StdCopy(stdoutWriter, stderrWriter, logReader)
		copyErrCh <- err
	}()

	waitResult := e.client.ContainerWait(logCtx, containerID, mobyclient.ContainerWaitOptions{
		Condition: mobycontainer.WaitConditionNotRunning,
	})

	exitCode := 0
	signal := ""
	timedOut := false

	select {
	case waitErr := <-waitResult.Error:
		if waitErr != nil {
			return Result{}, waitErr
		}
	case response := <-waitResult.Result:
		exitCode = int(response.StatusCode)
	case <-logCtx.Done():
		timedOut = errors.Is(logCtx.Err(), context.DeadlineExceeded)
		signal = "SIGKILL"
		_, _ = e.client.ContainerKill(context.Background(), containerID, mobyclient.ContainerKillOptions{Signal: signal})
		select {
		case response := <-waitResult.Result:
			exitCode = int(response.StatusCode)
		case <-time.After(2 * time.Second):
			exitCode = 137
		}
	}

	copyErr := <-copyErrCh
	stdoutWriter.Flush()
	stderrWriter.Flush()
	if copyErr != nil && !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, context.Canceled) {
		return Result{}, copyErr
	}

	inspectResult, inspectErr := e.client.ContainerInspect(context.Background(), containerID, mobyclient.ContainerInspectOptions{})
	if inspectErr != nil {
		inspectErr = fmt.Errorf("inspect container: %w", inspectErr)
	}
	imageDigest := e.imageDigest(context.Background(), spec.RunConfig.Run.Image)
	finished := time.Now().UTC()

	result := Result{
		ExitCode:    exitCode,
		Signal:      signal,
		TimedOut:    timedOut,
		StartedAt:   started,
		FinishedAt:  finished,
		Duration:    finished.Sub(started),
		StdoutLines: stdoutWriter.lines,
		StderrLines: stderrWriter.lines,
		Target: model.ExecutionTarget{
			OS:              "linux",
			Arch:            spec.Target.Arch,
			Mode:            spec.RunConfig.Platform.RunMode,
			ContainerID:     containerID,
			ContainerImage:  spec.RunConfig.Run.Image,
			ImageDigest:     imageDigest,
			InContainer:     spec.RunConfig.Platform.ContainerExecutionMode == model.ContainerExecutionInContainerMode,
			InKubernetes:    false,
			DockerAvailable: true,
			Capabilities:    inspectCapabilities(inspectResult.Container),
		},
		Metadata: map[string]any{
			"container_id":             containerID,
			"container_image":          spec.RunConfig.Run.Image,
			"container_execution_mode": spec.RunConfig.Platform.ContainerExecutionMode,
			"mounts":                   inspectMounts(inspectResult.Container),
			"warnings":                 created.Warnings,
		},
	}
	if rawInspect := inspectRaw(inspectResult.Raw); rawInspect != nil {
		result.Metadata["inspect"] = rawInspect
	}
	if inspectErr != nil {
		result.Metadata["inspect_error"] = inspectErr.Error()
	}
	if timedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func (e DockerExecutor) pullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("run.image is required for docker mode")
	}
	resp, err := e.client.ImagePull(ctx, image, mobyclient.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()
	return resp.Wait(ctx)
}

func (e DockerExecutor) imageDigest(ctx context.Context, image string) string {
	if image == "" {
		return ""
	}
	inspectResult, err := e.client.ImageInspect(ctx, image)
	if err != nil {
		return ""
	}
	if len(inspectResult.RepoDigests) > 0 {
		return inspectResult.RepoDigests[0]
	}
	return inspectResult.ID
}

type dockerLineWriter struct {
	ctx     context.Context
	stream  string
	spec    Spec
	handler proc.IOHandler
	buffer  bytes.Buffer
	lines   int
}

func (w *dockerLineWriter) Write(p []byte) (int, error) {
	_, _ = w.buffer.Write(p)
	w.drainCompleteLines()
	return len(p), nil
}

func (w *dockerLineWriter) Flush() {
	if w.buffer.Len() == 0 {
		return
	}
	w.emitLine(strings.TrimRight(w.buffer.String(), "\r\n"))
	w.buffer.Reset()
}

func (w *dockerLineWriter) drainCompleteLines() {
	for {
		data := w.buffer.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			return
		}

		line := strings.TrimRight(string(data[:idx]), "\r")
		w.emitLine(line)

		remaining := append([]byte(nil), data[idx+1:]...)
		w.buffer.Reset()
		_, _ = w.buffer.Write(remaining)
	}
}

func (w *dockerLineWriter) emitLine(line string) {
	w.lines++
	if w.spec.LogLineMaxBytes > 0 && len(line) > w.spec.LogLineMaxBytes {
		line = line[:w.spec.LogLineMaxBytes]
	}
	if w.handler != nil {
		_ = w.handler.OnLog(w.ctx, model.StructuredLog{
			Timestamp:    time.Now().UTC(),
			RunID:        w.spec.RunID,
			Attempt:      w.spec.Attempt,
			Phase:        w.spec.Phase,
			CommandClass: w.spec.CommandClass,
			Stream:       w.stream,
			LineNo:       w.lines,
			Line:         proc.Redact(line),
		})
	}
}

func envSlice(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	return items
}

func inspectRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func inspectMounts(inspect mobycontainer.InspectResponse) any {
	return inspect.Mounts
}

func inspectCapabilities(inspect mobycontainer.InspectResponse) []string {
	if inspect.HostConfig == nil {
		return nil
	}
	return normalizeCaps(inspect.HostConfig.CapAdd)
}

func normalizeCaps(caps []string) []string {
	if len(caps) == 0 {
		return nil
	}
	out := make([]string, 0, len(caps))
	for _, cap := range caps {
		out = append(out, strings.ToLower(cap))
	}
	return out
}
