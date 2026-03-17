package phase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/lotosli/sandbox-runner/internal/adapter"
	"github.com/lotosli/sandbox-runner/internal/artifact"
	"github.com/lotosli/sandbox-runner/internal/backend"
	"github.com/lotosli/sandbox-runner/internal/collector"
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
	req             *model.RunRequest
	writer          *artifact.Writer
	uploader        artifact.Uploader
	emitter         *telemetry.Emitter
	executor        executor.Executor
	backend         backend.SandboxBackend
	backendCaps     model.BackendCapabilities
	sandboxInfo     backend.SandboxInfo
	policy          policy.Engine
	collectorResult collector.BootstrapResult
	phaseResults    []model.PhaseResult
	result          *model.RunResult
	setupPlan       model.SetupPlan
	environment     model.EnvironmentFingerprint
	commandClass    string
	target          model.ExecutionTarget
	runCtx          context.Context
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
	cfg := req.RunConfig
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
	req.RunConfig = cfg

	state.target = platform.Detect(cfg.Platform.RunMode)
	state.target.BackendKind = string(cfg.Backend.Kind)
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		state.target.ProviderName = "opensandbox"
		state.target.RuntimeKind = string(cfg.OpenSandbox.Runtime)
		state.target.NetworkMode = cfg.OpenSandbox.NetworkMode
		state.target.ContainerImage = cfg.Sandbox.Image
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

	backendImpl, err := backend.New(cfg)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	state.backend = backendImpl

	caps, err := backendImpl.Capabilities(phaseCtx)
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUnsupportedCapability, err)
	}
	state.backendCaps = caps
	state.result.BackendKind = string(cfg.Backend.Kind)
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		state.result.ProviderName = "opensandbox"
		state.result.SandboxImage = cfg.Sandbox.Image
	}
	state.result.Metadata["backend"] = backendSnapshot(cfg, caps)

	if err := e.ensureRequiredCapabilities(cfg, caps); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxUnsupportedCapability, err)
	}

	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  "sandbox.create.start",
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

		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  "sandbox.create.end",
			Phase: model.PhasePrepare,
			At:    time.Now().UTC(),
			Attributes: map[string]string{
				"sandbox.id": info.ID,
			},
		})
		if err := backendImpl.Start(phaseCtx, info.ID); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeSandboxStartFailed, err)
		}
		_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{
			Name:  "sandbox.start",
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
		Target:          state.target,
		FeatureGates:    features,
		Backend:         backendSnapshot(cfg, caps),
		Sandbox:         sandboxSnapshot(state.sandboxInfo, cfg),
	}
	if err := state.writer.WriteContext(contextArtifact); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if err := state.writer.WriteProvider(providerArtifact(cfg, caps)); err != nil {
		return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
	}
	if snapshot := sandboxArtifact(state.sandboxInfo, cfg); snapshot.SandboxID != "" {
		if err := state.writer.WriteSandbox(snapshot); err != nil {
			return e.failPhase(phaseCtx, state, model.PhasePrepare, started, model.ErrorCodeConfigInvalid, err)
		}
	}

	state.result.RunID = cfg.Run.RunID
	state.result.Attempt = cfg.Run.Attempt
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhasePrepare
	state.result.ExitCode = 0
	state.phaseResults = append(state.phaseResults, successPhase(model.PhasePrepare, started, map[string]any{"warnings": warnings}))
	_ = state.emitter.EndPhase(phaseCtx, model.PhasePrepare, state.phaseResults[len(state.phaseResults)-1])
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

	state.phaseResults = append(state.phaseResults, successPhase(model.PhaseSetup, started, map[string]any{"project_type": plan.ProjectType}))
	_ = state.emitter.EndPhase(phaseCtx, model.PhaseSetup, state.phaseResults[len(state.phaseResults)-1])
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

	state.phaseResults = append(state.phaseResults, successPhaseWithExec(model.PhaseExecute, started, state.commandClass, result))
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhaseExecute
	_ = state.emitter.EndPhase(phaseCtx, model.PhaseExecute, state.phaseResults[len(state.phaseResults)-1])
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

	state.phaseResults = append(state.phaseResults, successPhase(model.PhaseVerify, started, nil))
	state.result.Status = model.StatusSucceeded
	state.result.Phase = model.PhaseVerify
	_ = state.emitter.EndPhase(phaseCtx, model.PhaseVerify, state.phaseResults[len(state.phaseResults)-1])
	return nil
}

func (e Engine) runCollect(ctx context.Context, state *runState) error {
	if state.writer == nil {
		return nil
	}
	phaseCtx, started := e.startPhase(ctx, state, model.PhaseCollect)
	cfg := state.req.RunConfig

	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		if syncer, ok := state.backend.(backend.WorkspaceSyncer); ok && state.sandboxInfo.ID != "" {
			_ = state.emitter.EmitEvent(phaseCtx, model.RunEvent{Name: "workspace.sync_out", Phase: model.PhaseCollect, At: time.Now().UTC()})
			remoteDir := path.Join(cfg.OpenSandbox.WorkspaceRoot, filepath.Base(cfg.Run.ArtifactDir))
			localDir := filepath.Join(cfg.Run.ArtifactDir, artifact.ArtifactsDirName, "opensandbox-workspace")
			if err := syncer.SyncWorkspaceOut(phaseCtx, state.sandboxInfo.ID, remoteDir, localDir); err != nil {
				state.result.Metadata["workspace_sync_out_error"] = err.Error()
			}
		}
		if provider, ok := state.backend.(backend.MetadataProvider); ok && state.sandboxInfo.ID != "" {
			info, err := provider.SandboxMetadata(phaseCtx, state.sandboxInfo.ID)
			if err == nil {
				state.sandboxInfo = info
				_ = state.writer.WriteSandbox(sandboxArtifact(info, cfg))
			} else {
				state.result.Metadata["sandbox_metadata_error"] = err.Error()
			}
			endpoints, err := provider.Endpoints(phaseCtx, state.sandboxInfo.ID, []int{44772, 8080})
			if err == nil {
				_ = state.writer.WriteEndpoints(model.EndpointsArtifact{Ports: endpoints})
			} else {
				state.result.Metadata["sandbox_endpoints_error"] = err.Error()
			}
		}
		if err := e.cleanupBackend(phaseCtx, state); err != nil {
			state.result.Metadata["sandbox_cleanup_error"] = err.Error()
			if state.result.Status == model.StatusSucceeded {
				state.result.Status = model.StatusPartial
			}
		}
	}

	refs, err := state.writer.ArtifactRefs()
	if err != nil {
		return e.failPhase(phaseCtx, state, model.PhaseCollect, started, model.ErrorCodeCollectFailed, err)
	}
	uploadRefs := makeUploadRefs(state.writer.Root(), refs)
	uploadedRefs, uploadErr := state.uploader.Upload(phaseCtx, uploadRefs, state.req.RunConfig.Artifacts, state.req.RunConfig.Run.DeploymentEnvironment, state.req.RunConfig.Run.RunID, state.req.RunConfig.Run.Attempt)
	state.result.Artifacts = mergeUploadedRefs(refs, uploadedRefs)
	if uploadErr != nil {
		state.result.Metadata["artifact_upload_error"] = uploadErr.Error()
		if state.result.Status == model.StatusSucceeded {
			state.result.Status = model.StatusPartial
			state.result.ErrorCode = model.ErrorCodeArtifactUploadFailed
			state.result.ErrorMessage = uploadErr.Error()
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

	state.phaseResults = append(state.phaseResults, successPhase(model.PhaseCollect, started, map[string]any{"artifacts": len(refs)}))
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
	_ = state.emitter.EndPhase(phaseCtx, model.PhaseCollect, state.phaseResults[len(state.phaseResults)-1])
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

func (e Engine) failWithoutArtifacts(state *runState, phase model.Phase, code model.ErrorCode, err error) error {
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
		"ts":            time.Now().UTC(),
		"phase":         phase,
		"command":       command,
		"command_class": proc.ClassifyCommand(command),
		"exit_code":     result.ExitCode,
		"timed_out":     result.TimedOut,
		"signal":        result.Signal,
		"duration_ms":   result.Duration.Milliseconds(),
		"target":        result.Target,
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
				"phase":         string(phase),
				"command_class": proc.ClassifyCommand(command),
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
	return backend.CreateSandboxRequest{
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		WorkspaceID:  cfg.Run.WorkspaceID,
		Image:        image,
		Entrypoint:   cfg.Sandbox.Entrypoint,
		Env:          env,
		Metadata:     map[string]string{"run_id": cfg.Run.RunID, "attempt": fmt.Sprintf("%d", cfg.Run.Attempt)},
		CPU:          cfg.Sandbox.CPU,
		Memory:       cfg.Sandbox.Memory,
		NetworkMode:  cfg.OpenSandbox.NetworkMode,
		TimeoutSec:   maxInt(cfg.OpenSandbox.TTLSec, 1800),
		WorkspaceDir: cfg.OpenSandbox.WorkspaceRoot,
	}
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
	default:
		return false
	}
}

func backendSnapshot(cfg model.RunConfig, caps model.BackendCapabilities) *model.BackendSnapshot {
	snapshot := &model.BackendSnapshot{
		Kind:         string(cfg.Backend.Kind),
		Provider:     string(cfg.Backend.Kind),
		Capabilities: caps,
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		snapshot.Provider = "opensandbox"
		snapshot.Runtime = string(cfg.OpenSandbox.Runtime)
		snapshot.ServerURL = cfg.OpenSandbox.BaseURL
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
		ID:          id,
		Status:      info.Status,
		NetworkMode: cfg.OpenSandbox.NetworkMode,
		ExpiresAt:   info.ExpiresAt,
		Metadata:    info.Metadata,
	}
}

func providerArtifact(cfg model.RunConfig, caps model.BackendCapabilities) model.ProviderArtifact {
	artifact := model.ProviderArtifact{
		BackendKind:         string(cfg.Backend.Kind),
		ProviderName:        string(cfg.Backend.Kind),
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
	return artifact
}

func sandboxArtifact(info backend.SandboxInfo, cfg model.RunConfig) model.SandboxArtifact {
	return model.SandboxArtifact{
		SandboxID:   info.ID,
		Status:      info.Status,
		ExpiresAt:   info.ExpiresAt,
		NetworkMode: cfg.OpenSandbox.NetworkMode,
		Metadata:    info.Metadata,
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
	if state.backend == nil || state.sandboxInfo.ID == "" || state.req.RunConfig.Backend.Kind != model.BackendKindOpenSandbox {
		return nil
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

func mergeUploadedRefs(refs []model.ArtifactRef, uploaded []model.ArtifactRef) []model.ArtifactRef {
	out := make([]model.ArtifactRef, len(refs))
	copy(out, refs)
	for i := range out {
		if i < len(uploaded) {
			out[i].URI = uploaded[i].URI
		}
	}
	return out
}
