package model

import "strings"

const runtimeProfileAliasMicroVM = "microvm"

func NormalizeExecutionRuntimeProfile(profile ExecutionRuntimeProfile) ExecutionRuntimeProfile {
	switch strings.ToLower(strings.TrimSpace(string(profile))) {
	case "":
		return ""
	case string(ExecutionRuntimeProfileDefault):
		return ExecutionRuntimeProfileDefault
	case string(ExecutionRuntimeProfileKata):
		return ExecutionRuntimeProfileKata
	case string(ExecutionRuntimeProfileGVisor):
		return ExecutionRuntimeProfileGVisor
	case string(ExecutionRuntimeProfileFirecracker), runtimeProfileAliasMicroVM:
		return ExecutionRuntimeProfileFirecracker
	default:
		return profile
	}
}

func NormalizeRuntimeProfile(profile RuntimeProfile) RuntimeProfile {
	switch strings.ToLower(strings.TrimSpace(string(profile))) {
	case "":
		return ""
	case string(RuntimeProfileDefault):
		return RuntimeProfileDefault
	case string(RuntimeProfileNative):
		return RuntimeProfileNative
	case string(RuntimeProfileKata):
		return RuntimeProfileKata
	case string(RuntimeProfileGVisor):
		return RuntimeProfileGVisor
	case string(RuntimeProfileFirecracker), runtimeProfileAliasMicroVM:
		return RuntimeProfileFirecracker
	case string(RuntimeProfileAppleContainer):
		return RuntimeProfileAppleContainer
	case string(RuntimeProfileOrbStackDocker):
		return RuntimeProfileOrbStackDocker
	case string(RuntimeProfileOrbStackK8s):
		return RuntimeProfileOrbStackK8s
	case string(RuntimeProfileOrbStackMachine):
		return RuntimeProfileOrbStackMachine
	default:
		return profile
	}
}

func RuntimeClassNameForConfig(cfg RunConfig) string {
	if value := strings.TrimSpace(cfg.Runtime.ClassName); value != "" {
		return value
	}
	if value := strings.TrimSpace(cfg.Kata.RuntimeClassName); value != "" && RequiresRuntimeClass(ExecutionRuntimeProfileForConfig(cfg)) {
		return value
	}
	return ""
}

func RequiresRuntimeClass(profile ExecutionRuntimeProfile) bool {
	switch NormalizeExecutionRuntimeProfile(profile) {
	case ExecutionRuntimeProfileKata, ExecutionRuntimeProfileGVisor, ExecutionRuntimeProfileFirecracker:
		return true
	default:
		return false
	}
}

func ExecutionRuntimeProfileForConfig(cfg RunConfig) ExecutionRuntimeProfile {
	if normalized := NormalizeExecutionRuntimeProfile(cfg.Execution.RuntimeProfile); normalized != "" {
		return normalized
	}
	switch NormalizeRuntimeProfile(cfg.Runtime.Profile) {
	case RuntimeProfileKata:
		return ExecutionRuntimeProfileKata
	case RuntimeProfileGVisor:
		return ExecutionRuntimeProfileGVisor
	case RuntimeProfileFirecracker:
		return ExecutionRuntimeProfileFirecracker
	default:
		return ExecutionRuntimeProfileDefault
	}
}

func VirtualizationForRuntimeProfile(profile RuntimeProfile) string {
	switch NormalizeRuntimeProfile(profile) {
	case RuntimeProfileKata:
		return "kata"
	case RuntimeProfileGVisor:
		return "gvisor"
	case RuntimeProfileFirecracker:
		return "firecracker"
	default:
		return "none"
	}
}
