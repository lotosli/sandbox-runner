package backend

import (
	"context"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type SandboxBackend interface {
	Kind() model.BackendKind
	Capabilities(ctx context.Context) (model.BackendCapabilities, error)

	Create(ctx context.Context, req CreateSandboxRequest) (SandboxInfo, error)
	Start(ctx context.Context, sandboxID string) error
	Stat(ctx context.Context, sandboxID string) (SandboxStatus, error)

	Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecHandle, error)
	StreamLogs(ctx context.Context, sandboxID string, execID string) (<-chan LogChunk, error)

	Upload(ctx context.Context, sandboxID string, localPath, remotePath string) error
	Download(ctx context.Context, sandboxID string, remotePath, localPath string) error

	Renew(ctx context.Context, sandboxID string, ttl time.Duration) error
	Pause(ctx context.Context, sandboxID string) error
	Resume(ctx context.Context, sandboxID string) error
	Delete(ctx context.Context, sandboxID string) error
}

type WorkspaceSyncer interface {
	SyncWorkspaceIn(ctx context.Context, sandboxID, localDir string) error
	SyncWorkspaceOut(ctx context.Context, sandboxID, remoteDir, localDir string) error
}

type MetadataProvider interface {
	SandboxMetadata(ctx context.Context, sandboxID string) (SandboxInfo, error)
	Endpoints(ctx context.Context, sandboxID string, ports []int) ([]model.Endpoint, error)
}

type ArtifactDownloader interface {
	DownloadArtifact(ctx context.Context, sandboxID, remotePath, localPath string) error
}

type CreateSandboxRequest struct {
	RunID        string            `json:"run_id"`
	Attempt      int               `json:"attempt"`
	WorkspaceID  string            `json:"workspace_id"`
	Image        string            `json:"image"`
	Entrypoint   []string          `json:"entrypoint,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CPU          string            `json:"cpu,omitempty"`
	Memory       string            `json:"memory,omitempty"`
	NetworkMode  string            `json:"network_mode,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	WorkspaceDir string            `json:"workspace_dir,omitempty"`
}

type ExecRequest struct {
	Command    string            `json:"command"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Background bool              `json:"background"`
	Timeout    time.Duration     `json:"timeout,omitempty"`
	Class      string            `json:"class,omitempty"`
}

type SandboxInfo struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	RuntimeKind string            `json:"runtime_kind,omitempty"`
	Endpoints   []model.Endpoint  `json:"endpoints,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

type SandboxStatus struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ExecHandle struct {
	ExecID   string `json:"exec_id"`
	Status   string `json:"status,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type ExecStatus struct {
	ID         string     `json:"id"`
	Running    bool       `json:"running"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type LogChunk struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
}

type ExecStatusProvider interface {
	ExecStatus(ctx context.Context, sandboxID string, execID string) (ExecStatus, error)
}

type ExecCanceler interface {
	CancelExec(ctx context.Context, sandboxID string, execID string) error
}
