package model

import "time"

type Phase string

const (
	PhasePrepare Phase = "prepare"
	PhaseSetup   Phase = "setup"
	PhaseExecute Phase = "execute"
	PhaseVerify  Phase = "verify"
	PhaseCollect Phase = "collect"
)

var OrderedPhases = []Phase{
	PhasePrepare,
	PhaseSetup,
	PhaseExecute,
	PhaseVerify,
	PhaseCollect,
}

type RunStatus string

const (
	StatusCreated   RunStatus = "CREATED"
	StatusSucceeded RunStatus = "SUCCEEDED"
	StatusFailed    RunStatus = "FAILED"
	StatusTimedOut  RunStatus = "TIMED_OUT"
	StatusAborted   RunStatus = "ABORTED"
	StatusPartial   RunStatus = "PARTIAL"
)

type RunMode string

const (
	RunModeLocalDirect            RunMode = "local_direct"
	RunModeLocalDocker            RunMode = "local_docker"
	RunModeSTGLinux               RunMode = "stg_linux"
	RunModeLocalOpenSandboxDocker RunMode = "local_opensandbox_docker"
	RunModeSTGOpenSandboxK8s      RunMode = "stg_opensandbox_k8s"
)

type BackendKind string

const (
	BackendKindDirect      BackendKind = "direct"
	BackendKindDocker      BackendKind = "docker"
	BackendKindK8s         BackendKind = "k8s"
	BackendKindOpenSandbox BackendKind = "opensandbox"
)

type ContainerExecutionMode string

const (
	ContainerExecutionHostRunner      ContainerExecutionMode = "host-runner"
	ContainerExecutionInContainerMode ContainerExecutionMode = "in-container-runner"
)

type CollectorMode string

const (
	CollectorModeRequire CollectorMode = "require"
	CollectorModeAuto    CollectorMode = "auto"
	CollectorModeSkip    CollectorMode = "skip"
)

type ArtifactBackend string

const (
	ArtifactBackendLocal ArtifactBackend = "local"
	ArtifactBackendS3    ArtifactBackend = "s3"
)

type ErrorCode string

const (
	ErrorCodeConfigInvalid        ErrorCode = "CONFIG_INVALID"
	ErrorCodePolicyDenied         ErrorCode = "POLICY_DENIED"
	ErrorCodeSetupFailed          ErrorCode = "SETUP_FAILED"
	ErrorCodeExecuteFailed        ErrorCode = "EXECUTE_FAILED"
	ErrorCodeVerifyFailed         ErrorCode = "VERIFY_FAILED"
	ErrorCodeCollectFailed        ErrorCode = "COLLECT_FAILED"
	ErrorCodeTimeout              ErrorCode = "TIMEOUT"
	ErrorCodeSignalAborted        ErrorCode = "SIGNAL_ABORTED"
	ErrorCodeCollectorUnavailable ErrorCode = "COLLECTOR_UNAVAILABLE"
	ErrorCodeArtifactUploadFailed ErrorCode = "ARTIFACT_UPLOAD_FAILED"

	ErrorCodePolicyFSDeny       ErrorCode = "POLICY_FS_DENY"
	ErrorCodePolicyNetDeny      ErrorCode = "POLICY_NET_DENY"
	ErrorCodePolicyToolDeny     ErrorCode = "POLICY_TOOL_DENY"
	ErrorCodePolicySecretDeny   ErrorCode = "POLICY_SECRET_DENY"
	ErrorCodePolicyResourceDeny ErrorCode = "POLICY_RESOURCE_DENY"

	ErrorCodeSandboxCreateFailed          ErrorCode = "sandbox.create_failed"
	ErrorCodeSandboxStartFailed           ErrorCode = "sandbox.start_failed"
	ErrorCodeSandboxExecFailed            ErrorCode = "sandbox.exec_failed"
	ErrorCodeSandboxStreamFailed          ErrorCode = "sandbox.stream_failed"
	ErrorCodeSandboxUploadFailed          ErrorCode = "sandbox.upload_failed"
	ErrorCodeSandboxDownloadFailed        ErrorCode = "sandbox.download_failed"
	ErrorCodeSandboxDeleteFailed          ErrorCode = "sandbox.delete_failed"
	ErrorCodeSandboxPauseFailed           ErrorCode = "sandbox.pause_failed"
	ErrorCodeSandboxResumeFailed          ErrorCode = "sandbox.resume_failed"
	ErrorCodeSandboxRenewFailed           ErrorCode = "sandbox.renew_failed"
	ErrorCodeSandboxUnsupportedCapability ErrorCode = "sandbox.unsupported_capability"
)

type RunConfig struct {
	Run         RunSection        `yaml:"run" json:"run"`
	Phases      PhasesConfig      `yaml:"phases" json:"phases"`
	Telemetry   TelemetryConfig   `yaml:"telemetry" json:"telemetry"`
	Collector   CollectorConfig   `yaml:"collector" json:"collector"`
	Artifacts   ArtifactsConfig   `yaml:"artifacts" json:"artifacts"`
	Platform    PlatformConfig    `yaml:"platform" json:"platform"`
	Backend     BackendConfig     `yaml:"backend" json:"backend"`
	OpenSandbox OpenSandboxConfig `yaml:"opensandbox" json:"opensandbox"`
	Sandbox     SandboxConfig     `yaml:"sandbox" json:"sandbox"`
	Provider    ProviderConfig    `yaml:"provider" json:"provider"`
	Go          GoConfig          `yaml:"go" json:"go"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type RunSection struct {
	ServiceName           string            `yaml:"service_name" json:"service_name"`
	Mode                  string            `yaml:"mode" json:"mode"`
	RunID                 string            `yaml:"run_id" json:"run_id"`
	Attempt               int               `yaml:"attempt" json:"attempt"`
	SandboxID             string            `yaml:"sandbox_id" json:"sandbox_id"`
	WorkspaceDir          string            `yaml:"workspace_dir" json:"workspace_dir"`
	ArtifactDir           string            `yaml:"artifact_dir" json:"artifact_dir"`
	OTLPEndpoint          string            `yaml:"otlp_endpoint" json:"otlp_endpoint"`
	DeploymentEnvironment string            `yaml:"deployment_environment" json:"deployment_environment"`
	Command               []string          `yaml:"command" json:"command"`
	Language              string            `yaml:"language" json:"language"`
	WorkspaceID           string            `yaml:"workspace_id,omitempty" json:"workspace_id,omitempty"`
	PatchID               string            `yaml:"patch_id,omitempty" json:"patch_id,omitempty"`
	TestCaseID            string            `yaml:"test_case_id,omitempty" json:"test_case_id,omitempty"`
	Image                 string            `yaml:"image,omitempty" json:"image,omitempty"`
	ImagePullPolicy       string            `yaml:"image_pull_policy,omitempty" json:"image_pull_policy,omitempty"`
	ExtraEnv              map[string]string `yaml:"extra_env,omitempty" json:"extra_env,omitempty"`
}

type PhasesConfig struct {
	Prepare PhaseConfig `yaml:"prepare" json:"prepare"`
	Setup   PhaseConfig `yaml:"setup" json:"setup"`
	Execute PhaseConfig `yaml:"execute" json:"execute"`
	Verify  PhaseConfig `yaml:"verify" json:"verify"`
	Collect PhaseConfig `yaml:"collect" json:"collect"`
}

type PhaseConfig struct {
	Enabled           bool     `yaml:"enabled" json:"enabled"`
	TimeoutSec        int      `yaml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	NetworkProfile    string   `yaml:"network_profile,omitempty" json:"network_profile,omitempty"`
	SmokeCommand      []string `yaml:"smoke_command,omitempty" json:"smoke_command,omitempty"`
	ExpectedArtifacts []string `yaml:"expected_artifacts,omitempty" json:"expected_artifacts,omitempty"`
}

type TelemetryConfig struct {
	Traces           bool `yaml:"traces" json:"traces"`
	Logs             bool `yaml:"logs" json:"logs"`
	Metrics          bool `yaml:"metrics" json:"metrics"`
	LogLineMaxBytes  int  `yaml:"log_line_max_bytes" json:"log_line_max_bytes"`
	EmitStdoutEvents bool `yaml:"emit_stdout_events" json:"emit_stdout_events"`
	EmitStderrEvents bool `yaml:"emit_stderr_events" json:"emit_stderr_events"`
}

type CollectorConfig struct {
	Mode                  CollectorMode `yaml:"mode" json:"mode"`
	HealthcheckTimeoutMs  int           `yaml:"healthcheck_timeout_ms" json:"healthcheck_timeout_ms"`
	LocalCollectorCommand []string      `yaml:"local_collector_command,omitempty" json:"local_collector_command,omitempty"`
	LocalCollectorConfig  string        `yaml:"local_collector_config,omitempty" json:"local_collector_config,omitempty"`
}

type ArtifactsConfig struct {
	Upload         bool            `yaml:"upload" json:"upload"`
	ObjectPrefix   string          `yaml:"object_prefix" json:"object_prefix"`
	Backend        ArtifactBackend `yaml:"backend" json:"backend"`
	Bucket         string          `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	Endpoint       string          `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Region         string          `yaml:"region,omitempty" json:"region,omitempty"`
	ForcePathStyle bool            `yaml:"force_path_style,omitempty" json:"force_path_style,omitempty"`
}

type PlatformConfig struct {
	TargetOS               string                 `yaml:"target_os" json:"target_os"`
	TargetArch             string                 `yaml:"target_arch" json:"target_arch"`
	RunMode                RunMode                `yaml:"run_mode" json:"run_mode"`
	ContainerExecutionMode ContainerExecutionMode `yaml:"container_execution_mode" json:"container_execution_mode"`
	FeatureGates           FeatureSet             `yaml:"feature_gates" json:"feature_gates"`
}

type BackendConfig struct {
	Kind BackendKind `yaml:"kind" json:"kind"`
}

type OpenSandboxRuntime string

const (
	OpenSandboxRuntimeDocker     OpenSandboxRuntime = "docker"
	OpenSandboxRuntimeKubernetes OpenSandboxRuntime = "kubernetes"
)

type OpenSandboxCleanupMode string

const (
	OpenSandboxCleanupDelete        OpenSandboxCleanupMode = "delete"
	OpenSandboxCleanupPause         OpenSandboxCleanupMode = "pause"
	OpenSandboxCleanupKeep          OpenSandboxCleanupMode = "keep"
	OpenSandboxCleanupPauseElseKeep OpenSandboxCleanupMode = "pause_if_supported_else_keep"
)

type OpenSandboxConfig struct {
	BaseURL          string                 `yaml:"base_url" json:"base_url"`
	APIKey           string                 `yaml:"api_key" json:"api_key"`
	Runtime          OpenSandboxRuntime     `yaml:"runtime" json:"runtime"`
	NetworkMode      string                 `yaml:"network_mode" json:"network_mode"`
	CreateTimeoutSec int                    `yaml:"create_timeout_sec" json:"create_timeout_sec"`
	PollIntervalMs   int                    `yaml:"poll_interval_ms" json:"poll_interval_ms"`
	CleanupMode      OpenSandboxCleanupMode `yaml:"cleanup_mode" json:"cleanup_mode"`
	TTLSec           int                    `yaml:"ttl_sec" json:"ttl_sec"`
	RenewOnLongRun   bool                   `yaml:"renew_on_long_run" json:"renew_on_long_run"`
	WorkspaceRoot    string                 `yaml:"workspace_root" json:"workspace_root"`
	UploadStrategy   string                 `yaml:"upload_strategy" json:"upload_strategy"`
	DownloadStrategy string                 `yaml:"download_strategy" json:"download_strategy"`
}

type SandboxConfig struct {
	Image      string            `yaml:"image" json:"image"`
	Entrypoint []string          `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	CPU        string            `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Memory     string            `yaml:"memory,omitempty" json:"memory,omitempty"`
}

type ProviderConfig struct {
	PreferOpenSandbox   bool     `yaml:"prefer_opensandbox" json:"prefer_opensandbox"`
	RequireCapabilities []string `yaml:"require_capabilities,omitempty" json:"require_capabilities,omitempty"`
	FallbackOrder       []string `yaml:"fallback_order,omitempty" json:"fallback_order,omitempty"`
}

type GoConfig struct {
	ClassifyCommands       bool              `yaml:"classify_commands" json:"classify_commands"`
	DetectGoEnv            bool              `yaml:"detect_go_env" json:"detect_go_env"`
	ManualSDKHelperEnabled bool              `yaml:"manual_sdk_helper_enabled" json:"manual_sdk_helper_enabled"`
	AutosdkBridgeEnabled   bool              `yaml:"autosdk_bridge_enabled" json:"autosdk_bridge_enabled"`
	OBIEnabled             bool              `yaml:"obi_enabled" json:"obi_enabled"`
	OBIConfigPath          string            `yaml:"obi_config_path,omitempty" json:"obi_config_path,omitempty"`
	ExtraEnv               map[string]string `yaml:"extra_env,omitempty" json:"extra_env,omitempty"`
}

type PolicyConfig struct {
	Filesystem      FilesystemPolicy          `yaml:"filesystem" json:"filesystem"`
	NetworkProfiles map[string]NetworkProfile `yaml:"network_profiles" json:"network_profiles"`
	Tools           ToolsPolicy               `yaml:"tools" json:"tools"`
	Secrets         SecretsPolicy             `yaml:"secrets" json:"secrets"`
	Resources       ResourcesPolicy           `yaml:"resources" json:"resources"`
}

type FilesystemPolicy struct {
	ReadAllow  []string `yaml:"read_allow" json:"read_allow"`
	WriteAllow []string `yaml:"write_allow" json:"write_allow"`
	Deny       []string `yaml:"deny" json:"deny"`
}

type NetworkProfile struct {
	AllowDomains []string `yaml:"allow_domains" json:"allow_domains"`
}

type ToolsPolicy struct {
	Allow        []string `yaml:"allow" json:"allow"`
	DenyPatterns []string `yaml:"deny_patterns" json:"deny_patterns"`
}

type SecretsPolicy struct {
	InjectEnv        []SecretBinding `yaml:"inject_env" json:"inject_env"`
	DenyExportToLogs bool            `yaml:"deny_export_to_logs" json:"deny_export_to_logs"`
}

type SecretBinding struct {
	Name       string  `yaml:"name" json:"name"`
	PhaseAllow []Phase `yaml:"phase_allow" json:"phase_allow"`
}

type ResourcesPolicy struct {
	TimeoutSecDefault int `yaml:"timeout_sec_default" json:"timeout_sec_default"`
	MaxMemoryMB       int `yaml:"max_memory_mb" json:"max_memory_mb"`
	MaxLogBytes       int `yaml:"max_log_bytes" json:"max_log_bytes"`
	MaxArtifactBytes  int `yaml:"max_artifact_bytes" json:"max_artifact_bytes"`
}

type FeatureSet struct {
	GoBasicRunner       bool `yaml:"go_basic_runner" json:"go_basic_runner"`
	GoManualSDK         bool `yaml:"go_manual_sdk" json:"go_manual_sdk"`
	GoAutoSDKBridge     bool `yaml:"go_autosdk_bridge" json:"go_autosdk_bridge"`
	OBIEBPF             bool `yaml:"obi_ebpf" json:"obi_ebpf"`
	K8sOperatorGoInject bool `yaml:"k8s_operator_go_inject" json:"k8s_operator_go_inject"`
	LocalDockerMode     bool `yaml:"local_docker_mode" json:"local_docker_mode"`
	LocalDirectMode     bool `yaml:"local_direct_mode" json:"local_direct_mode"`
	STGLinuxMode        bool `yaml:"stg_linux_mode" json:"stg_linux_mode"`
}

type ExecutionTarget struct {
	OS              string   `json:"os,omitempty"`
	Arch            string   `json:"arch,omitempty"`
	Mode            RunMode  `json:"mode,omitempty"`
	BackendKind     string   `json:"backend_kind,omitempty"`
	ProviderName    string   `json:"provider_name,omitempty"`
	RuntimeKind     string   `json:"runtime_kind,omitempty"`
	NetworkMode     string   `json:"network_mode,omitempty"`
	ContainerID     string   `json:"container_id,omitempty"`
	ContainerImage  string   `json:"container_image,omitempty"`
	ImageDigest     string   `json:"image_digest,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	InContainer     bool     `json:"in_container,omitempty"`
	InKubernetes    bool     `json:"in_kubernetes,omitempty"`
	DockerAvailable bool     `json:"docker_available,omitempty"`
}

type RunRequest struct {
	ConfigPath  string
	PolicyPath  string
	ArtifactDir string
	Command     []string
	RunConfig   RunConfig
	Policy      PolicyConfig
	Target      ExecutionTarget
	Version     VersionInfo
}

type VersionInfo struct {
	Version    string `json:"version"`
	GitSHA     string `json:"git_sha"`
	BuildTime  string `json:"build_time"`
	TargetOS   string `json:"target_os"`
	TargetArch string `json:"target_arch"`
}

type RunResult struct {
	RunID        string         `json:"run_id"`
	Attempt      int            `json:"attempt"`
	Status       RunStatus      `json:"status"`
	Phase        Phase          `json:"phase"`
	BackendKind  string         `json:"backend_kind,omitempty"`
	ProviderName string         `json:"provider_name,omitempty"`
	SandboxImage string         `json:"sandbox_image,omitempty"`
	CommandClass string         `json:"command_class"`
	ExitCode     int            `json:"exit_code"`
	Signal       string         `json:"signal"`
	TimedOut     bool           `json:"timed_out"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   time.Time      `json:"finished_at"`
	DurationMS   int64          `json:"duration_ms"`
	ErrorCode    ErrorCode      `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	PhaseResults []PhaseResult  `json:"phase_results,omitempty"`
	Artifacts    []ArtifactRef  `json:"artifacts,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type PhaseResult struct {
	Phase         Phase          `json:"phase"`
	Status        RunStatus      `json:"status"`
	BackendAction string         `json:"backend_action,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    time.Time      `json:"finished_at"`
	DurationMS    int64          `json:"duration_ms"`
	ErrorCode     ErrorCode      `json:"error_code,omitempty"`
	ErrorMessage  string         `json:"error_message,omitempty"`
	CommandClass  string         `json:"command_class,omitempty"`
	ExitCode      int            `json:"exit_code,omitempty"`
	TimedOut      bool           `json:"timed_out,omitempty"`
	Signal        string         `json:"signal,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type ArtifactRef struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	URI       string `json:"uri,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Skipped   bool   `json:"skipped,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type RunEvent struct {
	Name         string            `json:"name"`
	Phase        Phase             `json:"phase"`
	CommandClass string            `json:"command_class,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	At           time.Time         `json:"at"`
}

type StructuredLog struct {
	Timestamp      time.Time         `json:"ts"`
	RunID          string            `json:"run_id"`
	Attempt        int               `json:"attempt"`
	Phase          Phase             `json:"phase"`
	CommandClass   string            `json:"command_class"`
	CommandID      string            `json:"command_id,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	ExecProviderID string            `json:"exec_provider_id,omitempty"`
	Stream         string            `json:"stream"`
	LineNo         int               `json:"line_no"`
	Line           string            `json:"line"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

type MetricPoint struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Value      float64           `json:"value"`
	Attributes map[string]string `json:"attributes,omitempty"`
	At         time.Time         `json:"at"`
}

type SetupPlan struct {
	ProjectType string      `json:"project_type"`
	Runtime     string      `json:"runtime"`
	Lockfiles   []string    `json:"lockfiles"`
	Steps       []SetupStep `json:"steps"`
}

type SetupStep struct {
	ID   string   `json:"id"`
	Cmd  []string `json:"cmd"`
	Note string   `json:"note,omitempty"`
}

type EnvironmentFingerprint struct {
	OS             string            `json:"os"`
	Arch           string            `json:"arch"`
	Runtime        RuntimeInfo       `json:"runtime"`
	PackageManager RuntimeInfo       `json:"package_manager"`
	GitSHA         string            `json:"git_sha"`
	WorkspaceHash  string            `json:"workspace_hash"`
	LockfileHashes map[string]string `json:"lockfile_hashes"`
	GoEnv          map[string]string `json:"go_env,omitempty"`
}

type RuntimeInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type BackendCapabilities struct {
	SupportsPauseResume    bool `json:"supports_pause_resume"`
	SupportsTTL            bool `json:"supports_ttl"`
	SupportsFileUpload     bool `json:"supports_file_upload"`
	SupportsFileDownload   bool `json:"supports_file_download"`
	SupportsBackgroundExec bool `json:"supports_background_exec"`
	SupportsStreamLogs     bool `json:"supports_stream_logs"`
	SupportsEndpoints      bool `json:"supports_endpoints"`
	SupportsBridgeNetwork  bool `json:"supports_bridge_network"`
	SupportsHostNetwork    bool `json:"supports_host_network"`
	SupportsCodeInterp     bool `json:"supports_code_interp"`
}

type Endpoint struct {
	Name          string            `json:"name,omitempty"`
	ContainerPort int               `json:"container_port,omitempty"`
	URL           string            `json:"url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type BackendSnapshot struct {
	Kind         string              `json:"kind"`
	Provider     string              `json:"provider"`
	Runtime      string              `json:"runtime,omitempty"`
	ServerURL    string              `json:"server_url,omitempty"`
	Capabilities BackendCapabilities `json:"capabilities,omitempty"`
}

type SandboxSnapshot struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	NetworkMode string            `json:"network_mode,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ContextArtifact struct {
	RunID           string           `json:"run_id"`
	Attempt         int              `json:"attempt"`
	SandboxID       string           `json:"sandbox_id"`
	WorkspaceID     string           `json:"workspace_id,omitempty"`
	Mode            string           `json:"mode"`
	ServiceName     string           `json:"service_name"`
	GitSHA          string           `json:"git_sha,omitempty"`
	ImageDigest     string           `json:"image_digest,omitempty"`
	StartedAt       time.Time        `json:"started_at"`
	OriginalCommand []string         `json:"original_command"`
	OTLPEndpoint    string           `json:"otlp_endpoint"`
	Target          ExecutionTarget  `json:"target"`
	FeatureGates    FeatureSet       `json:"feature_gates"`
	Backend         *BackendSnapshot `json:"backend,omitempty"`
	Sandbox         *SandboxSnapshot `json:"sandbox,omitempty"`
}

type ReplayManifest struct {
	RunID                     string   `json:"run_id"`
	EnvironmentFingerprintRef string   `json:"environment_fingerprint_ref"`
	SetupPlanRef              string   `json:"setup_plan_ref"`
	CommandsRef               string   `json:"commands_ref"`
	ExpectedOutputs           []string `json:"expected_outputs"`
	Notes                     []string `json:"notes"`
}

type ProviderArtifact struct {
	BackendKind         string `json:"backend_kind"`
	ProviderName        string `json:"provider_name"`
	Runtime             string `json:"runtime,omitempty"`
	Server              string `json:"server,omitempty"`
	SupportsTTL         bool   `json:"supports_ttl,omitempty"`
	SupportsPauseResume bool   `json:"supports_pause_resume,omitempty"`
	SupportsFileUpload  bool   `json:"supports_file_upload,omitempty"`
	SupportsStreamLogs  bool   `json:"supports_stream_logs,omitempty"`
}

type SandboxArtifact struct {
	SandboxID   string            `json:"sandbox_id"`
	Status      string            `json:"status"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	NetworkMode string            `json:"network_mode,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type EndpointsArtifact struct {
	Ports []Endpoint `json:"ports"`
}

type RunnerError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	Phase        string `json:"phase,omitempty"`
	BackendKind  string `json:"backend_kind,omitempty"`
	ProviderCode string `json:"provider_code,omitempty"`
	Cause        error  `json:"-"`
}

func (e RunnerError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}
