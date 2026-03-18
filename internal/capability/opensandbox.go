package capability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
	osclient "github.com/lotosli/sandbox-runner/internal/opensandbox/client"
)

func probeOpenSandbox(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	if fullConfig.OpenSandbox.BaseURL == "" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "opensandbox base URL is required")
	}
	parsed, err := url.Parse(fullConfig.OpenSandbox.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "opensandbox base URL is invalid: %q", fullConfig.OpenSandbox.BaseURL)
	}

	client := osclient.New(osclient.Config{
		BaseURL:    parsed.String(),
		APIKey:     fullConfig.OpenSandbox.APIKey,
		Timeout:    openSandboxProbeHTTPTimeout(fullConfig),
		MaxRetries: 1,
		RetryWait:  250 * time.Millisecond,
	})

	details := map[string]any{
		"base_url":   parsed.String(),
		"runtime":    fullConfig.OpenSandbox.Runtime,
		"probe_mode": "provider.health+list",
	}
	warnings := []string{}

	health, err := client.Health(ctx)
	if err != nil {
		return model.CapabilityProbeResult{}, openSandboxConnectivityError(cfg, err, "opensandbox health check failed")
	}
	details["health_status"] = health.Status

	openAPIDoc, err := client.OpenAPI(ctx)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("opensandbox openapi discovery failed: %v", err))
	} else {
		details["api_version"] = openAPIDoc.Info.Version
		details["api_title"] = openAPIDoc.Info.Title
	}

	if _, err := client.ListSandboxes(ctx, 1, 1); err != nil {
		return model.CapabilityProbeResult{}, openSandboxConnectivityError(cfg, err, "opensandbox sandbox listing failed")
	}
	details["auth_checked"] = true

	if fullConfig.OpenSandbox.APIKey == "" {
		warnings = append(warnings, "opensandbox API key is empty; provider accepted unauthenticated probe requests")
	}

	if cfg.RuntimeProfile == model.ExecutionRuntimeProfileDefault {
		return okResult(details, warnings...), nil
	}

	runtimeDetails, runtimeWarnings, err := probeOpenSandboxRuntimeProfile(ctx, client, cfg, fullConfig)
	if err != nil {
		return model.CapabilityProbeResult{}, err
	}
	for key, value := range runtimeDetails {
		details[key] = value
	}
	warnings = append(warnings, runtimeWarnings...)
	return okResult(details, warnings...), nil
}

func probeOpenSandboxRuntimeProfile(ctx context.Context, client *osclient.Client, cfg model.ExecutionConfig, fullConfig model.RunConfig) (map[string]any, []string, error) {
	image := strings.TrimSpace(fullConfig.Sandbox.Image)
	if image == "" {
		image = strings.TrimSpace(fullConfig.Run.Image)
	}
	if image == "" {
		return nil, nil, probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "opensandbox runtime probe requires sandbox.image or run.image")
	}

	createReq := osclient.OSSandboxCreateRequest{
		Image: osclient.OSImageSpec{
			URI: image,
		},
		Timeout:        60,
		ResourceLimits: osclient.OSResourceLimits{},
		Metadata: map[string]string{
			"probe.kind":            "runtime_profile",
			"probe.managed_by":      "sandbox-runner",
			"probe.runtime_profile": string(cfg.RuntimeProfile),
			"runtime.profile":       string(cfg.RuntimeProfile),
		},
		Extensions: map[string]string{
			"runtime.profile": string(cfg.RuntimeProfile),
		},
	}
	if fullConfig.OpenSandbox.Runtime != "" {
		createReq.Extensions["runtime"] = string(fullConfig.OpenSandbox.Runtime)
	}
	if workspaceRoot := strings.TrimSpace(fullConfig.OpenSandbox.WorkspaceRoot); workspaceRoot != "" {
		createReq.Extensions["workspace_dir"] = workspaceRoot
	}
	if cfg.RuntimeProfile == model.ExecutionRuntimeProfileKata && strings.TrimSpace(fullConfig.Kata.RuntimeClassName) != "" {
		createReq.Metadata["runtime.class"] = fullConfig.Kata.RuntimeClassName
		createReq.Extensions["runtime.class"] = fullConfig.Kata.RuntimeClassName
	}

	info, err := client.CreateSandbox(ctx, createReq)
	if err != nil {
		return nil, nil, openSandboxRuntimeProbeError(cfg, err)
	}

	cleanupWarnings := []string{}
	waitErr := waitForOpenSandboxProbeSandbox(ctx, client, cfg, fullConfig, info)
	if cleanupErr := deleteOpenSandboxProbeSandbox(client, info.ID); cleanupErr != nil {
		cleanupWarnings = append(cleanupWarnings, fmt.Sprintf("opensandbox runtime probe cleanup failed: %v", cleanupErr))
	}
	if waitErr != nil {
		return nil, cleanupWarnings, waitErr
	}

	return map[string]any{
		"runtime_probe_strategy": "provider.create_start_delete",
		"runtime_probe_image":    image,
		"runtime_probe_status":   strings.ToLower(info.Status.State),
	}, cleanupWarnings, nil
}

func waitForOpenSandboxProbeSandbox(ctx context.Context, client *osclient.Client, cfg model.ExecutionConfig, fullConfig model.RunConfig, info osclient.OSSandboxInfo) error {
	timeout := time.Duration(fullConfig.OpenSandbox.CreateTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	pollInterval := time.Duration(fullConfig.OpenSandbox.PollIntervalMs) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	current := info
	for {
		switch state := strings.ToLower(current.Status.State); state {
		case "running", "paused":
			return nil
		case "failed", "terminated":
			return openSandboxRuntimeStateError(cfg, current.Status.State, current.Status.Reason, current.Status.Message)
		}

		select {
		case <-deadlineCtx.Done():
			return model.RunnerError{
				Code:        string(model.ErrorCodeCapabilityProbeFailed),
				Message:     fmt.Sprintf("opensandbox runtime probe timed out waiting for runtime profile %s", cfg.RuntimeProfile),
				BackendKind: string(cfg.Backend),
				Cause:       deadlineCtx.Err(),
			}
		case <-time.After(pollInterval):
		}

		next, err := client.GetSandbox(deadlineCtx, current.ID)
		if err != nil {
			return openSandboxConnectivityError(cfg, err, "opensandbox runtime probe status lookup failed")
		}
		current = next
	}
}

func deleteOpenSandboxProbeSandbox(client *osclient.Client, sandboxID string) error {
	if strings.TrimSpace(sandboxID) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := client.DeleteSandbox(ctx, sandboxID)
	if err == nil {
		return nil
	}
	var providerErr osclient.ProviderError
	if errors.As(err, &providerErr) && providerErr.StatusCode == 404 {
		return nil
	}
	return err
}

func openSandboxConnectivityError(cfg model.ExecutionConfig, err error, action string) error {
	return openSandboxProbeError(model.ErrorCodeCapabilityProviderUnreachable, cfg, err, "%s: %s", action, openSandboxProviderMessage(err))
}

func openSandboxRuntimeProbeError(cfg model.ExecutionConfig, err error) error {
	var providerErr osclient.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode == 401 || providerErr.StatusCode == 403 {
			return openSandboxProbeError(model.ErrorCodeCapabilityProviderUnreachable, cfg, err, "opensandbox runtime probe authentication failed: %s", openSandboxProviderMessage(err))
		}
		if providerErr.StatusCode >= 400 && providerErr.StatusCode < 500 {
			return openSandboxProbeError(model.ErrorCodeCapabilityRuntimeUnavailable, cfg, err, "opensandbox provider rejected runtime profile %s: %s", cfg.RuntimeProfile, openSandboxProviderMessage(err))
		}
	}
	return openSandboxProbeError(model.ErrorCodeCapabilityProbeFailed, cfg, err, "opensandbox runtime probe failed while creating sandbox: %s", openSandboxProviderMessage(err))
}

func openSandboxRuntimeStateError(cfg model.ExecutionConfig, state, reason, message string) error {
	parts := []string{strings.ToLower(strings.TrimSpace(state))}
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return model.RunnerError{
		Code:        string(model.ErrorCodeCapabilityRuntimeUnavailable),
		Message:     fmt.Sprintf("opensandbox runtime probe failed for runtime profile %s: %s", cfg.RuntimeProfile, strings.Join(parts, ": ")),
		BackendKind: string(cfg.Backend),
	}
}

func openSandboxProbeError(code model.ErrorCode, cfg model.ExecutionConfig, err error, format string, args ...any) error {
	runnerErr := model.RunnerError{
		Code:        string(code),
		Message:     fmt.Sprintf(format, args...),
		BackendKind: string(cfg.Backend),
		Cause:       err,
	}
	var providerErr osclient.ProviderError
	if errors.As(err, &providerErr) {
		runnerErr.ProviderCode = providerErr.Code
		runnerErr.Retryable = providerErr.Retryable
	}
	return runnerErr
}

func openSandboxProviderMessage(err error) string {
	if err == nil {
		return "unknown provider error"
	}
	var providerErr osclient.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Code != "" {
			return fmt.Sprintf("%s: %s", providerErr.Code, providerErr.Message)
		}
		return providerErr.Message
	}
	return err.Error()
}

func openSandboxProbeHTTPTimeout(fullConfig model.RunConfig) time.Duration {
	timeout := time.Duration(fullConfig.OpenSandbox.CreateTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if timeout > 30*time.Second {
		return 30 * time.Second
	}
	return timeout
}
