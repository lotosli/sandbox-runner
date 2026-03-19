package telemetry

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type Emitter struct {
	cfg           model.RunConfig
	tp            *sdktrace.TracerProvider
	mp            *sdkmetric.MeterProvider
	lp            *sdklog.LoggerProvider
	tracer        trace.Tracer
	meter         metric.Meter
	logger        otellog.Logger
	runCtx        context.Context
	runSpan       trace.Span
	phaseCtxs     map[model.Phase]context.Context
	phaseSpans    map[model.Phase]trace.Span
	runCounter    metric.Int64Counter
	failCounter   metric.Int64Counter
	stdoutCounter metric.Int64Counter
	stderrCounter metric.Int64Counter
	artifactBytes metric.Int64Counter
	policyDenied  metric.Int64Counter
	phaseDuration metric.Float64Histogram
	commandDur    metric.Float64Histogram
	mu            sync.Mutex
}

func New(ctx context.Context, cfg model.RunConfig, enabled bool) (*Emitter, error) {
	res := resource.NewSchemaless(
		semconv.ServiceNameKey.String(cfg.Run.ServiceName),
		attribute.String("deployment.environment.name", cfg.Run.DeploymentEnvironment),
		attribute.String("run_id", cfg.Run.RunID),
		attribute.Int("attempt", cfg.Run.Attempt),
		attribute.String("sandbox_id", cfg.Run.SandboxID),
		attribute.String("execution.backend", string(cfg.Execution.Backend)),
		attribute.String("execution.provider", string(cfg.Execution.Provider)),
		attribute.String("execution.runtime_profile", string(cfg.Execution.RuntimeProfile)),
	)

	var (
		tp *sdktrace.TracerProvider
		mp *sdkmetric.MeterProvider
		lp *sdklog.LoggerProvider
	)
	if enabled {
		traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(cfg.Run.OTLPEndpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, err
		}
		metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(cfg.Run.OTLPEndpoint), otlpmetrichttp.WithInsecure())
		if err != nil {
			return nil, err
		}
		logExporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(logEndpointURL(cfg.Run.OTLPEndpoint)), otlploghttp.WithInsecure())
		if err != nil {
			return nil, err
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		)
		mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		)
		lp = sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		)
	} else {
		tp = sdktrace.NewTracerProvider(sdktrace.WithResource(res))
		mp = sdkmetric.NewMeterProvider(sdkmetric.WithResource(res))
		lp = sdklog.NewLoggerProvider(sdklog.WithResource(res))
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otellogglobal.SetLoggerProvider(lp)

	tracer := otel.Tracer("github.com/lotosli/sandbox-runner")
	meter := otel.Meter("github.com/lotosli/sandbox-runner")
	logger := lp.Logger("github.com/lotosli/sandbox-runner")

	runCounter, _ := meter.Int64Counter("sandbox_run_total")
	failCounter, _ := meter.Int64Counter("sandbox_run_failed_total")
	stdoutCounter, _ := meter.Int64Counter("sandbox_stdout_lines_total")
	stderrCounter, _ := meter.Int64Counter("sandbox_stderr_lines_total")
	artifactBytes, _ := meter.Int64Counter("sandbox_artifact_bytes_total")
	policyDenied, _ := meter.Int64Counter("sandbox_policy_denied_total")
	phaseDuration, _ := meter.Float64Histogram("sandbox_phase_duration_ms")
	commandDur, _ := meter.Float64Histogram("sandbox_command_duration_ms")

	return &Emitter{
		cfg:           cfg,
		tp:            tp,
		mp:            mp,
		lp:            lp,
		tracer:        tracer,
		meter:         meter,
		logger:        logger,
		phaseCtxs:     map[model.Phase]context.Context{},
		phaseSpans:    map[model.Phase]trace.Span{},
		runCounter:    runCounter,
		failCounter:   failCounter,
		stdoutCounter: stdoutCounter,
		stderrCounter: stderrCounter,
		artifactBytes: artifactBytes,
		policyDenied:  policyDenied,
		phaseDuration: phaseDuration,
		commandDur:    commandDur,
	}, nil
}

func (e *Emitter) StartRun(ctx context.Context, req *model.RunRequest) (context.Context, error) {
	runCtx, span := e.tracer.Start(ctx, "sandbox.run", trace.WithAttributes(commonAttrs(req.RunConfig, model.PhasePrepare)...))
	e.runCtx = runCtx
	e.runSpan = span
	e.runCounter.Add(runCtx, 1)
	return runCtx, nil
}

func (e *Emitter) StartPhase(ctx context.Context, phase model.Phase) (context.Context, error) {
	phaseCtx, span := e.tracer.Start(ctx, "phase."+string(phase), trace.WithAttributes(attribute.String("phase", string(phase))))
	e.mu.Lock()
	e.phaseCtxs[phase] = phaseCtx
	e.phaseSpans[phase] = span
	e.mu.Unlock()
	return phaseCtx, nil
}

func (e *Emitter) EmitEvent(ctx context.Context, event model.RunEvent) error {
	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{
		attribute.String("phase", string(event.Phase)),
		attribute.String("command_class", event.CommandClass),
	}
	for key, value := range event.Attributes {
		attrs = append(attrs, attribute.String(key, value))
	}
	span.AddEvent(event.Name, trace.WithAttributes(attrs...))
	if event.Name == "policy.denied" {
		e.policyDenied.Add(ctx, 1)
	}
	return nil
}

func (e *Emitter) EmitLog(ctx context.Context, entry model.StructuredLog) error {
	if entry.Stream == "stdout" {
		e.stdoutCounter.Add(ctx, 1)
	}
	if entry.Stream == "stderr" {
		e.stderrCounter.Add(ctx, 1)
	}
	trace.SpanFromContext(ctx).AddEvent("process."+entry.Stream, trace.WithAttributes(
		attribute.String("line", entry.Line),
		attribute.Int("line_no", entry.LineNo),
		attribute.String("provider", entry.Provider),
	))
	e.logger.Emit(ctx, recordForStructuredLog(entry))
	return nil
}

func (e *Emitter) EmitMetric(ctx context.Context, point model.MetricPoint) error {
	switch point.Name {
	case "sandbox_phase_duration_ms":
		e.phaseDuration.Record(ctx, point.Value)
	case "sandbox_command_duration_ms":
		e.commandDur.Record(ctx, point.Value)
	case "sandbox_artifact_bytes_total":
		e.artifactBytes.Add(ctx, int64(point.Value))
	}
	return nil
}

func (e *Emitter) EndPhase(ctx context.Context, phase model.Phase, result model.PhaseResult) error {
	e.phaseDuration.Record(ctx, float64(result.DurationMS), metric.WithAttributes(attribute.String("phase", string(phase))))
	e.mu.Lock()
	defer e.mu.Unlock()
	if span, ok := e.phaseSpans[phase]; ok {
		if result.Status != model.StatusSucceeded {
			span.SetStatus(codes.Error, result.ErrorMessage)
		}
		span.End()
		delete(e.phaseSpans, phase)
		delete(e.phaseCtxs, phase)
	}
	return nil
}

func (e *Emitter) EndRun(ctx context.Context, result *model.RunResult) error {
	if result != nil && result.Status != model.StatusSucceeded {
		e.failCounter.Add(ctx, 1)
		e.runSpan.SetStatus(codes.Error, result.ErrorMessage)
	}
	if e.runSpan != nil {
		e.runSpan.End()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.lp.ForceFlush(shutdownCtx); err != nil {
		return fmt.Errorf("flush log provider: %w", err)
	}
	if err := e.lp.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown log provider: %w", err)
	}
	if err := e.tp.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown trace provider: %w", err)
	}
	if err := e.mp.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown meter provider: %w", err)
	}
	return nil
}

func commonAttrs(cfg model.RunConfig, phase model.Phase) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("deployment.environment.name", cfg.Run.DeploymentEnvironment),
		attribute.String("run_id", cfg.Run.RunID),
		attribute.Int("attempt", cfg.Run.Attempt),
		attribute.String("sandbox_id", cfg.Run.SandboxID),
		attribute.String("phase", string(phase)),
	}
	if cfg.Execution.Backend != "" {
		attrs = append(attrs,
			attribute.String("execution.backend", string(cfg.Execution.Backend)),
			attribute.String("execution.provider", string(cfg.Execution.Provider)),
			attribute.String("execution.runtime_profile", string(cfg.Execution.RuntimeProfile)),
		)
	}
	if level := cfg.Metadata["execution.compatibility_level"]; level != "" {
		attrs = append(attrs, attribute.String("execution.compatibility_level", level))
	}
	if cfg.Backend.Kind != "" {
		attrs = append(attrs,
			attribute.String("backend.kind", string(cfg.Backend.Kind)),
			attribute.String("sandbox.backend.kind", string(cfg.Backend.Kind)),
		)
	}
	if cfg.Runtime.Profile != "" {
		attrs = append(attrs,
			attribute.String("runtime.profile", string(cfg.Runtime.Profile)),
			attribute.String("sandbox.runtime.profile", string(cfg.Runtime.Profile)),
			attribute.String("sandbox.runtime.class", cfg.Kata.RuntimeClassName),
		)
		attrs = append(attrs, attribute.String("sandbox.virtualization", telemetryVirtualization(cfg.Runtime.Profile)))
	}
	if cfg.Backend.Kind == model.BackendKindDevContainer {
		attrs = append(attrs,
			attribute.String("devcontainer.workspace_folder", cfg.DevContainer.WorkspaceFolder),
			attribute.String("devcontainer.config_path", cfg.DevContainer.ConfigPath),
		)
	}
	if provider := telemetryBackendProvider(cfg); provider != "" {
		attrs = append(attrs,
			attribute.String("backend.provider", provider),
			attribute.String("sandbox.provider.name", providerForTelemetry(cfg)),
		)
	}
	if platform := telemetryLocalPlatform(cfg); platform != "" {
		attrs = append(attrs, attribute.String("local.platform", platform))
	}
	if cfg.Backend.Kind == model.BackendKindOrbStackMachine && cfg.OrbStack.MachineName != "" {
		attrs = append(attrs, attribute.String("machine.name", cfg.OrbStack.MachineName))
	}
	if cfg.Backend.Kind != model.BackendKindOrbStackMachine && cfg.Run.SandboxID != "" {
		attrs = append(attrs, attribute.String("container.id", cfg.Run.SandboxID))
	}
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		attrs = append(attrs,
			attribute.String("sandbox.runtime.kind", string(cfg.OpenSandbox.Runtime)),
			attribute.String("sandbox.network.mode", cfg.OpenSandbox.NetworkMode),
			attribute.Bool("sandbox.ttl.enabled", cfg.OpenSandbox.TTLSec > 0),
		)
	} else if cfg.Backend.Kind == model.BackendKindDevContainer {
		attrs = append(attrs,
			attribute.String("sandbox.runtime.kind", "devcontainer"),
		)
	} else if cfg.Backend.Kind == model.BackendKindAppleContainer {
		attrs = append(attrs, attribute.String("sandbox.runtime.kind", "apple-container"))
	} else if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		attrs = append(attrs, attribute.String("sandbox.runtime.kind", "orbstack-machine"))
	}
	return attrs
}

func recordForStructuredLog(entry model.StructuredLog) otellog.Record {
	record := otellog.Record{}
	record.SetTimestamp(entry.Timestamp)
	record.SetObservedTimestamp(time.Now().UTC())
	record.SetSeverity(severityForStream(entry.Stream))
	record.SetSeverityText(strings.ToUpper(entry.Stream))
	record.SetEventName("process." + entry.Stream)
	record.SetBody(otellog.StringValue(entry.Line))

	attrs := []otellog.KeyValue{
		otellog.String("run_id", entry.RunID),
		otellog.Int("attempt", entry.Attempt),
		otellog.String("phase", string(entry.Phase)),
		otellog.String("command_class", entry.CommandClass),
		otellog.String("stream", entry.Stream),
		otellog.Int("line_no", entry.LineNo),
	}
	if entry.Provider != "" {
		attrs = append(attrs, otellog.String("provider", entry.Provider))
	}
	if entry.CommandID != "" {
		attrs = append(attrs, otellog.String("command_id", entry.CommandID))
	}
	if entry.ExecProviderID != "" {
		attrs = append(attrs, otellog.String("exec_provider_id", entry.ExecProviderID))
	}
	for key, value := range entry.Attributes {
		attrs = append(attrs, otellog.String(key, value))
	}
	record.AddAttributes(attrs...)
	return record
}

func severityForStream(stream string) otellog.Severity {
	switch stream {
	case "stderr":
		return otellog.SeverityWarn
	default:
		return otellog.SeverityInfo
	}
}

func telemetryBackendProvider(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "docker"
	case model.BackendKindK8s:
		return string(model.ExecutionProviderForK8sProvider(cfg.K8s.Provider))
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	default:
		return string(cfg.Backend.Kind)
	}
}

func providerForTelemetry(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindOpenSandbox:
		return "opensandbox"
	case model.BackendKindDevContainer:
		return "devcontainer"
	default:
		return telemetryBackendProvider(cfg)
	}
}

func telemetryLocalPlatform(cfg model.RunConfig) string {
	switch {
	case cfg.Backend.Kind == model.BackendKindAppleContainer:
		return "macos"
	case cfg.Backend.Kind == model.BackendKindOrbStackMachine:
		return "orbstack"
	case cfg.Backend.Kind == model.BackendKindDocker && cfg.Docker.Provider == model.DockerProviderOrbStack:
		return "orbstack"
	case cfg.Backend.Kind == model.BackendKindK8s:
		return model.K8sLocalPlatform(cfg.K8s.Provider)
	default:
		return ""
	}
}

func telemetryVirtualization(profile model.RuntimeProfile) string {
	switch profile {
	case model.RuntimeProfileKata:
		return "kata"
	case model.RuntimeProfileAppleContainer:
		return "apple-container"
	case model.RuntimeProfileOrbStackMachine:
		return "vm"
	default:
		return "none"
	}
}

func logEndpointURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}

	switch {
	case u.Path == "", u.Path == "/":
		u.Path = "/v1/logs"
	case strings.HasSuffix(u.Path, "/v1/logs"):
	default:
		u.Path = strings.TrimRight(u.Path, "/") + "/v1/logs"
	}
	return u.String()
}
