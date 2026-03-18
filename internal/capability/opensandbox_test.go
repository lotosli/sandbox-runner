package capability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestProbeOpenSandboxDefaultChecksConnectivityAndAuth(t *testing.T) {
	var sawAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy"})
		case r.Method == http.MethodGet && r.URL.Path == "/openapi.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"openapi": "3.1.0",
				"info": map[string]any{
					"title":   "OpenSandbox Lifecycle API",
					"version": "0.1.0",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes":
			sawAPIKey = r.Header.Get("OPEN-SANDBOX-API-KEY")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{},
				"pagination": map[string]any{
					"page":        1,
					"pageSize":    1,
					"totalItems":  0,
					"totalPages":  0,
					"hasNextPage": false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendOpenSandbox,
		Provider:       model.ProviderOpenSandbox,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.APIKey = "probe-key"

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("Probe() OK = false, want true")
	}
	if sawAPIKey != "probe-key" {
		t.Fatalf("api key = %q, want %q", sawAPIKey, "probe-key")
	}
	if got := result.Details["api_version"]; got != "0.1.0" {
		t.Fatalf("api_version = %v, want 0.1.0", got)
	}
}

func TestProbeOpenSandboxConditionalRuntimeUsesLiveProviderProbe(t *testing.T) {
	var created bool
	var deleted bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy"})
		case r.Method == http.MethodGet && r.URL.Path == "/openapi.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"openapi": "3.1.0",
				"info": map[string]any{
					"title":   "OpenSandbox Lifecycle API",
					"version": "0.1.0",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{},
				"pagination": map[string]any{
					"page":        1,
					"pageSize":    1,
					"totalItems":  0,
					"totalPages":  0,
					"hasNextPage": false,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			created = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			metadata := payload["metadata"].(map[string]any)
			if metadata["runtime.profile"] != "kata" {
				t.Fatalf("metadata runtime.profile = %v, want kata", metadata["runtime.profile"])
			}
			extensions := payload["extensions"].(map[string]any)
			if extensions["runtime.profile"] != "kata" {
				t.Fatalf("extensions runtime.profile = %v, want kata", extensions["runtime.profile"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "sbx-probe",
				"status": map[string]any{
					"state": "running",
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx-probe":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendOpenSandbox,
		Provider:       model.ProviderOpenSandbox,
		RuntimeProfile: model.ExecutionRuntimeProfileKata,
	}
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeKubernetes
	cfg.Kata.RuntimeClassName = "kata"
	cfg.Run.Image = "alpine:3.20"

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("Probe() OK = false, want true")
	}
	if !created {
		t.Fatal("expected runtime probe sandbox create request")
	}
	if !deleted {
		t.Fatal("expected runtime probe sandbox delete request")
	}
	if got := result.Details["runtime_probe_strategy"]; got != "provider.create_start_delete" {
		t.Fatalf("runtime_probe_strategy = %v, want provider.create_start_delete", got)
	}
}

func TestProbeOpenSandboxConditionalRuntimeMapsProviderRuntimeRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy"})
		case r.Method == http.MethodGet && r.URL.Path == "/openapi.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"openapi": "3.1.0",
				"info": map[string]any{
					"title":   "OpenSandbox Lifecycle API",
					"version": "0.1.0",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{},
				"pagination": map[string]any{
					"page":        1,
					"pageSize":    1,
					"totalItems":  0,
					"totalPages":  0,
					"hasNextPage": false,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":        "RUNTIME_UNSUPPORTED",
				"message":     "runtime profile kata is not supported",
				"status_code": http.StatusUnprocessableEntity,
				"retryable":   false,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendOpenSandbox,
		Provider:       model.ProviderOpenSandbox,
		RuntimeProfile: model.ExecutionRuntimeProfileKata,
	}
	cfg.OpenSandbox.BaseURL = server.URL
	cfg.OpenSandbox.Runtime = model.OpenSandboxRuntimeKubernetes
	cfg.Kata.RuntimeClassName = "kata"
	cfg.Run.Image = "alpine:3.20"

	_, err := Probe(context.Background(), cfg.Execution, cfg)
	if err == nil {
		t.Fatal("Probe() error = nil, want runtime unavailable error")
	}

	var runnerErr model.RunnerError
	if !errors.As(err, &runnerErr) {
		t.Fatalf("Probe() error = %T, want RunnerError", err)
	}
	if runnerErr.Code != string(model.ErrorCodeCapabilityRuntimeUnavailable) {
		t.Fatalf("Probe() code = %s, want %s", runnerErr.Code, model.ErrorCodeCapabilityRuntimeUnavailable)
	}
	if runnerErr.ProviderCode != "RUNTIME_UNSUPPORTED" {
		t.Fatalf("Probe() provider code = %s, want RUNTIME_UNSUPPORTED", runnerErr.ProviderCode)
	}
}
