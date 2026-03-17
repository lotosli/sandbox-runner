package platform

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	features.STGLinuxMode = cfg.Platform.RunMode == model.RunModeSTGLinux || cfg.Platform.RunMode == model.RunModeSTGOpenSandboxK8s

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

	if cfg.Platform.RunMode == model.RunModeLocalDirect {
		features.OBIEBPF = false
	}

	if cfg.Go.ManualSDKHelperEnabled {
		features.GoManualSDK = true
	}
	if cfg.Go.AutosdkBridgeEnabled && target.OS == "linux" {
		features.GoAutoSDKBridge = true
	}
	if cfg.Go.OBIEnabled && target.OS == "linux" && hasCapability(target.Capabilities, "cap_bpf") {
		features.OBIEBPF = true
	}
	target.BackendKind = string(cfg.Backend.Kind)
	if cfg.Backend.Kind == model.BackendKindOpenSandbox {
		target.ProviderName = "opensandbox"
		target.RuntimeKind = string(cfg.OpenSandbox.Runtime)
		target.NetworkMode = cfg.OpenSandbox.NetworkMode
	}
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
			if strings.HasSuffix(strings.ToLower(value), "2000000") || strings.Contains(strings.ToLower(value), "2000000") {
				return []string{"cap_bpf"}
			}
		}
	}
	return nil
}

func hasCapability(capabilities []string, capability string) bool {
	for _, item := range capabilities {
		if strings.EqualFold(item, capability) {
			return true
		}
	}
	return false
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
