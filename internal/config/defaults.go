package config

import "github.com/lotosli/sandbox-runner/internal/model"

func DefaultRunConfig() model.RunConfig {
	return model.RunConfig{
		Run: model.RunSection{
			ServiceName:           "sandbox-runner",
			Mode:                  "local",
			Attempt:               1,
			SandboxID:             "local-dev",
			WorkspaceDir:          ".",
			ArtifactDir:           ".sandbox-runner",
			OTLPEndpoint:          "http://127.0.0.1:4318",
			DeploymentEnvironment: "local",
			Language:              "auto",
			ImagePullPolicy:       "IfNotPresent",
			ExtraEnv:              map[string]string{},
		},
		Phases: model.PhasesConfig{
			Prepare: model.PhaseConfig{Enabled: true, TimeoutSec: 60},
			Setup:   model.PhaseConfig{Enabled: true, TimeoutSec: 900, NetworkProfile: "setup-online"},
			Execute: model.PhaseConfig{Enabled: true, TimeoutSec: 1800, NetworkProfile: "execute-offline"},
			Verify:  model.PhaseConfig{Enabled: true, TimeoutSec: 300},
			Collect: model.PhaseConfig{Enabled: true, TimeoutSec: 120},
		},
		Telemetry: model.TelemetryConfig{
			Traces:           true,
			Logs:             true,
			Metrics:          true,
			LogLineMaxBytes:  8192,
			EmitStdoutEvents: true,
			EmitStderrEvents: true,
		},
		Collector: model.CollectorConfig{
			Mode:                 model.CollectorModeAuto,
			HealthcheckTimeoutMs: 1000,
			LocalCollectorConfig: "configs/otelcol.local.yaml",
		},
		Artifacts: model.ArtifactsConfig{
			Upload:       false,
			ObjectPrefix: "sandbox-runner-runs/",
			Backend:      model.ArtifactBackendLocal,
		},
		Platform: model.PlatformConfig{
			TargetOS:               "auto",
			TargetArch:             "auto",
			RunMode:                model.RunModeLocalDirect,
			ContainerExecutionMode: model.ContainerExecutionHostRunner,
			FeatureGates: model.FeatureSet{
				GoBasicRunner:            true,
				GoManualSDK:              true,
				GoAutoSDKBridge:          false,
				OBIEBPF:                  false,
				K8sOperatorGoInject:      false,
				LocalDockerMode:          false,
				LocalDirectMode:          true,
				STGLinuxMode:             false,
				LocalAppleContainerMode:  false,
				LocalOrbStackMachineMode: false,
			},
		},
		Backend: model.BackendConfig{
			Kind: model.BackendKindDirect,
		},
		Runtime: model.RuntimeConfig{
			Profile: model.RuntimeProfileNative,
		},
		Kata: model.KataConfig{
			Enabled:                       false,
			RuntimeClassName:              "kata",
			ContainerdRuntime:             "io.containerd.kata.v2",
			RequireHardwareVirtualization: true,
			FailIfUnavailable:             false,
		},
		DevContainer: model.DevContainerConfig{
			CLIPath:                 "devcontainer",
			ConfigPath:              ".devcontainer/devcontainer.json",
			MountWorkspaceGitRoot:   true,
			RemoveExistingContainer: false,
			SkipPostCreate:          false,
			RunUserCommands:         true,
			UpTimeoutSec:            300,
			ExecTimeoutSec:          1800,
			IDLabelPrefix:           "ai-sandbox-runner",
			LogLevel:                "info",
			CleanupMode:             "delete",
		},
		Docker: model.DockerConfig{
			Provider:      model.DockerProviderDocker,
			Binary:        "docker",
			ComposeBinary: "docker",
			Context:       "default",
		},
		K8s: model.K8sConfig{
			Provider:  model.K8sProviderRemote,
			Namespace: "ai-sandbox-runner-runs",
		},
		AppleContainer: model.AppleContainerConfig{
			Enabled:          false,
			Binary:           "container",
			WorkspaceRoot:    "/workspace",
			CreateTimeoutSec: 120,
			ExecTimeoutSec:   1800,
			CleanupMode:      "delete",
			UploadStrategy:   "tar",
			DownloadStrategy: "tar",
		},
		OrbStack: model.OrbStackConfig{
			Enabled:               false,
			OrbBinary:             "orb",
			OrbCtlBinary:          "orbctl",
			DockerProviderEnabled: true,
			MachineEnabled:        false,
			MachineName:           "ai-runner-dev",
			MachineAutoCreate:     false,
			MachineDistro:         "ubuntu",
			MachineWorkspaceRoot:  "",
			MachineCleanupMode:    "keep",
			UploadStrategy:        "copy",
			DownloadStrategy:      "copy",
			K8sProviderEnabled:    false,
			DockerContext:         "orbstack",
			KubeContext:           "orbstack",
		},
		OpenSandbox: model.OpenSandboxConfig{
			BaseURL:          "http://127.0.0.1:8080",
			Runtime:          model.OpenSandboxRuntimeDocker,
			NetworkMode:      "bridge",
			CreateTimeoutSec: 120,
			PollIntervalMs:   1000,
			CleanupMode:      model.OpenSandboxCleanupDelete,
			TTLSec:           1800,
			RenewOnLongRun:   true,
			WorkspaceRoot:    "/workspace",
			UploadStrategy:   "tar",
			DownloadStrategy: "tar",
		},
		Sandbox: model.SandboxConfig{
			Entrypoint: []string{"/bin/sh", "-lc"},
			Env:        map[string]string{},
		},
		Provider: model.ProviderConfig{
			PreferOpenSandbox:   false,
			RequireCapabilities: []string{},
			FallbackOrder:       []string{"opensandbox", "docker", "direct"},
			MacLocalPreferred:   []string{"apple-container", "orbstack-machine", "docker", "devcontainer", "direct"},
		},
		Go: model.GoConfig{
			ClassifyCommands:       true,
			DetectGoEnv:            true,
			ManualSDKHelperEnabled: true,
			AutosdkBridgeEnabled:   false,
			OBIEnabled:             false,
			ExtraEnv:               map[string]string{},
		},
		Metadata: map[string]string{},
	}
}

func DefaultPolicyConfig() model.PolicyConfig {
	return model.PolicyConfig{
		Filesystem: model.FilesystemPolicy{
			ReadAllow:  []string{"/workspace", "/tmp"},
			WriteAllow: []string{"/workspace", "/tmp", "/workspace/.sandbox-runner"},
			Deny:       []string{"/root/.ssh", "/workspace/.env", "/workspace/secrets"},
		},
		NetworkProfiles: map[string]model.NetworkProfile{
			"setup-online": {
				AllowDomains: []string{
					"pypi.org",
					"files.pythonhosted.org",
					"registry.npmjs.org",
					"repo.maven.apache.org",
				},
			},
			"execute-offline": {AllowDomains: []string{}},
		},
		Tools: model.ToolsPolicy{
			Allow:        []string{"python", "python3", "pytest", "opentelemetry-instrument", "node", "npm", "java", "mvn", "bash", "sh", "go", "dlv"},
			DenyPatterns: []string{"rm -rf /", "curl * | sh", "wget * | sh"},
		},
		Secrets: model.SecretsPolicy{
			InjectEnv:        []model.SecretBinding{},
			DenyExportToLogs: true,
		},
		Resources: model.ResourcesPolicy{
			TimeoutSecDefault: 1800,
			MaxMemoryMB:       4096,
			MaxLogBytes:       104857600,
			MaxArtifactBytes:  536870912,
		},
	}
}
