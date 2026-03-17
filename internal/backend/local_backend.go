package backend

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

var errUnsupported = errors.New("operation not supported by local backend")

type LocalBackend struct {
	kind         model.BackendKind
	cfg          model.RunConfig
	capabilities model.BackendCapabilities
}

func NewLocalBackend(kind model.BackendKind, cfg model.RunConfig) *LocalBackend {
	caps := model.BackendCapabilities{}
	if kind == model.BackendKindDocker {
		caps.SupportsStreamLogs = true
		caps.SupportsBridgeNetwork = true
		caps.SupportsHostNetwork = true
		caps.SupportsOCIImage = true
	}
	if kind == model.BackendKindK8s {
		caps.SupportsEndpoints = true
		caps.SupportsFileUpload = true
		caps.SupportsFileDownload = true
		caps.SupportsRuntimeProfile = true
		caps.SupportsK8sTarget = true
	}
	return &LocalBackend{kind: kind, cfg: cfg, capabilities: caps}
}

func (b *LocalBackend) Kind() model.BackendKind { return b.kind }

func (b *LocalBackend) Capabilities(ctx context.Context) (model.BackendCapabilities, error) {
	_ = ctx
	return b.capabilities, nil
}

func (b *LocalBackend) RuntimeInfo(ctx context.Context) (model.RuntimeInfo, error) {
	_ = ctx
	info := model.RuntimeInfo{
		ProviderKind:     string(b.kind),
		BackendProvider:  backendProviderForConfig(b.cfg),
		RuntimeProfile:   string(b.cfg.Runtime.Profile),
		RuntimeClassName: b.cfg.Kata.RuntimeClassName,
		HostOS:           runtime.GOOS,
		HostArch:         runtime.GOARCH,
		Available:        true,
	}
	switch b.kind {
	case model.BackendKindDocker:
		info.ContainerRuntime = "docker"
		if b.cfg.Docker.Provider == model.DockerProviderOrbStack {
			info.LocalPlatform = "orbstack"
		}
	case model.BackendKindK8s:
		info.ContainerRuntime = "kubernetes"
		if b.cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
			info.LocalPlatform = "orbstack"
		}
	default:
		info.ContainerRuntime = string(b.kind)
	}
	if b.cfg.Runtime.Profile == model.RuntimeProfileKata {
		info.Virtualization = "kata"
	} else if b.cfg.Runtime.Profile == model.RuntimeProfileOrbStackDocker || b.cfg.Runtime.Profile == model.RuntimeProfileOrbStackK8s {
		info.Virtualization = "none"
	} else {
		info.Virtualization = "none"
	}
	return info, nil
}

func (b *LocalBackend) Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error) {
	_ = ctx
	return SandboxInfo{
		ID:          req.RunID,
		Status:      "ready",
		RuntimeKind: string(b.kind),
		Metadata:    req.Metadata,
	}, nil
}

func (b *LocalBackend) Start(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}

func (b *LocalBackend) Stat(ctx context.Context, sandboxID string) (SandboxStatus, error) {
	_ = ctx
	return SandboxStatus{ID: sandboxID, Status: "ready"}, nil
}

func (b *LocalBackend) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error) {
	_ = ctx
	_ = sandboxID
	_ = req
	return ExecHandle{}, errUnsupported
}

func (b *LocalBackend) StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error) {
	_ = ctx
	_ = sandboxID
	_ = execID
	return nil, errUnsupported
}

func (b *LocalBackend) Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error {
	_ = ctx
	_ = sandboxID
	_ = localPath
	_ = remotePath
	return nil
}

func (b *LocalBackend) Download(ctx context.Context, sandboxID string, remotePath, localPath string) error {
	_ = ctx
	_ = sandboxID
	_ = remotePath
	_ = localPath
	return nil
}

func (b *LocalBackend) Renew(ctx context.Context, sandboxID string, ttl time.Duration) error {
	_ = ctx
	_ = sandboxID
	_ = ttl
	return errUnsupported
}

func (b *LocalBackend) Pause(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *LocalBackend) Resume(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return errUnsupported
}

func (b *LocalBackend) Delete(ctx context.Context, sandboxID string) error {
	_ = ctx
	_ = sandboxID
	return nil
}
