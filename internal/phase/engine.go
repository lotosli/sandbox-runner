package phase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/adapter"
	"github.com/lotosli/sandbox-runner/internal/artifact"
	"github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/capability"
	"github.com/lotosli/sandbox-runner/internal/collector"
	"github.com/lotosli/sandbox-runner/internal/compat"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/envsync"
	"github.com/lotosli/sandbox-runner/internal/executor"
	"github.com/lotosli/sandbox-runner/internal/model"
	"github.com/lotosli/sandbox-runner/internal/platform"
	"github.com/lotosli/sandbox-runner/internal/policy"
	"github.com/lotosli/sandbox-runner/internal/proc"
	"github.com/lotosli/sandbox-runner/internal/telemetry"
)

type Engine struct {
	planner  envsync.Planner
	registry adapter.Registry
}

func NewEngine() Engine {
	return Engine{
		planner:  envsync.NewPlanner(),
		registry: adapter.NewRegistry(),
	}
}

type runState struct {
	req              *model.RunRequest
	writer           *artifact.Writer
	uploader         artifact.Uploader
	emitter          *telemetry.Emitter
	executor         executor.Executor
	backend          backend.SandboxBackend
	backendCaps      model.BackendCapabilities
	runtimeInfo      model.RuntimeInfo
	sandboxInfo      backend.SandboxInfo
	devcontainerInfo *model.DevContainerArtifact
	policy           policy.Engine
	collectorResult  collector.BootstrapResult
	phaseResults     []model.PhaseResult
	result           *model.RunResult
	setupPlan        model.SetupPlan
	environment      model.EnvironmentFingerprint
	commandClass     string
	target           model.ExecutionTarget
	resolution       model.ExecutionResolution
	runCtx           context.Context
}

func (e Engine) Run(ctx context.Context, req *model.RunRequest) (*model.RunResult, error) {
	state := &runState{
		req:      req,
		uploader: artifact.DefaultUploader{},
		result: &model.RunResult{
			StartedAt: time.Now().UTC(),
			Status:    model.StatusCreated,
			Metadata:  map[string]any{},
		},
	}

	prepareErr := e.runPrepare(ctx, state)
	if prepareErr != nil && state.writer == nil {
		return state.result, prepareErr
	}

	var mainErr error
	if prepareErr == nil {
		if err := e.runSetup(ctx, state); err != nil {
			mainErr = err
		} else if err := e.runExecute(ctx, state); err != nil {
			mainErr = err
		} else if err := e.runVerify(ctx, state); err != nil {
			mainErr = err
		}
	} else {
		mainErr = prepareErr
	}

	if err := e.runCollect(ctx, state); err != nil && mainErr == nil {
		mainErr = err
	}
	state.result.PhaseResults = state.phaseResults
	state.result.FinishedAt = time.Now().UTC()
	state.result.DurationMS = state.result.FinishedAt.Sub(state.result.StartedAt).Milliseconds()

	if state.writer != nil {
		_ = state.writer.Close()
	}
	if state.emitter != nil {
		_ = state.emitter.EndRun(state.baseCtx(ctx), state.result)
	}
	_ = state.collectorResult.Shutdown()
	return state.result, mainErr
}

func (e Engine) runPrepare(ctx context.Context, state *runState) error {
	req := state.req
	cfg := config.NormalizeRunConfig(req.RunConfig)
	if cfg.Run.RunID == "" {
		cfg.Run.RunID = generateRunID()
	}
	if cfg.Run.Attempt <= 0 {
		cfg.Run.Attempt = 1
	}
	workspace, err := filepath.Abs(cfg.Run.WorkspaceDir)
	if err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeConfigInvalid, err)
	}
	artifactDir, err := filepath.Abs(cfg.Run.ArtifactDir)
	if err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeConfigInvalid, err)
	}
	cfg.Run.WorkspaceDir = workspace
	cfg.Run.ArtifactDir = artifactDir
	compatibilityResult := compat.ValidateCompatibility(cfg.Execution)
	if compatibilityResult.Level == model.SupportUnsupported {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeUnsupportedExecutionCombo, model.RunnerError{
			Code:        string(model.ErrorCodeUnsupportedExecutionCombo),
			Message:     compatibilityResult.Message,
			BackendKind: string(cfg.Execution.Backend),
		})
	}
	capabilityResult, err := capability.Probe(ctx, cfg.Execution, cfg)
	if err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeCapabilityProbeFailed, err)
	}
	state.resolution = model.ExecutionResolution{
		Config:        cfg.Execution,
		Compatibility: compatibilityResult,
		Capability:    capabilityResult,
	}
	if cfg.Metadata == nil {
		cfg.Metadata = map[string]string{}
	}
	cfg.Metadata["execution.backend"] = string(cfg.Execution.Backend)
	cfg.Metadata["execution.provider"] = string(cfg.Execution.Provider)
	cfg.Metadata["execution.runtime_profile"] = string(cfg.Execution.RuntimeProfile)
	cfg.Metadata["execution.compatibility_level"] = string(compatibilityResult.Level)
	req.RunConfig = cfg

	state.target = platform.Detect(cfg.Platform.RunMode)
	state.target.BackendKind = string(cfg.Execution.Backend)
	state.target.ProviderName = string(cfg.Execution.Provider)
	state.target.BackendProvider = string(cfg.Execution.Provider)
	state.target.RuntimeProfile = string(cfg.Execution.RuntimeProfile)
	state.target.RuntimeClassName = cfg.Kata.RuntimeClassName
	state.target.Virtualization = virtualizationForRuntime(cfg.Runtime.Profile)
	state.target.RuntimeKind = runtimeKindForConfig(cfg)
	state.target.LocalPlatform = localPlatformForConfig(cfg)
	state.target.ContainerImage = sandboxImage(cfg)
	state.target.Execution = cfg.Execution
	state.target.CompatibilityLevel = compatibilityResult.Level
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		state.target.NetworkMode = cfg.OpenSandbox.NetworkMode
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		state.target.MachineName = cfg.OrbStack.MachineName
	}
	features, warnings, err := platform.ResolveFeatures(cfg, state.target)
	if err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeConfigInvalid, err)
	}
	cfg.Platform.FeatureGates = features
	req.RunConfig = cfg

	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeConfigInvalid, err)
	}
	writer, err := artifact.NewWriter(artifactDir, int64(req.Policy.Resources.MaxArtifactBytes))
	if err != nil {
		return e.failWithoutArtifacts(state, model.PhasePrepare, model.ErrorCodeConfigInvalid, err)
	}
	state.writer = writer
	state.policy = policy.NewEngine(req.Policy, cfg)

	if err := state.policy.CheckPathRead(ctx, model.PhasePrepare, workspace); err != nil {
		return e.failPhase(ctx, state, model.PhasePrepare, time.Now().UTC(), model.ErrorCodePolicyFSDeny, err)
	}
	if err := state.policy.CheckPathWrite(ctx, model.PhasePrepare, artifactDir); err != nil {
		return e.failPhase(ctx, state, model.PhasePrepare, time.Now().UTC(), model.ErrorCodePolicyFSDeny, err)
	}

	bootstrapResult, err := collector.Bootstrap(ctx, cfg)
	if err != nil {
		return e.failPhase(ctx, state, model.PhasePrepare, time.Now().UTC(), model.ErrorCodeCollectorUnavailable, err)
	}
	state.collectorResult = bootstrapResult
	state.result.Metadata["collector_warnings"] = bootstrapResult.Warnings

	emitter, err := telemetry.New(ctx, cfg, bootstrapResult.Enabled)
	if err != nil {
		return e.failPhase(ctx, state, model.PhasePrepare, time.Now().UTC(), model.ErrorCodeCollectorUnavailable, err)
	}
	state.emitter = emitter
	runCtx, err := emitter.StartRun(ctx, req)
	if err != nil {
		return e.failPhase(ctx, state, model.PhasePrepare, time.Now().UTC(), model.ErrorCodeCollectorUnavailable, err)
	}
	state.runCtx = runCtx
	phaseCtx, started := e.startPhase(ctx, state, model.PhasePrepare)

	backendImpl, err := backend.New(state.resolution, cfg)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	state.backend = backendImpl

	caps, err := backendImpl.Capabilities(phaseCtx)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUnsupportedCapability, err)
	}
	state.backendCaps = caps
	state.result.BackendKind = string(cfg.Execution.Backend)
	state.result.ProviderName = string(cfg.Execution.Provider)
	state.result.BackendProvider = string(cfg.Execution.Provider)
	state.result.SandboxImage = sandboxImage(cfg)
	runtimeInfo, err := backendImpl.RuntimeInfo(phaseCtx)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUnsupportedCapability, err)
	}
	state.runtimeInfo = runtimeInfo
	state.result.RuntimeProfile = string(cfg.Execution.RuntimeProfile)
	state.result.RuntimeClassName = runtimeInfo.RuntimeClassName
	state.result.MachineName = firstNonEmpty(runtimeInfo.MachineName, cfg.OrbStack.MachineName)
	state.target.RuntimeClassName = runtimeInfo.RuntimeClassName
	if runtimeInfo.Virtualization != "" {
		state.target.Virtualization = runtimeInfo.Virtualization
	}
	state.target.LocalPlatform = firstNonEmpty(runtimeInfo.LocalPlatform, state.target.LocalPlatform)
	state.target.MachineName = firstNonEmpty(runtimeInfo.MachineName, state.target.MachineName)
	state.target.ContainerID = firstNonEmpty(runtimeInfo.ContainerID, state.target.ContainerID)
	state.result.Metadata["execution"] = state.resolution.Config
	state.result.Metadata["compatibility"] = state.resolution.Compatibility
	state.result.Metadata["capability_probe"] = state.resolution.Capability
	state.result.Metadata["backend"] = backendSnapshot(cfg, caps)
	state.result.Metadata["runtime"] = runtimeArtifact(cfg, runtimeInfo)
	_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
		Name:  "runtime.profile.selected",
		Phase: model.PhasePrepare,
		At:    time.Now().UTC(),
		Attributes: map[string]string{
			"execution.backend":             string(cfg.Execution.Backend),
			"execution.provider":            string(cfg.Execution.Provider),
			"execution.runtime_profile":     string(cfg.Execution.RuntimeProfile),
			"execution.compatibility_level": string(compatibilityResult.Level),
			"sandbox.runtime.profile":       runtimeInfo.RuntimeProfile,
			"sandbox.runtime.class":         runtimeInfo.RuntimeClassName,
			"sandbox.virtualization":        runtimeInfo.Virtualization,
		},
	})
	if cfg.Runtime.Profile == model.RuntimeProfileKata {
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "kata.preflight.start", Phase: model.PhasePrepare, At: time.Now().UTC()})
		if err := e.ensureRuntimeProfileSupport(cfg, caps); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeProviderRuntimeUnsupported, err)
		}
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  "kata.preflight.end",
			Phase: model.PhasePrepare,
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"checked_by": runtimeInfo.CheckedBy,
				"detail":     runtimeInfo.Detail,
			},
		})
	}

	if err := e.ensureRequiredCapabilities(cfg, caps); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUnsupportedCapability, err)
	}

	if requiresManagedSandbox(cfg) {
		startEvent, endEvent := "sandbox.create.start", "sandbox.create.end"
		if cfg.Backend.Kind == model.BackendKindDevContainer {
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "devcontainer.read_configuration.start", Phase: model.PhasePrepare, At: time.Now().UTC()})
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "devcontainer.up.start", Phase: model.PhasePrepare, At: time.Now().UTC()})
			startEvent, endEvent = "devcontainer.up.start", "devcontainer.up.end"
		} else if cfg.Backend.Kind == model.BackendKindAppleContainer {
			startEvent, endEvent = "container.create.start", "container.create.end"
		} else if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
			startEvent, endEvent = "machine.create.start", "machine.create.end"
		}
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  startEvent,
			Phase: model.PhasePrepare,
			At:    time.Now().UTC(),
		})
		info, err := backendImpl.Create(phaseCtx, e.buildCreateSandboxRequest(cfg))
		if err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxCreateFailed, err)
		}
		state.sandboxInfo = info
		cfg.Run.SandboxID = info.ID
		req.RunConfig = cfg
		state.target.ContainerID = info.ID
		if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
			state.target.ContainerID = ""
			state.target.MachineName = info.ID
			state.result.MachineName = info.ID
		}

		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  endEvent,
			Phase: model.PhasePrepare,
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"sandbox.id": info.ID,
			},
		})
		if cfg.Backend.Kind == model.BackendKindDevContainer {
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
				Name:       "devcontainer.read_configuration.end",
				Phase:      model.PhasePrepare,
				At:         time.Now().UTC(),
				Attributes: map[string]string{"sandbox.id": info.ID},
			})
		}
		if err := backendImpl.Start(phaseCtx, info.ID); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxStartFailed, err)
		}
		startedEvent := "sandbox.start"
		if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
			startedEvent = "machine.start"
		}
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  startedEvent,
			Phase: model.PhasePrepare,
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"sandbox.id": info.ID,
			},
		})
		if syncer, ok := backendImpl.(backend.WorkspaceSyncer); ok {
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "workspace.sync_in", Phase: model.PhasePrepare, At: time.Now().UTC()})
			if err := syncer.SyncWorkspaceIn(phaseCtx, info.ID, workspace); err != nil {
				return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUploadFailed, err)
			}
		}
		if provider, ok := backendImpl.(backend.DevContainerMetadataProvider); ok {
			devInfo, err := provider.DevContainerMetadata(phaseCtx, info.ID)
			if err == nil {
				state.devcontainerInfo = &devInfo
				_ = state.writer.WriteDevContainer(devInfo)
				if cfg.Backend.Kind == model.BackendKindDevContainer && devInfo.WorkspaceFolder != "" {
					_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
						Name:  "devcontainer.run_user_commands.end",
						Phase: model.PhasePrepare,
						At:    time.Now().UTC(),
						Attributes: map[string]string{
							"devcontainer.workspace_folder": devInfo.WorkspaceFolder,
						},
					})
				}
			} else {
				state.result.Metadata["devcontainer_metadata_error"] = err.Error()
			}
		}
	}

	contextArtifact := model.ContextArtifact{
		RunID:           cfg.Run.RunID,
		Attempt:         cfg.Run.Attempt,
		SandboxID:       cfg.Run.SandboxID,
		WorkspaceID:     cfg.Run.WorkspaceID,
		Mode:            cfg.Run.Mode,
		ServiceName:     cfg.Run.ServiceName,
		StartedAt:       state.result.StartedAt,
		OriginalCommand: cfg.Run.Command,
		OTLPEndpoint:    cfg.Run.OTLPEndpoint,
		Execution:       state.resolution.Config,
		Compatibility:   state.resolution.Compatibility,
		CapabilityProbe: state.resolution.Capability,
		Target:          state.target,
		FeatureGates:    features,
		Backend:         backendSnapshot(cfg, caps),
		Sandbox:         sandboxSnapshot(state.sandboxInfo, cfg),
	}
	if err := state.writer.WriteContext(contextArtifact); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if err := state.writer.WriteProvider(providerArtifact(cfg, caps, state.devcontainerInfo)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if err := state.writer.WriteBackendProfile(backendProfileArtifact(cfg, runtimeInfo)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if err := state.writer.WriteRuntime(runtimeArtifact(cfg, runtimeInfo)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if snapshot := sandboxArtifact(state.sandboxInfo, cfg); snapshot.SandboxID != "" {
		if err := state.writer.WriteSandbox(snapshot); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
		}
	}
	if containerInfo := containerArtifact(state.sandboxInfo, cfg); containerInfo.ContainerID != "" {
		if err := state.writer.WriteContainer(containerInfo); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
		}
	}
	if machineInfo := machineArtifact(state.sandboxInfo, cfg); machineInfo.MachineName != "" {
		if err := state.writer.WriteMachine(machineInfo); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
		}
	}

	state.result.RunID = cfg.Run.RunID
	state.result.Attempt = cfg.Run.Attempt
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhasePrepare
	state.result.ExitCode = 0
	prepareResult := successPhase(model.PhasePrepare, started, map[string]any{"warnings": warnings})
	prepareResult.BackendAction = prepareActionForBackend(cfg)
	state.phaseResults = append(state.phaseResults, prepareResult)
	e.endPhase(state, phaseCtx, model.PhasePrepare, state.phaseResults[len(state.phaseResults)-1])
	return nil
}

func (e Engine) runSetup(ctx context.Context, state *runState) error {
	if !state.req.RunConfig.Phases.Setup.Enabled {
		return nil
	}
	phaseCtx, started := e.startPhase(ctx, state, model.PhaseSetup)
	if _, err := state.policy.ResolveNetworkProfile(phaseCtx, model.PhaseSetup); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodePolicyNetDeny, err)
	}

	plan, fingerprint, err := e.planner.Plan(phaseCtx, state.req.RunConfig.Run.WorkspaceDir)
	state.setupPlan = plan
	state.environment = fingerprint
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodeSetupFailed, err)
	}
	if err := state.writer.WriteSetupPlan(plan); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodeSetupFailed, err)
	}
	if err := state.writer.WriteEnvironment(fingerprint); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodeSetupFailed, err)
	}

	execImpl, err := executor.New(state.req.RunConfig, state.target, state.backend)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodeSetupFailed, err)
	}
	state.executor = execImpl

	for _, step := range plan.Steps {
		if err := state.policy.CheckCommand(phaseCtx, model.PhaseSetup, step.Cmd); err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodePolicyToolDeny, err)
		}
		stepEnv, err := state.policy.ResolveSecrets(phaseCtx, model.PhaseSetup)
		if err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodePolicySecretDeny, err)
		}
		result, execErr := execImpl.Run(phaseCtx, executor.Spec{
			Phase:           model.PhaseSetup,
			Command:         step.Cmd,
			Env:             stepEnv,
			Dir:             state.req.RunConfig.Run.WorkspaceDir,
			Timeout:         time.Duration(state.policy.TimeoutForPhase(model.PhaseSetup)) * time.Second,
			RunID:           state.req.RunConfig.Run.RunID,
			Attempt:         state.req.RunConfig.Run.Attempt,
			CommandClass:    proc.ClassifyCommand(step.Cmd),
			ArtifactDir:     state.req.RunConfig.Run.ArtifactDir,
			LogLineMaxBytes: state.req.RunConfig.Telemetry.LogLineMaxBytes,
			RunConfig:       state.req.RunConfig,
			Target:          state.target,
		}, newLogHandler(phaseCtx, state))
		e.recordCommand(phaseCtx, state, model.PhaseSetup, step.Cmd, result)
		if execErr != nil || result.ExitCode != 0 {
			return e.failPhaseWithStatus(phaseCtx, state, model.PhaseSetup, started, model.ErrorCodeSetupFailed, commandError(step.Cmd, result, execErr), model.StatusFailed, result)
		}
	}

	setupResult := successPhase(model.PhaseSetup, started, map[string]any{"project_type": plan.ProjectType})
	setupResult.BackendAction = execActionForBackend(state.req.RunConfig)
	state.phaseResults = append(state.phaseResults, setupResult)
	e.endPhase(state, phaseCtx, model.PhaseSetup, state.phaseResults[len(state.phaseResults)-1])
	return nil
}

func (e Engine) runExecute(ctx context.Context, state *runState) error {
	if !state.req.RunConfig.Phases.Execute.Enabled {
		return nil
	}
	phaseCtx, started := e.startPhase(ctx, state, model.PhaseExecute)
	cfg := state.req.RunConfig
	if len(cfg.Run.Command) == 0 {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodeConfigInvalid, errors.New("no command provided"))
	}
	if _, err := state.policy.ResolveNetworkProfile(phaseCtx, model.PhaseExecute); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodePolicyNetDeny, err)
	}

	env := map[string]string{}
	for k, v := range cfg.Run.ExtraEnv {
		env[k] = v
	}
	for k, v := range cfg.Go.ExtraEnv {
		env[k] = v
	}
	secrets, err := state.policy.ResolveSecrets(phaseCtx, model.PhaseExecute)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodePolicySecretDeny, err)
	}
	for k, v := range secrets {
		env[k] = v
	}

	adapterImpl, err := e.registry.Resolve(cfg.Run.Language, cfg.Run.Command, env)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodeExecuteFailed, err)
	}
	command, env, err := adapterImpl.Rewrite(cfg.Run.Command, env, cfg)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodeExecuteFailed, err)
	}
	if err := state.policy.CheckCommand(phaseCtx, model.PhaseExecute, command); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodePolicyToolDeny, err)
	}
	if state.executor == nil {
		state.executor, err = executor.New(cfg, state.target, state.backend)
		if err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseExecute, started, model.ErrorCodeExecuteFailed, err)
		}
	}

	state.commandClass = proc.ClassifyCommand(command)
	result, execErr := state.executor.Run(phaseCtx, executor.Spec{
		Phase:           model.PhaseExecute,
		Command:         command,
		Env:             env,
		Dir:             cfg.Run.WorkspaceDir,
		Timeout:         time.Duration(state.policy.TimeoutForPhase(model.PhaseExecute)) * time.Second,
		RunID:           cfg.Run.RunID,
		Attempt:         cfg.Run.Attempt,
		CommandClass:    state.commandClass,
		ArtifactDir:     cfg.Run.ArtifactDir,
		LogLineMaxBytes: cfg.Telemetry.LogLineMaxBytes,
		RunConfig:       cfg,
		Target:          state.target,
	}, newLogHandler(phaseCtx, state))
	e.recordCommand(phaseCtx, state, model.PhaseExecute, command, result)
	state.result.CommandClass = state.commandClass
	state.result.ExitCode = result.ExitCode
	state.result.Metadata["execution_target"] = result.Target

	if execErr != nil || result.ExitCode != 0 {
		code := model.ErrorCodeExecuteFailed
		status := model.StatusFailed
		if errors.Is(execErr, context.DeadlineExceeded) || result.TimedOut {
			code = model.ErrorCodeTimeout
			status = model.StatusTimedOut
		}
		return e.failPhaseWithStatus(phaseCtx, state, model.PhaseExecute, started, code, commandError(command, result, execErr), status, result)
	}

	executeResult := successPhaseWithExec(model.PhaseExecute, started, state.commandClass, result)
	executeResult.BackendAction = execActionForBackend(cfg)
	state.phaseResults = append(state.phaseResults, executeResult)
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhaseExecute
	e.endPhase(state, phaseCtx, model.PhaseExecute, state.phaseResults[len(state.phaseResults)-1])
	return nil
}

func (e Engine) runVerify(ctx context.Context, state *runState) error {
	if !state.req.RunConfig.Phases.Verify.Enabled {
		return nil
	}
	phaseCtx, started := e.startPhase(ctx, state, model.PhaseVerify)
	cfg := state.req.RunConfig
	if _, err := state.policy.ResolveNetworkProfile(phaseCtx, model.PhaseVerify); err != nil && cfg.Phases.Verify.NetworkProfile != "" {
		return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodePolicyNetDeny, err)
	}

	if len(cfg.Phases.Verify.SmokeCommand) > 0 {
		if err := state.policy.CheckCommand(phaseCtx, model.PhaseVerify, cfg.Phases.Verify.SmokeCommand); err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodePolicyToolDeny, err)
		}
		if state.executor == nil {
			execImpl, err := executor.New(cfg, state.target, state.backend)
			if err != nil {
				return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodeVerifyFailed, err)
			}
			state.executor = execImpl
		}
		verifyEnv, err := state.policy.ResolveSecrets(phaseCtx, model.PhaseVerify)
		if err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodePolicySecretDeny, err)
		}
		result, execErr := state.executor.Run(phaseCtx, executor.Spec{
			Phase:           model.PhaseVerify,
			Command:         cfg.Phases.Verify.SmokeCommand,
			Env:             verifyEnv,
			Dir:             cfg.Run.WorkspaceDir,
			Timeout:         time.Duration(state.policy.TimeoutForPhase(model.PhaseVerify)) * time.Second,
			RunID:           cfg.Run.RunID,
			Attempt:         cfg.Run.Attempt,
			CommandClass:    proc.ClassifyCommand(cfg.Phases.Verify.SmokeCommand),
			ArtifactDir:     cfg.Run.ArtifactDir,
			LogLineMaxBytes: cfg.Telemetry.LogLineMaxBytes,
			RunConfig:       cfg,
			Target:          state.target,
		}, newLogHandler(phaseCtx, state))
		e.recordCommand(phaseCtx, state, model.PhaseVerify, cfg.Phases.Verify.SmokeCommand, result)
		if execErr != nil || result.ExitCode != 0 {
			return e.failPhaseWithStatus(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodeVerifyFailed, commandError(cfg.Phases.Verify.SmokeCommand, result, execErr), model.StatusFailed, result)
		}
	}

	if err := e.downloadExpectedArtifacts(phaseCtx, state); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodeSandboxDownloadFailed, err)
	}

	for _, expected := range cfg.Phases.Verify.ExpectedArtifacts {
		path := filepath.Join(cfg.Run.ArtifactDir, expected)
		if _, err := os.Stat(path); err != nil {
			return e.failPhase(phaseCtx, state, model.PhaseVerify, started, model.ErrorCodeVerifyFailed, fmt.Errorf("expected artifact missing: %s", expected))
		}
	}

	verifyResult := successPhase(model.PhaseVerify, started, nil)
	if len(cfg.Phases.Verify.SmokeCommand) > 0 {
		verifyResult.BackendAction = execActionForBackend(cfg)
	}
	state.phaseResults = append(state.phaseResults, verifyResult)
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhaseVerify
	e.endPhase(state, phaseCtx, model.PhaseVerify, state.phaseResults[len(state.phaseResults)-1])
	return nil
}

func (e Engine) runCollect(ctx context.Context, state *runState) error {
	if state.writer == nil {
		return nil
	}
	phaseCtx, started := e.startPhase(ctx, state, model.PhaseCollect)
	cfg := state.req.RunConfig

	if requiresManagedSandbox(cfg) {
		if syncer, ok := state.backend.(backend.WorkspaceSyncer); ok && state.sandboxInfo.ID != "" {
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "workspace.sync_out", Phase: model.PhaseCollect, At: time.Now().UTC()})
			remoteDir := collectRemoteArtifactDir(cfg)
			localDir := filepath.Join(cfg.Run.ArtifactDir, artifact.ArtifactsDirName, string(cfg.Backend.Kind)+"-workspace")
			if err := syncer.SyncWorkspaceOut(phaseCtx, state.sandboxInfo.ID, remoteDir, localDir); err != nil {
				state.result.Metadata["workspace_sync_out_error"] = err.Error()
			}
		}
		if provider, ok := state.backend.(backend.MetadataProvider); ok && state.sandboxInfo.ID != "" {
			info, err := provider.SandboxMetadata(phaseCtx, state.sandboxInfo.ID)
			if err == nil {
				state.sandboxInfo = info
				_ = state.writer.WriteSandbox(sandboxArtifact(info, cfg))
				if containerInfo := containerArtifact(info, cfg); containerInfo.ContainerID != "" {
					_ = state.writer.WriteContainer(containerInfo)
				}
				if machineInfo := machineArtifact(info, cfg); machineInfo.MachineName != "" {
					_ = state.writer.WriteMachine(machineInfo)
				}
			} else {
				state.result.Metadata["sandbox_metadata_error"] = err.Error()
			}
			if cfg.Backend.Kind == model.BackendKindOpenSandbox {
				endpoints, err := provider.Endpoints(phaseCtx, state.sandboxInfo.ID, []int{44772, 8080})
				if err == nil {
					_ = state.writer.WriteEndpoints(model.EndpointsArtifact{Ports: endpoints})
				} else {
					state.result.Metadata["sandbox_endpoints_error"] = err.Error()
				}
			}
		}
		if provider, ok := state.backend.(backend.DevContainerMetadataProvider); ok && state.sandboxInfo.ID != "" {
			devInfo, err := provider.DevContainerMetadata(phaseCtx, state.sandboxInfo.ID)
			if err == nil {
				state.devcontainerInfo = &devInfo
				_ = state.writer.WriteDevContainer(devInfo)
			} else {
				state.result.Metadata["devcontainer_metadata_error"] = err.Error()
			}
		}
		if err := e.cleanupBackend(phaseCtx, state); err != nil {
			state.result.Metadata["sandbox_cleanup_error"] = err.Error()
			if state.result.Status == model.StatusSucceeded {
				state.result.Status = model.StatusPartial
			}
		}
	}

	replay := model.ReplayManifest{
		RunID:                     state.req.RunConfig.Run.RunID,
		EnvironmentFingerprintRef: artifact.EnvironmentFileName,
		SetupPlanRef:              artifact.SetupPlanFileName,
		CommandsRef:               artifact.CommandsFileName,
		ExpectedOutputs:           state.req.RunConfig.Phases.Verify.ExpectedArtifacts,
		Notes: []string{
			fmt.Sprintf("project_type=%s", state.setupPlan.ProjectType),
			fmt.Sprintf("run_mode=%s", state.req.RunConfig.Platform.RunMode),
		},
	}
	if len(state.req.RunConfig.Phases.Verify.SmokeCommand) > 0 {
		replay.Notes = append(replay.Notes, fmt.Sprintf("verify phase runs %v", state.req.RunConfig.Phases.Verify.SmokeCommand))
	}
	if err := state.writer.WriteReplay(replay); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}

	collectResult := successPhase(model.PhaseCollect, started, map[string]any{"artifacts": 0})
	collectResult.BackendAction = cleanupActionForBackend(cfg)
	state.phaseResults = append(state.phaseResults, collectResult)
	state.result.Phase = model.PhaseCollect
	if state.result.Status == model.StatusCreated {
		state.result.Status = model.StatusSucceeded
	}
	state.result.PhaseResults = state.phaseResults

	if err := state.writer.WritePhases(state.phaseResults); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	if err := state.writer.WriteResults(state.result); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	if err := state.writer.WriteIndex(buildArtifactIndex(state, nil)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}

	refs, err := state.writer.ArtifactRefs()
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	uploadRefs := makeUploadRefs(state.writer.Root(), refs)
	uploadedRefs, uploadErr := state.uploader.Upload(phaseCtx, uploadRefs, state.req.RunConfig.Artifacts, state.req.RunConfig.Run.DeploymentEnvironment, state.req.RunConfig.Run.RunID, state.req.RunConfig.Run.Attempt)
	finalRefs := mergeUploadedRefs(state.writer.Root(), refs, uploadedRefs)
	state.result.Artifacts = finalRefs
	collectResult.Metadata["artifacts"] = len(finalRefs)
	state.phaseResults[len(state.phaseResults)-1] = collectResult
	if uploadErr != nil {
		state.result.Metadata["artifact_upload_error"] = uploadErr.Error()
		if state.result.Status == model.StatusSucceeded {
			state.result.Status = model.StatusPartial
			state.result.ErrorCode = model.ErrorCodeArtifactUploadFailed
			state.result.ErrorMessage = uploadErr.Error()
		}
	}

	if err := state.writer.WriteResults(state.result); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	if err := state.writer.WriteIndex(buildArtifactIndex(state, finalRefs)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	if uploadErr == nil && state.req.RunConfig.Artifacts.Upload {
		if _, err := state.uploader.Upload(phaseCtx, makeUploadRefs(state.writer.Root(), filterArtifactRefs(finalRefs, artifact.ResultsFileName, artifact.IndexFileName)), state.req.RunConfig.Artifacts, state.req.RunConfig.Run.DeploymentEnvironment, state.req.RunConfig.Run.RunID, state.req.RunConfig.Run.Attempt); err != nil {
			state.result.Metadata["artifact_summary_upload_error"] = err.Error()
			if state.result.Status == model.StatusSucceeded {
				state.result.Status = model.StatusPartial
				state.result.ErrorCode = model.ErrorCodeArtifactUploadFailed
				state.result.ErrorMessage = err.Error()
			}
			_ = state.writer.WriteResults(state.result)
			_ = state.writer.WriteIndex(buildArtifactIndex(state, finalRefs))
			e.endPhase(state, phaseCtx, model.PhaseCollect, state.phaseResults[len(state.phaseResults)-1])
			return err
		}
	}
	e.endPhase(state, phaseCtx, model.PhaseCollect, state.phaseResults[len(state.phaseResults)-1])
	return uploadErr
}

func (e Engine) startPhase(ctx context.Context, state *runState, phase model.Phase) (context.Context, time.Time) {
	started := time.Now().UTC()
	base := state.baseCtx(ctx)
	if state.emitter != nil {
		if phaseCtx, err := state.emitter.StartPhase(base, phase); err == nil {
			return phaseCtx, started
		}
	}
	return base, started
}

func (s *runState) baseCtx(fallback context.Context) context.Context {
	if s.runCtx != nil {
		return s.runCtx
	}
	return fallback
}

func (e Engine) endPhase(state *runState, ctx context.Context, phase model.Phase, result model.PhaseResult) {
	if state.emitter != nil {
		_ = state.emitter.EndPhase(ctx, phase, result)
	}
}

func (e Engine) failWithoutArtifacts(state *runState, phase model.Phase, code model.ErrorCode, err error) error {
	code = errorCodeForErr(code, err)
	state.result.Status = model.StatusFailed
	state.result.Phase = phase
	state.result.ErrorCode = code
	if err != nil {
		state.result.ErrorMessage = err.Error()
	}
	return err
}

func (e Engine) failPhase(ctx context.Context, state *runState, phase model.Phase, started time.Time, code model.ErrorCode, err error) error {
	return e.failPhaseWithStatus(ctx, state, phase, started, code, err, model.StatusFailed, executor.Result{})
}

func (e Engine) failPhaseWithStatus(ctx context.Context, state *runState, phase model.Phase, started time.Time, code model.ErrorCode, err error, status model.RunStatus, execResult executor.Result) error {
	code = errorCodeForErr(code, err)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	phaseResult := model.PhaseResult{
		Phase:        phase,
		Status:       status,
		StartedAt:    started,
		FinishedAt:   time.Now().UTC(),
		DurationMS:   time.Since(started).Milliseconds(),
		ErrorCode:    code,
		ErrorMessage: msg,
		ExitCode:     execResult.ExitCode,
		TimedOut:     execResult.TimedOut,
		Signal:       execResult.Signal,
		CommandClass: state.commandClass,
	}
	state.phaseResults = append(state.phaseResults, phaseResult)
	state.result.Status = status
	state.result.Phase = phase
	state.result.ErrorCode = code
	state.result.ErrorMessage = msg
	state.result.ExitCode = execResult.ExitCode
	state.result.TimedOut = execResult.TimedOut
	state.result.Signal = execResult.Signal
	if state.emitter != nil {
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{
			Name:  "phase.error",
			Phase: phase,
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"error_code": string(code),
			},
		})
		_ = state.emitter.EndPhase(ctx, phase, phaseResult)
	}
	return err
}

func successPhase(phase model.Phase, started time.Time, metadata map[string]any) model.PhaseResult {
	return model.PhaseResult{
		Phase:      phase,
		Status:     model.StatusSucceeded,
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
		DurationMS: time.Since(started).Milliseconds(),
		Metadata:   metadata,
	}
}

func successPhaseWithExec(phase model.Phase, started time.Time, commandClass string, result executor.Result) model.PhaseResult {
	return model.PhaseResult{
		Phase:        phase,
		Status:       model.StatusSucceeded,
		StartedAt:    started,
		FinishedAt:   time.Now().UTC(),
		DurationMS:   time.Since(started).Milliseconds(),
		CommandClass: commandClass,
		ExitCode:     result.ExitCode,
		Signal:       result.Signal,
		TimedOut:     result.TimedOut,
		Metadata: map[string]any{
			"stdout_lines": result.StdoutLines,
			"stderr_lines": result.StderrLines,
			"target":       result.Target,
		},
	}
}

func (e Engine) recordCommand(ctx context.Context, state *runState, phase model.Phase, command []string, result executor.Result) {
	record := map[string]any{
		"ts":                  time.Now().UTC(),
		"phase":               phase,
		"command":             command,
		"command_class":       proc.ClassifyCommand(command),
		"exit_code":           result.ExitCode,
		"timed_out":           result.TimedOut,
		"signal":              result.Signal,
		"duration_ms":         result.Duration.Milliseconds(),
		"target":              result.Target,
		"execution":           state.resolution.Config,
		"compatibility_level": state.resolution.Compatibility.Level,
	}
	for key, value := range result.Metadata {
		record[key] = value
	}
	if state.writer != nil {
		_ = state.writer.AppendCommand(record)
	}
	if state.emitter != nil {
		_ = state.emitter.EmitMetric(ctx, model.MetricPoint{
			Name:  "sandbox_command_duration_ms",
			Kind:  "histogram",
			Value: float64(result.Duration.Milliseconds()),
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"phase":                   string(phase),
				"command_class":           proc.ClassifyCommand(command),
				"sandbox.runtime.profile": state.runtimeInfo.RuntimeProfile,
			},
		})
	}
}

type logHandler struct {
	ctx   context.Context
	state *runState
}

func newLogHandler(ctx context.Context, state *runState) logHandler {
	return logHandler{ctx: ctx, state: state}
}

func (h logHandler) OnLog(ctx context.Context, log model.StructuredLog) error {
	_ = ctx
	if log.Attributes == nil {
		log.Attributes = map[string]string{}
	}
	log.Attributes["execution.backend"] = string(h.state.resolution.Config.Backend)
	log.Attributes["execution.provider"] = string(h.state.resolution.Config.Provider)
	log.Attributes["execution.runtime_profile"] = string(h.state.resolution.Config.RuntimeProfile)
	log.Attributes["execution.compatibility_level"] = string(h.state.resolution.Compatibility.Level)
	if h.state.emitter != nil {
		_ = h.state.emitter.EmitLog(h.ctx, log)
	}
	switch log.Stream {
	case "stdout":
		return h.state.writer.AppendStdout(log)
	case "stderr":
		return h.state.writer.AppendStderr(log)
	default:
		return nil
	}
}

func (e Engine) buildCreateSandboxRequest(cfg model.RunConfig) backend.CreateSandboxRequest {
	image := cfg.Sandbox.Image
	if image == "" {
		image = cfg.Run.Image
	}
	env := map[string]string{}
	for key, value := range cfg.Sandbox.Env {
		env[key] = value
	}
	for key, value := range cfg.Run.ExtraEnv {
		if _, exists := env[key]; !exists {
			env[key] = value
		}
	}
	workspaceDir := cfg.Run.WorkspaceDir
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		workspaceDir = cfg.OpenSandbox.WorkspaceRoot
	} else if cfg.Backend.Kind == model.BackendKindAppleContainer {
		workspaceDir = cfg.Run.WorkspaceDir
	} else if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		workspaceDir = cfg.OrbStack.MachineWorkspaceRoot
	}
	return backend.CreateSandboxRequest{
		RunID:       cfg.Run.RunID,
		Attempt:     cfg.Run.Attempt,
		WorkspaceID: cfg.Run.WorkspaceID,
		Image:       image,
		Entrypoint:  cfg.Sandbox.Entrypoint,
		Env:         env,
		Metadata: map[string]string{
			"run_id":                        cfg.Run.RunID,
			"attempt":                       fmt.Sprintf("%d", cfg.Run.Attempt),
			"execution.backend":             string(cfg.Execution.Backend),
			"execution.provider":            string(cfg.Execution.Provider),
			"execution.runtime_profile":     string(cfg.Execution.RuntimeProfile),
			"execution.compatibility_level": cfg.Metadata["execution.compatibility_level"],
			"runtime.profile":               string(cfg.Runtime.Profile),
			"runtime.class":                 cfg.Kata.RuntimeClassName,
			"backend.kind":                  string(cfg.Backend.Kind),
			"backend.provider":              backendProviderForConfig(cfg),
			"local.platform":                localPlatformForConfig(cfg),
		},
		CPU:          cfg.Sandbox.CPU,
		Memory:       cfg.Sandbox.Memory,
		NetworkMode:  cfg.OpenSandbox.NetworkMode,
		TimeoutSec:   maxInt(cfg.OpenSandbox.TTLSec, 1800),
		WorkspaceDir: workspaceDir,
	}
}

func (e Engine) ensureRuntimeProfileSupport(cfg model.RunConfig, caps model.BackendCapabilities) error {
	if cfg.Runtime.Profile == model.RuntimeProfileKata && !caps.SupportsRuntimeProfile {
		return model.RunnerError{
			Code:        string(model.ErrorCodeProviderRuntimeUnsupported),
			Message:     fmt.Sprintf("backend %s does not support runtime profile %s", cfg.Backend.Kind, cfg.Runtime.Profile),
			BackendKind: string(cfg.Backend.Kind),
		}
	}
	return nil
}

func (e Engine) ensureRequiredCapabilities(cfg model.RunConfig, caps model.BackendCapabilities) error {
	for _, capability := range cfg.Provider.RequireCapabilities {
		if !hasCapability(capability, caps) {
			return fmt.Errorf("backend %s does not support required capability %s", cfg.Backend.Kind, capability)
		}
	}
	return nil
}

func hasCapability(name string, caps model.BackendCapabilities) bool {
	switch name {
	case "pause_resume":
		return caps.SupportsPauseResume
	case "ttl":
		return caps.SupportsTTL
	case "file_upload":
		return caps.SupportsFileUpload
	case "file_download":
		return caps.SupportsFileDownload
	case "background_exec":
		return caps.SupportsBackgroundExec
	case "stream_logs":
		return caps.SupportsStreamLogs
	case "endpoints":
		return caps.SupportsEndpoints
	case "bridge_network":
		return caps.SupportsBridgeNetwork
	case "host_network":
		return caps.SupportsHostNetwork
	case "code_interp":
		return caps.SupportsCodeInterp
	case "runtime_profile":
		return caps.SupportsRuntimeProfile
	case "devcontainer":
		return caps.SupportsDevContainer
	case "machine_exec":
		return caps.SupportsMachineExec
	case "oci_image":
		return caps.SupportsOCIImage
	case "vm_isolation":
		return caps.SupportsVMIsolation
	case "k8s_target":
		return caps.SupportsK8sTarget
	default:
		return false
	}
}

func backendSnapshot(cfg model.RunConfig, caps model.BackendCapabilities) *model.BackendSnapshot {
	snapshot := &model.BackendSnapshot{
		Kind:             string(cfg.Execution.Backend),
		Provider:         providerNameForConfig(cfg),
		BackendProvider:  backendProviderForConfig(cfg),
		RuntimeProfile:   string(cfg.Execution.RuntimeProfile),
		RuntimeClassName: cfg.Kata.RuntimeClassName,
		Virtualization:   virtualizationForRuntime(cfg.Runtime.Profile),
		LocalPlatform:    localPlatformForConfig(cfg),
		Capabilities:     caps,
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		snapshot.Runtime = string(cfg.OpenSandbox.Runtime)
		snapshot.ServerURL = cfg.OpenSandbox.BaseURL
	}
	if cfg.Backend.Kind == model.BackendKindDevContainer {
		snapshot.Runtime = "devcontainer"
	}
	if cfg.Backend.Kind == model.BackendKindAppleContainer {
		snapshot.Runtime = "apple-container"
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		snapshot.Runtime = "orbstack-machine"
	}
	return snapshot
}

func sandboxSnapshot(info backend.SandboxInfo, cfg model.RunConfig) *model.SandboxSnapshot {
	if info.ID == "" && cfg.Run.SandboxID == "" {
		return nil
	}
	id := info.ID
	if id == "" {
		id = cfg.Run.SandboxID
	}
	return &model.SandboxSnapshot{
		ID:               id,
		Status:           info.Status,
		NetworkMode:      cfg.OpenSandbox.NetworkMode,
		RuntimeProfile:   string(cfg.Execution.RuntimeProfile),
		RuntimeClassName: cfg.Kata.RuntimeClassName,
		Virtualization:   virtualizationForRuntime(cfg.Runtime.Profile),
		MachineName:      machineNameForSandbox(cfg, info),
		ExpiresAt:        info.ExpiresAt,
		Metadata:         info.Metadata,
	}
}

func providerArtifact(cfg model.RunConfig, caps model.BackendCapabilities, devInfo *model.DevContainerArtifact) model.ProviderArtifact {
	artifact := model.ProviderArtifact{
		BackendKind:         string(cfg.Execution.Backend),
		ProviderName:        providerNameForConfig(cfg),
		BackendProvider:     backendProviderForConfig(cfg),
		RuntimeProfile:      string(cfg.Execution.RuntimeProfile),
		RuntimeClassName:    cfg.Kata.RuntimeClassName,
		LocalPlatform:       localPlatformForConfig(cfg),
		SupportsTTL:         caps.SupportsTTL,
		SupportsPauseResume: caps.SupportsPauseResume,
		SupportsFileUpload:  caps.SupportsFileUpload,
		SupportsStreamLogs:  caps.SupportsStreamLogs,
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		artifact.ProviderName = "opensandbox"
		artifact.Runtime = string(cfg.OpenSandbox.Runtime)
		artifact.Server = cfg.OpenSandbox.BaseURL
	}
	if cfg.Backend.Kind == model.BackendKindDevContainer && devInfo != nil {
		copied := *devInfo
		artifact.DevContainer = &copied
	}
	return artifact
}

func sandboxArtifact(info backend.SandboxInfo, cfg model.RunConfig) model.SandboxArtifact {
	return model.SandboxArtifact{
		SandboxID:        info.ID,
		Status:           info.Status,
		ExpiresAt:        info.ExpiresAt,
		NetworkMode:      cfg.OpenSandbox.NetworkMode,
		RuntimeProfile:   string(cfg.Execution.RuntimeProfile),
		RuntimeClassName: cfg.Kata.RuntimeClassName,
		Virtualization:   virtualizationForRuntime(cfg.Runtime.Profile),
		MachineName:      machineNameForSandbox(cfg, info),
		Metadata:         info.Metadata,
	}
}

func (e Engine) downloadExpectedArtifacts(ctx context.Context, state *runState) error {
	cfg := state.req.RunConfig
	if cfg.Backend.Kind != model.BackendKindOpenSandbox || len(cfg.Phases.Verify.ExpectedArtifacts) == 0 {
		return nil
	}
	downloader, ok := state.backend.(backend.ArtifactDownloader)
	if !ok || state.sandboxInfo.ID == "" {
		return nil
	}
	for _, expected := range cfg.Phases.Verify.ExpectedArtifacts {
		remotePath := path.Join(cfg.OpenSandbox.WorkspaceRoot, filepath.Base(cfg.Run.ArtifactDir), filepath.ToSlash(expected))
		localPath := filepath.Join(cfg.Run.ArtifactDir, expected)
		if err := downloader.DownloadArtifact(ctx, state.sandboxInfo.ID, remotePath, localPath); err != nil {
			return err
		}
	}
	return nil
}

func (e Engine) cleanupBackend(ctx context.Context, state *runState) error {
	if state.backend == nil || state.sandboxInfo.ID == "" || !requiresManagedSandbox(state.req.RunConfig) {
		return nil
	}
	if state.req.RunConfig.Backend.Kind == model.BackendKindDevContainer {
		if strings.EqualFold(state.req.RunConfig.DevContainer.CleanupMode, "keep") {
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "devcontainer.keep", Phase: model.PhaseCollect, At: time.Now().UTC()})
			return nil
		}
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "devcontainer.down", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return state.backend.Delete(ctx, state.sandboxInfo.ID)
	}
	if state.req.RunConfig.Backend.Kind == model.BackendKindAppleContainer {
		if strings.EqualFold(state.req.RunConfig.AppleContainer.CleanupMode, "keep") {
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "container.keep", Phase: model.PhaseCollect, At: time.Now().UTC()})
			return nil
		}
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "container.delete", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return state.backend.Delete(ctx, state.sandboxInfo.ID)
	}
	if state.req.RunConfig.Backend.Kind == model.BackendKindOrbStackMachine {
		switch strings.ToLower(state.req.RunConfig.OrbStack.MachineCleanupMode) {
		case "stop":
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "machine.stop", Phase: model.PhaseCollect, At: time.Now().UTC()})
		case "delete":
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "machine.delete", Phase: model.PhaseCollect, At: time.Now().UTC()})
		default:
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "machine.keep", Phase: model.PhaseCollect, At: time.Now().UTC()})
			return nil
		}
		return state.backend.Delete(ctx, state.sandboxInfo.ID)
	}
	switch state.req.RunConfig.OpenSandbox.CleanupMode {
	case model.OpenSandboxCleanupKeep:
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "sandbox.cleanup.keep", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return nil
	case model.OpenSandboxCleanupPause:
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "sandbox.pause", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return state.backend.Pause(ctx, state.sandboxInfo.ID)
	case model.OpenSandboxCleanupPauseElseKeep:
		if state.backendCaps.SupportsPauseResume {
			_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "sandbox.cleanup.pause", Phase: model.PhaseCollect, At: time.Now().UTC()})
			return state.backend.Pause(ctx, state.sandboxInfo.ID)
		}
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "sandbox.cleanup.keep", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return nil
	default:
		_ = state.emitter.EmitEvent(ctx, model.RunEvent{Name: "sandbox.cleanup.delete", Phase: model.PhaseCollect, At: time.Now().UTC()})
		return state.backend.Delete(ctx, state.sandboxInfo.ID)
	}
}

func runtimeArtifact(cfg model.RunConfig, runtimeInfo model.RuntimeInfo) model.RuntimeArtifact {
	containerID := runtimeInfo.ContainerID
	if cfg.Backend.Kind != model.BackendKindOrbStackMachine {
		containerID = firstNonEmpty(containerID, cfg.Run.SandboxID)
	}
	return model.RuntimeArtifact{
		BackendKind:      string(cfg.Execution.Backend),
		ProviderName:     providerNameForConfig(cfg),
		BackendProvider:  firstNonEmpty(runtimeInfo.BackendProvider, backendProviderForConfig(cfg)),
		RuntimeProfile:   firstNonEmpty(string(cfg.Execution.RuntimeProfile), runtimeInfo.RuntimeProfile),
		RuntimeClassName: runtimeInfo.RuntimeClassName,
		ContainerRuntime: runtimeInfo.ContainerRuntime,
		Virtualization:   runtimeInfo.Virtualization,
		HostOS:           runtimeInfo.HostOS,
		HostArch:         runtimeInfo.HostArch,
		LocalPlatform:    firstNonEmpty(runtimeInfo.LocalPlatform, localPlatformForConfig(cfg)),
		MachineName:      firstNonEmpty(runtimeInfo.MachineName, cfg.OrbStack.MachineName),
		ContainerID:      containerID,
		Available:        runtimeInfo.Available,
		CheckedBy:        runtimeInfo.CheckedBy,
		Detail:           runtimeInfo.Detail,
	}
}

func providerNameForConfig(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	return backendProviderForConfig(cfg)
}

func backendProviderForConfig(cfg model.RunConfig) string {
	if cfg.Execution.Provider != "" {
		return string(cfg.Execution.Provider)
	}
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "native"
	case model.BackendKindK8s:
		if cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
			return "orbstack"
		}
		return "native"
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	case model.BackendKindOpenSandbox:
		return "opensandbox"
	case model.BackendKindDevContainer, model.BackendKindAppleContainer:
		return "native"
	default:
		return string(cfg.Backend.Kind)
	}
}

func localPlatformForConfig(cfg model.RunConfig) string {
	switch {
	case cfg.Execution.Backend == model.ExecutionBackendAppleContainer:
		return "macos"
	case cfg.Execution.Backend == model.ExecutionBackendMachine:
		return "orbstack"
	case cfg.Execution.Provider == model.ProviderOrbStack:
		return "orbstack"
	default:
		return ""
	}
}

func runtimeKindForConfig(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindOpenSandbox:
		return string(cfg.OpenSandbox.Runtime)
	case model.BackendKindDevContainer:
		return "devcontainer"
	case model.BackendKindAppleContainer:
		return "apple-container"
	case model.BackendKindOrbStackMachine:
		return "orbstack-machine"
	case model.BackendKindDocker:
		return "docker"
	case model.BackendKindK8s:
		return "kubernetes"
	default:
		return string(cfg.Backend.Kind)
	}
}

func sandboxImage(cfg model.RunConfig) string {
	if cfg.Sandbox.Image != "" {
		return cfg.Sandbox.Image
	}
	return cfg.Run.Image
}

func machineNameForSandbox(cfg model.RunConfig, info backend.SandboxInfo) string {
	if cfg.Backend.Kind != model.BackendKindOrbStackMachine {
		return ""
	}
	if info.Metadata["machine_name"] != "" {
		return info.Metadata["machine_name"]
	}
	if info.ID != "" {
		return info.ID
	}
	return cfg.OrbStack.MachineName
}

func backendProfileArtifact(cfg model.RunConfig, runtimeInfo model.RuntimeInfo) model.BackendProfileArtifact {
	return model.BackendProfileArtifact{
		BackendKind:     string(cfg.Execution.Backend),
		BackendProvider: firstNonEmpty(runtimeInfo.BackendProvider, backendProviderForConfig(cfg)),
		RuntimeProfile:  string(cfg.Execution.RuntimeProfile),
		LocalPlatform:   firstNonEmpty(runtimeInfo.LocalPlatform, localPlatformForConfig(cfg)),
	}
}

func machineArtifact(info backend.SandboxInfo, cfg model.RunConfig) model.MachineArtifact {
	if cfg.Backend.Kind != model.BackendKindOrbStackMachine {
		return model.MachineArtifact{}
	}
	return model.MachineArtifact{
		MachineName: machineNameForSandbox(cfg, info),
		Status:      info.Status,
		Distro:      info.Metadata["distro"],
	}
}

func containerArtifact(info backend.SandboxInfo, cfg model.RunConfig) model.ContainerArtifact {
	if cfg.Backend.Kind != model.BackendKindAppleContainer {
		return model.ContainerArtifact{}
	}
	image := info.Metadata["image"]
	if image == "" {
		image = sandboxImage(cfg)
	}
	return model.ContainerArtifact{
		ContainerID: firstNonEmpty(info.ID, cfg.Run.SandboxID),
		Image:       image,
		Status:      info.Status,
	}
}

func collectRemoteArtifactDir(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindOpenSandbox:
		return path.Join(cfg.OpenSandbox.WorkspaceRoot, filepath.Base(cfg.Run.ArtifactDir))
	case model.BackendKindAppleContainer:
		return path.Join(cfg.AppleContainer.WorkspaceRoot, filepath.Base(cfg.Run.ArtifactDir))
	case model.BackendKindOrbStackMachine:
		return path.Join(cfg.OrbStack.MachineWorkspaceRoot, filepath.Base(cfg.Run.ArtifactDir))
	default:
		return ""
	}
}

func prepareActionForBackend(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindAppleContainer:
		return "container.create"
	case model.BackendKindOrbStackMachine:
		return "machine.create"
	case model.BackendKindDevContainer:
		return "devcontainer.up"
	case model.BackendKindOpenSandbox:
		return "sandbox.create"
	default:
		return ""
	}
}

func execActionForBackend(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindAppleContainer:
		return "container.exec"
	case model.BackendKindOrbStackMachine:
		return "machine.exec"
	case model.BackendKindDevContainer, model.BackendKindOpenSandbox:
		return "sandbox.exec"
	default:
		return ""
	}
}

func cleanupActionForBackend(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindAppleContainer:
		if strings.EqualFold(cfg.AppleContainer.CleanupMode, "keep") {
			return "container.keep"
		}
		return "container.delete"
	case model.BackendKindOrbStackMachine:
		switch strings.ToLower(cfg.OrbStack.MachineCleanupMode) {
		case "stop":
			return "machine.stop"
		case "delete":
			return "machine.delete"
		default:
			return "machine.keep"
		}
	case model.BackendKindDevContainer:
		if strings.EqualFold(cfg.DevContainer.CleanupMode, "keep") {
			return "devcontainer.keep"
		}
		return "devcontainer.down"
	case model.BackendKindOpenSandbox:
		switch cfg.OpenSandbox.CleanupMode {
		case model.OpenSandboxCleanupKeep:
			return "sandbox.keep"
		case model.OpenSandboxCleanupPause, model.OpenSandboxCleanupPauseElseKeep:
			return "sandbox.pause"
		default:
			return "sandbox.delete"
		}
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func requiresManagedSandbox(cfg model.RunConfig) bool {
	switch cfg.Backend.Kind {
	case model.BackendKindOpenSandbox, model.BackendKindDevContainer, model.BackendKindAppleContainer, model.BackendKindOrbStackMachine:
		return true
	default:
		return false
	}
}

func virtualizationForRuntime(profile model.RuntimeProfile) string {
	switch profile {
	case model.RuntimeProfileKata:
		return "kata"
	case model.RuntimeProfileGVisor:
		return "gvisor"
	case model.RuntimeProfileFirecracker:
		return "firecracker"
	case model.RuntimeProfileAppleContainer:
		return "apple-container"
	case model.RuntimeProfileOrbStackMachine:
		return "vm"
	default:
		return "none"
	}
}

func errorCodeForErr(defaultCode model.ErrorCode, err error) model.ErrorCode {
	if err == nil {
		return defaultCode
	}
	var runnerErr model.RunnerError
	if errors.As(err, &runnerErr) && runnerErr.Code != "" {
		return model.ErrorCode(runnerErr.Code)
	}
	return defaultCode
}

func maxInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func commandError(command []string, result executor.Result, err error) error {
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%v exited with code %d", command, result.ExitCode)
	}
	return nil
}

func generateRunID() string {
	return fmt.Sprintf("r-%s", time.Now().UTC().Format("20060102-150405"))
}

func ExitCodeForResult(result *model.RunResult, err error) int {
	if result != nil {
		if result.ExitCode != 0 {
			return result.ExitCode
		}
		switch result.Status {
		case model.StatusTimedOut:
			return 124
		case model.StatusAborted:
			return 130
		case model.StatusFailed, model.StatusPartial:
			return 1
		default:
			return 0
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	return 1
}

func makeUploadRefs(root string, refs []model.ArtifactRef) []model.ArtifactRef {
	out := make([]model.ArtifactRef, 0, len(refs))
	for _, ref := range refs {
		clone := ref
		clone.Path = filepath.Join(root, ref.Path)
		out = append(out, clone)
	}
	return out
}

func mergeUploadedRefs(root string, refs []model.ArtifactRef, uploaded []model.ArtifactRef) []model.ArtifactRef {
	out := make([]model.ArtifactRef, len(refs))
	copy(out, refs)
	uriByAbsPath := map[string]string{}
	for _, ref := range uploaded {
		if ref.URI == "" {
			continue
		}
		uriByAbsPath[filepath.Clean(ref.Path)] = ref.URI
	}
	for i := range out {
		absPath := filepath.Clean(filepath.Join(root, out[i].Path))
		if uri, ok := uriByAbsPath[absPath]; ok {
			out[i].URI = uri
		}
	}
	return out
}

func filterArtifactRefs(refs []model.ArtifactRef, paths ...string) []model.ArtifactRef {
	allow := map[string]struct{}{}
	for _, path := range paths {
		allow[path] = struct{}{}
	}
	out := make([]model.ArtifactRef, 0, len(paths))
	for _, ref := range refs {
		if _, ok := allow[ref.Path]; ok {
			out = append(out, ref)
		}
	}
	return out
}

func buildArtifactIndex(state *runState, refs []model.ArtifactRef) model.ArtifactIndex {
	files := map[string]string{}
	for _, ref := range refs {
		switch ref.Path {
		case artifact.IndexFileName:
			files["index"] = ref.Path
		case artifact.ResultsFileName:
			files["results"] = ref.Path
		case artifact.PhasesFileName:
			files["phases"] = ref.Path
		case artifact.ReplayFileName:
			files["replay"] = ref.Path
		case artifact.CommandsFileName:
			files["commands"] = ref.Path
		case artifact.StdoutFileName:
			files["stdout"] = ref.Path
		case artifact.StderrFileName:
			files["stderr"] = ref.Path
		case artifact.ContextFileName:
			files["context"] = ref.Path
		case artifact.EnvironmentFileName:
			files["environment"] = ref.Path
		case artifact.SetupPlanFileName:
			files["setup_plan"] = ref.Path
		case artifact.ProviderFileName:
			files["provider"] = ref.Path
		case artifact.RuntimeFileName:
			files["runtime"] = ref.Path
		case artifact.BackendProfileFileName:
			files["backend_profile"] = ref.Path
		case artifact.SandboxFileName:
			files["sandbox"] = ref.Path
		case artifact.DevContainerFileName:
			files["devcontainer"] = ref.Path
		case artifact.EndpointsFileName:
			files["endpoints"] = ref.Path
		case artifact.MachineFileName:
			files["machine"] = ref.Path
		case artifact.ContainerFileName:
			files["container"] = ref.Path
		}
	}
	if len(files) == 0 {
		files["index"] = artifact.IndexFileName
		files["results"] = artifact.ResultsFileName
		files["phases"] = artifact.PhasesFileName
		files["replay"] = artifact.ReplayFileName
		files["commands"] = artifact.CommandsFileName
		files["stdout"] = artifact.StdoutFileName
		files["stderr"] = artifact.StderrFileName
		files["context"] = artifact.ContextFileName
	}
	return model.ArtifactIndex{
		SchemaVersion:      1,
		RunID:              state.result.RunID,
		Attempt:            state.result.Attempt,
		Status:             state.result.Status,
		Phase:              state.result.Phase,
		ExitCode:           state.result.ExitCode,
		CommandClass:       state.result.CommandClass,
		Execution:          state.resolution.Config,
		CompatibilityLevel: state.resolution.Compatibility.Level,
		SuggestedReadOrder: artifactIndexReadOrder(files),
		Files:              files,
		Artifacts:          refs,
	}
}

func artifactIndexReadOrder(files map[string]string) []string {
	order := []string{"index", "results", "phases", "commands", "stdout", "stderr", "replay", "context", "environment", "setup_plan", "provider", "runtime", "backend_profile", "sandbox", "devcontainer", "endpoints", "machine", "container"}
	out := make([]string, 0, len(order))
	for _, key := range order {
		if files[key] != "" {
			out = append(out, key)
		}
	}
	return out
}
