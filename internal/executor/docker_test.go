package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"iter"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
	mobycontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	mobyclient "github.com/moby/moby/client"
)

type fakeDockerClient struct {
	pullImage    string
	createdID    string
	logs         []byte
	waitStatus   int64
	inspect      mobyclient.ContainerInspectResult
	imageInspect mobyclient.ImageInspectResult
}

func (f *fakeDockerClient) ImagePull(ctx context.Context, ref string, options mobyclient.ImagePullOptions) (mobyclient.ImagePullResponse, error) {
	_ = ctx
	_ = options
	f.pullImage = ref
	return fakeImagePullResponse{ReadCloser: io.NopCloser(bytes.NewReader(nil))}, nil
}

func (f *fakeDockerClient) ImageInspect(ctx context.Context, image string, opts ...mobyclient.ImageInspectOption) (mobyclient.ImageInspectResult, error) {
	_ = ctx
	_ = image
	_ = opts
	return f.imageInspect, nil
}

func (f *fakeDockerClient) ContainerCreate(ctx context.Context, options mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error) {
	_ = ctx
	_ = options
	if f.createdID == "" {
		f.createdID = "ctr-123"
	}
	return mobyclient.ContainerCreateResult{ID: f.createdID}, nil
}

func (f *fakeDockerClient) ContainerStart(ctx context.Context, container string, options mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error) {
	_ = ctx
	_ = container
	_ = options
	return mobyclient.ContainerStartResult{}, nil
}

func (f *fakeDockerClient) ContainerLogs(ctx context.Context, container string, options mobyclient.ContainerLogsOptions) (mobyclient.ContainerLogsResult, error) {
	_ = ctx
	_ = container
	_ = options
	return io.NopCloser(bytes.NewReader(f.logs)), nil
}

func (f *fakeDockerClient) ContainerWait(ctx context.Context, container string, options mobyclient.ContainerWaitOptions) mobyclient.ContainerWaitResult {
	_ = ctx
	_ = container
	_ = options
	resultCh := make(chan mobycontainer.WaitResponse, 1)
	errCh := make(chan error, 1)
	resultCh <- mobycontainer.WaitResponse{StatusCode: f.waitStatus}
	return mobyclient.ContainerWaitResult{Result: resultCh, Error: errCh}
}

func (f *fakeDockerClient) ContainerKill(ctx context.Context, container string, options mobyclient.ContainerKillOptions) (mobyclient.ContainerKillResult, error) {
	_ = ctx
	_ = container
	_ = options
	return mobyclient.ContainerKillResult{}, nil
}

func (f *fakeDockerClient) ContainerInspect(ctx context.Context, container string, options mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error) {
	_ = ctx
	_ = container
	_ = options
	return f.inspect, nil
}

func (f *fakeDockerClient) ContainerRemove(ctx context.Context, container string, options mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error) {
	_ = ctx
	_ = container
	_ = options
	return mobyclient.ContainerRemoveResult{}, nil
}

func (f *fakeDockerClient) Ping(ctx context.Context, options mobyclient.PingOptions) (mobyclient.PingResult, error) {
	_ = ctx
	_ = options
	return mobyclient.PingResult{}, nil
}

func (f *fakeDockerClient) Close() error { return nil }

type fakeImagePullResponse struct {
	io.ReadCloser
}

func (f fakeImagePullResponse) JSONMessages(ctx context.Context) iter.Seq2[jsonstream.Message, error] {
	_ = ctx
	return func(yield func(jsonstream.Message, error) bool) {}
}

func (f fakeImagePullResponse) Wait(ctx context.Context) error {
	_ = ctx
	return nil
}

type captureLogs struct {
	logs []model.StructuredLog
}

func (c *captureLogs) OnLog(ctx context.Context, log model.StructuredLog) error {
	_ = ctx
	c.logs = append(c.logs, log)
	return nil
}

func TestDockerExecutorRunUsesSDKClient(t *testing.T) {
	raw := multiplexedLogs(
		dockerFrame{stream: 1, payload: "hello from stdout\n"},
		dockerFrame{stream: 2, payload: "hello from stderr\n"},
	)

	cfg := config.DefaultRunConfig()
	cfg.Run.Image = "alpine:3.20"
	cfg.Platform.RunMode = model.RunModeLocalDocker

	exec := DockerExecutor{
		runCfg: cfg,
		client: &fakeDockerClient{
			logs:       raw,
			waitStatus: 0,
			inspect: mobyclient.ContainerInspectResult{
				Container: mobycontainer.InspectResponse{
					HostConfig: &mobycontainer.HostConfig{CapAdd: []string{"CAP_NET_BIND_SERVICE"}},
				},
			},
			imageInspect: mobyclient.ImageInspectResult{},
		},
	}

	handler := &captureLogs{}
	result, err := exec.Run(context.Background(), Spec{
		Phase:           model.PhaseExecute,
		Command:         []string{"go", "test", "./..."},
		Env:             map[string]string{"RUN_ID": "r-1"},
		Dir:             t.TempDir(),
		Timeout:         3 * time.Second,
		RunID:           "r-1",
		Attempt:         1,
		CommandClass:    "test.run",
		ArtifactDir:     t.TempDir(),
		LogLineMaxBytes: 8192,
		RunConfig:       cfg,
		Target:          model.ExecutionTarget{Arch: "arm64"},
	}, handler)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Target.ContainerID != "ctr-123" {
		t.Fatalf("container id = %s, want ctr-123", result.Target.ContainerID)
	}
	if result.StdoutLines != 1 || result.StderrLines != 1 {
		t.Fatalf("stdout/stderr lines = %d/%d, want 1/1", result.StdoutLines, result.StderrLines)
	}
	if len(handler.logs) != 2 {
		t.Fatalf("captured logs = %d, want 2", len(handler.logs))
	}
}

func TestDockerClientHostUsesCurrentDockerContextWhenConfigUsesDefault(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDocker
	cfg.Docker.Context = "default"

	origShow := dockerContextShow
	origInspect := dockerContextInspect
	defer func() {
		dockerContextShow = origShow
		dockerContextInspect = origInspect
	}()

	dockerContextShow = func() string { return "orbstack" }
	dockerContextInspect = func(name string) (string, error) {
		if name != "orbstack" {
			t.Fatalf("inspect name = %q, want orbstack", name)
		}
		return "unix:///tmp/orbstack.sock", nil
	}

	host, err := dockerClientHost(cfg)
	if err != nil {
		t.Fatalf("dockerClientHost() error = %v", err)
	}
	if host != "unix:///tmp/orbstack.sock" {
		t.Fatalf("host = %q, want unix:///tmp/orbstack.sock", host)
	}
}

type dockerFrame struct {
	stream  byte
	payload string
}

func multiplexedLogs(frames ...dockerFrame) []byte {
	var buf bytes.Buffer
	for _, frame := range frames {
		header := make([]byte, 8)
		header[0] = frame.stream
		binary.BigEndian.PutUint32(header[4:], uint32(len(frame.payload)))
		_, _ = buf.Write(header)
		_, _ = buf.WriteString(frame.payload)
	}
	return buf.Bytes()
}
