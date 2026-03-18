package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

var (
	appleContainerProbeGOOS   = runtime.GOOS
	appleContainerProbeGOARCH = runtime.GOARCH
)

func probeAppleContainer(ctx context.Context, cfg model.ExecutionConfig, fullConfig model.RunConfig) (model.CapabilityProbeResult, error) {
	if appleContainerProbeGOOS != "darwin" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple-container backend requires macOS")
	}
	if appleContainerProbeGOARCH != "arm64" {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple-container backend requires darwin/arm64")
	}
	path, err := exec.LookPath(fullConfig.AppleContainer.Binary)
	if err != nil {
		return model.CapabilityProbeResult{}, probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple container CLI not found: %v", err)
	}
	image := fullConfig.AppleContainer.Image
	if image == "" {
		image = fullConfig.Run.Image
	}
	status, err := appleContainerSystemStatus(ctx, path)
	if err != nil {
		return model.CapabilityProbeResult{}, appleContainerProviderError(cfg, "apple container service check failed", err)
	}
	if status.Status != "running" {
		return model.CapabilityProbeResult{}, probeFailure(
			model.ErrorCodeCapabilityProviderUnreachable,
			cfg,
			"apple container services are not running (status=%s); run `container system start --disable-kernel-install`",
			firstNonEmpty(status.Status, "unknown"),
		)
	}
	if image == "" {
		return okResult(map[string]any{
			"binary":         path,
			"service_status": status.Status,
		}), nil
	}
	if err := appleContainerCreateProbe(ctx, path, image, fullConfig.AppleContainer.CreateTimeoutSec); err != nil {
		return model.CapabilityProbeResult{}, appleContainerCreateProbeError(cfg, image, err)
	}
	return okResult(map[string]any{
		"binary":         path,
		"image":          image,
		"service_status": status.Status,
		"probe_mode":     "service_status+create_remove",
	}), nil
}

type appleContainerSystemStatusResult struct {
	Status string `json:"status"`
}

func appleContainerSystemStatus(ctx context.Context, binary string) (appleContainerSystemStatusResult, error) {
	output, err := appleContainerCommand(ctx, binary, 15*time.Second, "system", "status", "--format", "json")
	if err != nil {
		return appleContainerSystemStatusResult{}, err
	}
	var result appleContainerSystemStatusResult
	if err := json.Unmarshal(output, &result); err != nil {
		return appleContainerSystemStatusResult{}, fmt.Errorf("parse apple container system status: %w", err)
	}
	return result, nil
}

func appleContainerCreateProbe(ctx context.Context, binary string, image string, timeoutSec int) error {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	_, err := appleContainerCommand(ctx, binary, timeout, "create", "--remove", image, "/bin/sh", "-lc", "true")
	return err
}

func appleContainerCommand(ctx context.Context, binary string, timeout time.Duration, args ...string) ([]byte, error) {
	commandCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(commandCtx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return nil, fmt.Errorf("%s", text)
	}
	return output, nil
}

func appleContainerProviderError(cfg model.ExecutionConfig, action string, err error) error {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "XPC connection error"), strings.Contains(message, "Connection invalid"):
		return probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "%s: %s; run `container system start --disable-kernel-install`", action, message)
	default:
		return probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "%s: %s", action, message)
	}
}

func appleContainerCreateProbeError(cfg model.ExecutionConfig, image string, err error) error {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "default kernel not configured"):
		return probeFailure(
			model.ErrorCodeCapabilityProviderUnreachable,
			cfg,
			"apple-container default kernel is not configured; run `container system kernel set --recommended --arch arm64 --force` then `container system start --disable-kernel-install`: %s",
			message,
		)
	case strings.Contains(message, "XPC connection error"), strings.Contains(message, "Connection invalid"):
		return probeFailure(model.ErrorCodeCapabilityProviderUnreachable, cfg, "apple container service became unavailable during create probe: %s", message)
	default:
		return probeFailure(model.ErrorCodeCapabilityProbeFailed, cfg, "apple container create probe failed for image %s: %s", image, message)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
