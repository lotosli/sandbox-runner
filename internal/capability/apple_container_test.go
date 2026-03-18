package capability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestProbeAppleContainerReportsDefaultKernelHint(t *testing.T) {
	restore := forceAppleProbeRuntime(t)
	defer restore()

	logPath := filepath.Join(t.TempDir(), "calls.log")
	script := writeAppleContainerProbeScript(t, `#!/bin/sh
if [ "$1" = "system" ] && [ "$2" = "status" ]; then
  printf '%s\n' '{"status":"running"}'
  exit 0
fi
if [ "$1" = "create" ]; then
  printf '%s\n' "$*" >> "$APPLE_PROBE_LOG"
  echo "Error: default kernel not configured for architecture arm64" >&2
  exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`)
	t.Setenv("APPLE_PROBE_LOG", logPath)

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendAppleContainer,
		Provider:       model.ProviderNative,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.AppleContainer.Binary = script
	cfg.Run.Image = "alpine:3.20"

	_, err := Probe(context.Background(), cfg.Execution, cfg)
	if err == nil {
		t.Fatal("Probe() error = nil, want provider unreachable error")
	}
	if !strings.Contains(err.Error(), "container system kernel set --recommended --arch arm64 --force") {
		t.Fatalf("Probe() error = %v, want kernel fix hint", err)
	}
}

func TestProbeAppleContainerReportsServiceUnavailable(t *testing.T) {
	restore := forceAppleProbeRuntime(t)
	defer restore()

	script := writeAppleContainerProbeScript(t, `#!/bin/sh
if [ "$1" = "system" ] && [ "$2" = "status" ]; then
  echo "failed to list containers: XPC connection error: Connection invalid" >&2
  exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`)

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendAppleContainer,
		Provider:       model.ProviderNative,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.AppleContainer.Binary = script
	cfg.Run.Image = "alpine:3.20"

	_, err := Probe(context.Background(), cfg.Execution, cfg)
	if err == nil {
		t.Fatal("Probe() error = nil, want provider unreachable error")
	}
	if !strings.Contains(err.Error(), "container system start --disable-kernel-install") {
		t.Fatalf("Probe() error = %v, want start hint", err)
	}
}

func TestProbeAppleContainerChecksRunningServiceAndCreateProbe(t *testing.T) {
	restore := forceAppleProbeRuntime(t)
	defer restore()

	logPath := filepath.Join(t.TempDir(), "calls.log")
	script := writeAppleContainerProbeScript(t, `#!/bin/sh
if [ "$1" = "system" ] && [ "$2" = "status" ]; then
  printf '%s\n' '{"status":"running"}'
  exit 0
fi
if [ "$1" = "create" ]; then
  printf '%s\n' "$*" >> "$APPLE_PROBE_LOG"
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`)
	t.Setenv("APPLE_PROBE_LOG", logPath)

	cfg := config.DefaultRunConfig()
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendAppleContainer,
		Provider:       model.ProviderNative,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.AppleContainer.Binary = script
	cfg.Run.Image = "alpine:3.20"

	result, err := Probe(context.Background(), cfg.Execution, cfg)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.OK {
		t.Fatal("Probe() OK = false, want true")
	}
	if got := result.Details["probe_mode"]; got != "service_status+create_remove" {
		t.Fatalf("probe_mode = %v, want service_status+create_remove", got)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	args := string(data)
	if !strings.Contains(args, "create --remove alpine:3.20 /bin/sh -lc true") {
		t.Fatalf("create probe args = %q, want lightweight create probe", args)
	}
}

func writeAppleContainerProbeScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "container")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func forceAppleProbeRuntime(t *testing.T) func() {
	t.Helper()
	originalOS := appleContainerProbeGOOS
	originalArch := appleContainerProbeGOARCH
	appleContainerProbeGOOS = "darwin"
	appleContainerProbeGOARCH = "arm64"
	return func() {
		appleContainerProbeGOOS = originalOS
		appleContainerProbeGOARCH = originalArch
	}
}
