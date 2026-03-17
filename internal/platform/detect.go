package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type Report struct {
	ExecutionTarget model.ExecutionTarget `json:"execution_target"`
	FeatureGates    model.FeatureSet      `json:"feature_gates"`
	Warnings        []string              `json:"warnings,omitempty"`
}

func Detect(mode model.RunMode) model.ExecutionTarget {
	target := model.ExecutionTarget{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		Mode: mode,
	}
	target.InContainer = inContainer()
	target.InKubernetes = os.Getenv("KUBERNETES_SERVICE_HOST") != ""
	target.DockerAvailable = dockerAvailable()
	if runtime.GOOS == "linux" {
		target.Capabilities = detectLinuxCapabilities()
	}
	return target
}

func ResolveFeatures(cfg model.RunConfig, target model.ExecutionTarget) (model.FeatureSet, []string, error) {
	features := cfg.Platform.FeatureGates
	warnings := []string{}

	features.LocalDirectMode = cfg.Platform.RunMode == model.RunModeLocalDirect
	features.LocalDockerMode = cfg.Platform.RunMode == model.RunModeLocalDocker || cfg.Platform.RunMode == model.RunModeLocalOpenSandboxDocker
	features.LocalAppleContainerMode = cfg.Platform.RunMode == model.RunModeLocalAppleContainer
	features.LocalOrbStackMachineMode = cfg.Platform.RunMode == model.RunModeLocalOrbStackMachine
	features.STGLinuxMode = cfg.Platform.RunMode == model.RunModeSTGLinux || cfg.Platform.RunMode == model.RunModeSTGOpenSandboxK8s
	localRestrictedMode := cfg.Platform.RunMode == model.RunModeLocalDirect ||
		cfg.Platform.RunMode == model.RunModeLocalDevContainer ||
		cfg.Platform.RunMode == model.RunModeLocalAppleContainer ||
		cfg.Platform.RunMode == model.RunModeLocalOrbStackMachine

	if target.OS != "linux" {
		if features.OBIEBPF || cfg.Go.OBIEnabled {
			return features, warnings, UnsupportedFeatureError("features.obi_ebpf", target.OS, "switch to Linux Docker mode or stg_linux")
		}
		features.OBIEBPF = false
		features.GoAutoSDKBridge = false
	}

	if target.OS == "linux" && !hasCapability(target.Capabilities, "cap_bpf") {
		features.OBIEBPF = false
		if cfg.Go.OBIEnabled {
			warnings = append(warnings, "obi_ebpf requested but CAP_BPF not available; feature disabled")
		}
	}

	if localRestrictedMode {
		features.OBIEBPF = false
		features.GoAutoSDKBridge = false
	}

	if cfg.Go.ManualSDKHelperEnabled {
		features.GoManualSDK = true
	}
	if cfg.Go.AutosdkBridgeEnabled && target.OS == "linux" && !localRestrictedMode {
		features.GoAutoSDKBridge = true
	}
	if cfg.Go.OBIEnabled && target.OS == "linux" && hasCapability(target.Capabilities, "cap_bpf") && !localRestrictedMode {
		features.OBIEBPF = true
	}
	target.BackendKind = string(cfg.Backend.Kind)
	target.ProviderName = providerName(cfg)
	target.BackendProvider = backendProvider(cfg)
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		target.ProviderName = "opensandbox"
		target.RuntimeKind = string(cfg.OpenSandbox.Runtime)
		target.NetworkMode = cfg.OpenSandbox.NetworkMode
	} else if cfg.Backend.Kind == model.BackendKindDevContainer {
		target.ProviderName = "devcontainer"
		target.RuntimeKind = "devcontainer"
	} else if cfg.Backend.Kind == model.BackendKindAppleContainer {
		target.RuntimeKind = "apple-container"
		target.LocalPlatform = "macos"
	} else if cfg.Backend.Kind == model.BackendKindOrbStackMachine {
		target.RuntimeKind = "orbstack-machine"
		target.LocalPlatform = "orbstack"
		target.MachineName = cfg.OrbStack.MachineName
	} else if cfg.Backend.Kind == model.BackendKindDocker && cfg.Docker.Provider == model.DockerProviderOrbStack {
		target.LocalPlatform = "orbstack"
	} else if cfg.Backend.Kind == model.BackendKindK8s && cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
		target.LocalPlatform = "orbstack"
	}
	target.RuntimeProfile = string(cfg.Runtime.Profile)
	target.RuntimeClassName = cfg.Kata.RuntimeClassName
	target.Virtualization = virtualizationForProfile(cfg.Runtime.Profile)
	return features, warnings, nil
}

func DoctorReport(image string) Report {
	target := Detect(model.RunModeLocalDirect)
	cfg := config.DefaultRunConfig()
	if image != "" {
		cfg.Run.Image = image
		cfg.Platform.RunMode = model.RunModeLocalDocker
		target.Mode = model.RunModeLocalDocker
	}
	features, warnings, err := ResolveFeatures(cfg, target)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	return Report{
		ExecutionTarget: target,
		FeatureGates:    features,
		Warnings:        warnings,
	}
}

func MarshalReport(r Report) string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}

type unsupportedFeatureError struct {
	Feature     string
	Platform    string
	Remediation string
}

func (e unsupportedFeatureError) Error() string {
	return "unsupported feature " + e.Feature + " on " + e.Platform + ": " + e.Remediation
}

func UnsupportedFeatureError(feature, platformName, remediation string) error {
	return unsupportedFeatureError{Feature: feature, Platform: platformName, Remediation: remediation}
}

func dockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		text := string(data)
		return strings.Contains(text, "docker") || strings.Contains(text, "kubepods") || strings.Contains(text, "containerd")
	}
	return false
}

func detectLinuxCapabilities() []string {
	statusPath := "/proc/self/status"
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CapEff:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
			caps, err := parseLinuxCapabilities(value)
			if err == nil {
				return caps
			}
			return nil
		}
	}
	return nil
}

func parseLinuxCapabilities(value string) ([]string, error) {
	mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("parse CapEff %q: %w", value, err)
	}
	out := make([]string, 0, len(linuxCapabilityNames))
	for bit, name := range linuxCapabilityNames {
		if mask&(uint64(1)<<bit) != 0 {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

func hasCapability(capabilities []string, capability string) bool {
	for _, item := range capabilities {
		if strings.EqualFold(item, capability) {
			return true
		}
	}
	return false
}

var linuxCapabilityNames = map[uint]string{
	0:  "cap_chown",
	1:  "cap_dac_override",
	2:  "cap_dac_read_search",
	3:  "cap_fowner",
	4:  "cap_fsetid",
	5:  "cap_kill",
	6:  "cap_setgid",
	7:  "cap_setuid",
	8:  "cap_setpcap",
	9:  "cap_linux_immutable",
	10: "cap_net_bind_service",
	11: "cap_net_broadcast",
	12: "cap_net_admin",
	13: "cap_net_raw",
	14: "cap_ipc_lock",
	15: "cap_ipc_owner",
	16: "cap_sys_module",
	17: "cap_sys_rawio",
	18: "cap_sys_chroot",
	19: "cap_sys_ptrace",
	20: "cap_sys_pacct",
	21: "cap_sys_admin",
	22: "cap_sys_boot",
	23: "cap_sys_nice",
	24: "cap_sys_resource",
	25: "cap_sys_time",
	26: "cap_sys_tty_config",
	27: "cap_mknod",
	28: "cap_lease",
	29: "cap_audit_write",
	30: "cap_audit_control",
	31: "cap_setfcap",
	32: "cap_mac_override",
	33: "cap_mac_admin",
	34: "cap_syslog",
	35: "cap_wake_alarm",
	36: "cap_block_suspend",
	37: "cap_audit_read",
	38: "cap_perfmon",
	39: "cap_bpf",
	40: "cap_checkpoint_restore",
}

func providerName(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindOpenSandbox:
		return "opensandbox"
	case model.BackendKindDevContainer:
		return "devcontainer"
	default:
		return string(cfg.Backend.Kind)
	}
}

func backendProvider(cfg model.RunConfig) string {
	switch cfg.Backend.Kind {
	case model.BackendKindDirect:
		return "native"
	case model.BackendKindDocker:
		if cfg.Docker.Provider == model.DockerProviderOrbStack {
			return "orbstack"
		}
		return "docker"
	case model.BackendKindK8s:
		if cfg.K8s.Provider == model.K8sProviderOrbStackLocal {
			return "orbstack"
		}
		return "k8s"
	case model.BackendKindOrbStackMachine:
		return "orbstack"
	default:
		return string(cfg.Backend.Kind)
	}
}

func virtualizationForProfile(profile model.RuntimeProfile) string {
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

func KubeconfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if value := os.Getenv("KUBECONFIG"); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}
