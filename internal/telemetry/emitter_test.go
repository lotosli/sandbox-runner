package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
	otellog "go.opentelemetry.io/otel/log"
)

func TestRecordForStructuredLog(t *testing.T) {
	ts := time.Now().UTC()
	record := recordForStructuredLog(model.StructuredLog{
		Timestamp:    ts,
		RunID:        "r-1",
		Attempt:      2,
		Phase:        model.PhaseExecute,
		CommandClass: "test.run",
		Stream:       "stderr",
		LineNo:       42,
		Line:         "boom",
		Attributes:   map[string]string{"tool_name": "pytest"},
	})

	if got := record.EventName(); got != "process.stderr" {
		t.Fatalf("event name = %s, want process.stderr", got)
	}
	if got := record.Severity(); got != otellog.SeverityWarn {
		t.Fatalf("severity = %v, want %v", got, otellog.SeverityWarn)
	}
	if got := record.Body().AsString(); got != "boom" {
		t.Fatalf("body = %s, want boom", got)
	}
	if got := record.AttributesLen(); got < 6 {
		t.Fatalf("attributes len = %d, want >= 6", got)
	}
}

func TestLogEndpointURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "base URL", in: "http://127.0.0.1:4318", want: "http://127.0.0.1:4318/v1/logs"},
		{name: "root path", in: "http://127.0.0.1:4318/", want: "http://127.0.0.1:4318/v1/logs"},
		{name: "prefix path", in: "http://127.0.0.1:4318/prefix", want: "http://127.0.0.1:4318/prefix/v1/logs"},
		{name: "already logs path", in: "http://127.0.0.1:4318/prefix/v1/logs", want: "http://127.0.0.1:4318/prefix/v1/logs"},
		{name: "invalid passthrough", in: "://bad", want: "://bad"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := logEndpointURL(tc.in); got != tc.want {
				t.Fatalf("logEndpointURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEmitterEnabledExportsLogsAndTraces(t *testing.T) {
	var (
		mu   sync.Mutex
		hits = map[string]int{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Run.OTLPEndpoint = server.URL
	cfg.Run.RunID = "r-otlp"
	cfg.Run.Attempt = 2
	cfg.Run.SandboxID = "sandbox-otlp"
	cfg.Run.ServiceName = "sandbox-runner-test"

	emitter, err := New(context.Background(), cfg, true)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	runCtx, err := emitter.StartRun(context.Background(), &model.RunRequest{RunConfig: cfg})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	phaseCtx, err := emitter.StartPhase(runCtx, model.PhaseExecute)
	if err != nil {
		t.Fatalf("StartPhase() error = %v", err)
	}

	if err := emitter.EmitLog(phaseCtx, model.StructuredLog{
		Timestamp:    time.Now().UTC(),
		RunID:        cfg.Run.RunID,
		Attempt:      cfg.Run.Attempt,
		Phase:        model.PhaseExecute,
		CommandClass: "test.run",
		Stream:       "stdout",
		LineNo:       1,
		Line:         "hello collector",
	}); err != nil {
		t.Fatalf("EmitLog() error = %v", err)
	}

	if err := emitter.EndPhase(phaseCtx, model.PhaseExecute, model.PhaseResult{
		Phase:      model.PhaseExecute,
		Status:     model.StatusSucceeded,
		DurationMS: 12,
	}); err != nil {
		t.Fatalf("EndPhase() error = %v", err)
	}

	if err := emitter.EndRun(runCtx, &model.RunResult{Status: model.StatusSucceeded}); err != nil {
		t.Fatalf("EndRun() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["/v1/logs"] == 0 {
		t.Fatalf("log exporter did not POST to /v1/logs: hits=%v", hits)
	}
	if hits["/v1/traces"] == 0 {
		t.Fatalf("trace exporter did not POST to /v1/traces: hits=%v", hits)
	}
}
