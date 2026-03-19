package platform

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestResolveFeaturesRejectsOBIOnDarwin(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Go.OBIEnabled = true
	target := model.ExecutionTarget{OS: "darwin", Arch: "arm64", Mode: model.RunModeLocalDirect}
	if _, _, err := ResolveFeatures(cfg, target); err == nil {
		t.Fatal("expected OBI enablement on darwin to fail")
	}
}

func TestResolveFeaturesDisablesLinuxOnlyFeaturesOnWindowsLocal(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDirect
	cfg.Go.AutosdkBridgeEnabled = true
	target := model.ExecutionTarget{OS: "windows", Arch: "amd64", Mode: model.RunModeLocalDirect}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if features.OBIEBPF {
		t.Fatal("expected OBI/eBPF to be disabled on windows local mode")
	}
	if features.GoAutoSDKBridge {
		t.Fatal("expected Go Auto SDK bridge to be disabled on windows local mode")
	}
	if !features.LocalDirectMode {
		t.Fatal("expected local_direct feature gate to remain enabled")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestResolveFeaturesDisablesLinuxOnlyFeaturesOnAppleContainer(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalAppleContainer
	cfg.Backend.Kind = model.BackendKindAppleContainer
	cfg.Runtime.Profile = model.RuntimeProfileAppleContainer
	cfg.Go.AutosdkBridgeEnabled = true
	target := model.ExecutionTarget{OS: "darwin", Arch: "arm64", Mode: model.RunModeLocalAppleContainer}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if features.OBIEBPF {
		t.Fatal("expected OBI/eBPF to be disabled on apple-container mode")
	}
	if features.GoAutoSDKBridge {
		t.Fatal("expected auto SDK bridge to be disabled on apple-container mode")
	}
	if !features.LocalAppleContainerMode {
		t.Fatal("expected local_apple_container feature gate to be enabled")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestResolveFeaturesDisablesLinuxOnlyFeaturesOnOrbStackMachine(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalOrbStackMachine
	cfg.Backend.Kind = model.BackendKindOrbStackMachine
	cfg.Runtime.Profile = model.RuntimeProfileOrbStackMachine
	cfg.Go.AutosdkBridgeEnabled = true
	target := model.ExecutionTarget{OS: "darwin", Arch: "arm64", Mode: model.RunModeLocalOrbStackMachine}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if features.OBIEBPF {
		t.Fatal("expected OBI/eBPF to be disabled on orbstack-machine mode")
	}
	if features.GoAutoSDKBridge {
		t.Fatal("expected auto SDK bridge to be disabled on orbstack-machine mode")
	}
	if !features.LocalOrbStackMachineMode {
		t.Fatal("expected local_orbstack_machine feature gate to be enabled")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestResolveFeaturesRejectsOBIOnWindows(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Go.OBIEnabled = true
	target := model.ExecutionTarget{OS: "windows", Arch: "amd64", Mode: model.RunModeLocalDirect}
	if _, _, err := ResolveFeatures(cfg, target); err == nil {
		t.Fatal("expected OBI enablement on windows to fail")
	}
}

func TestResolveFeaturesDisablesLinuxOnlyFeaturesOnLocalDevContainer(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeLocalDevContainer
	cfg.Backend.Kind = model.BackendKindDevContainer
	cfg.Go.OBIEnabled = true
	cfg.Go.AutosdkBridgeEnabled = true
	target := model.ExecutionTarget{
		OS:           "linux",
		Arch:         "amd64",
		Mode:         model.RunModeLocalDevContainer,
		Capabilities: []string{"cap_bpf"},
	}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if features.OBIEBPF {
		t.Fatal("expected local_devcontainer to keep OBI disabled")
	}
	if features.GoAutoSDKBridge {
		t.Fatal("expected local_devcontainer to keep auto SDK bridge disabled")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestResolveFeaturesDisablesOBIWithoutLinuxCapability(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Go.OBIEnabled = true
	target := model.ExecutionTarget{
		OS:           "linux",
		Arch:         "amd64",
		Mode:         model.RunModeSTGLinux,
		Capabilities: []string{"cap_net_bind_service"},
	}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if features.OBIEBPF {
		t.Fatal("expected OBI/eBPF to stay disabled without CAP_BPF")
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when OBI is requested without CAP_BPF")
	}
}

func TestResolveFeaturesEnablesLinuxSTGOptionsWhenSupported(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Go.OBIEnabled = true
	cfg.Go.AutosdkBridgeEnabled = true
	target := model.ExecutionTarget{
		OS:           "linux",
		Arch:         "arm64",
		Mode:         model.RunModeSTGLinux,
		Capabilities: []string{"cap_bpf"},
	}

	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		t.Fatalf("ResolveFeatures() error = %v", err)
	}
	if !features.OBIEBPF {
		t.Fatal("expected OBI/eBPF to be enabled on supported Linux target")
	}
	if !features.GoAutoSDKBridge {
		t.Fatal("expected Go Auto SDK bridge to be enabled on supported Linux target")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestProviderHelpersReportNamedK8sProvider(t *testing.T) {
	cfg := config.DefaultRunConfig()
	cfg.Platform.RunMode = model.RunModeSTGLinux
	cfg.Backend.Kind = model.BackendKindK8s
	cfg.Execution = model.ExecutionConfig{
		Backend:        model.ExecutionBackendK8s,
		Provider:       model.ProviderMinikube,
		RuntimeProfile: model.ExecutionRuntimeProfileDefault,
	}
	cfg.K8s.Provider = model.K8sProviderMinikube
	if got := providerName(cfg); got != "minikube" {
		t.Fatalf("providerName() = %q, want minikube", got)
	}
	if got := backendProvider(cfg); got != "minikube" {
		t.Fatalf("backendProvider() = %q, want minikube", got)
	}
}

func TestParseLinuxCapabilities(t *testing.T) {
	caps, err := parseLinuxCapabilities("000000c000000000")
	if err != nil {
		t.Fatalf("parseLinuxCapabilities() error = %v", err)
	}
	if !hasCapability(caps, "cap_perfmon") {
		t.Fatalf("capabilities %v do not include cap_perfmon", caps)
	}
	if !hasCapability(caps, "cap_bpf") {
		t.Fatalf("capabilities %v do not include cap_bpf", caps)
	}
}
