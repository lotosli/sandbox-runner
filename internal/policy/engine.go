package policy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/shlex"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type Engine struct {
	cfg    model.PolicyConfig
	runCfg model.RunConfig
}

func NewEngine(cfg model.PolicyConfig, runCfg model.RunConfig) Engine {
	return Engine{cfg: cfg, runCfg: runCfg}
}

func (e Engine) CheckCommand(ctx context.Context, phase model.Phase, cmd []string) error {
	_ = ctx
	if len(cmd) == 0 {
		return PolicyError{Code: model.ErrorCodePolicyToolDeny, Message: "empty command"}
	}
	tool := filepath.Base(cmd[0])
	if tool == "" {
		return PolicyError{Code: model.ErrorCodePolicyToolDeny, Message: "empty command"}
	}
	if !containsFold(e.cfg.Tools.Allow, tool) && !strings.HasPrefix(cmd[0], ".") && !strings.HasPrefix(cmd[0], "/") {
		return PolicyError{Code: model.ErrorCodePolicyToolDeny, Message: fmt.Sprintf("tool %s is not allowed in phase %s", tool, phase)}
	}
	joined := strings.Join(procNormalize(cmd), " ")
	for _, pattern := range e.cfg.Tools.DenyPatterns {
		if matchesPattern(strings.ToLower(pattern), strings.ToLower(joined)) {
			return PolicyError{Code: model.ErrorCodePolicyToolDeny, Message: fmt.Sprintf("command denied by pattern %q", pattern)}
		}
	}
	return nil
}

func (e Engine) CheckPathRead(ctx context.Context, phase model.Phase, path string) error {
	_ = ctx
	_ = phase
	return e.checkPath(path, e.cfg.Filesystem.ReadAllow)
}

func (e Engine) CheckPathWrite(ctx context.Context, phase model.Phase, path string) error {
	_ = ctx
	_ = phase
	return e.checkPath(path, e.cfg.Filesystem.WriteAllow)
}

func (e Engine) ResolveNetworkProfile(ctx context.Context, phase model.Phase) (model.NetworkProfile, error) {
	_ = ctx
	name := phaseNetworkProfile(e.runCfg, phase)
	profile, ok := e.cfg.NetworkProfiles[name]
	if !ok {
		return model.NetworkProfile{}, PolicyError{Code: model.ErrorCodePolicyNetDeny, Message: fmt.Sprintf("network profile %s not found", name)}
	}
	if e.runCfg.Platform.RunMode == model.RunModeLocalDirect {
		return profile, nil
	}
	return profile, nil
}

func (e Engine) ResolveSecrets(ctx context.Context, phase model.Phase) (map[string]string, error) {
	_ = ctx
	values := map[string]string{}
	for _, binding := range e.cfg.Secrets.InjectEnv {
		if !containsPhase(binding.PhaseAllow, phase) {
			continue
		}
		value := strings.TrimSpace(getenv(binding.Name))
		if value == "" {
			continue
		}
		values[binding.Name] = value
	}
	return values, nil
}

func (e Engine) TimeoutForPhase(phase model.Phase) int {
	switch phase {
	case model.PhasePrepare:
		return e.runCfg.Phases.Prepare.TimeoutSec
	case model.PhaseSetup:
		return max(e.runCfg.Phases.Setup.TimeoutSec, e.cfg.Resources.TimeoutSecDefault)
	case model.PhaseExecute:
		return max(e.runCfg.Phases.Execute.TimeoutSec, e.cfg.Resources.TimeoutSecDefault)
	case model.PhaseVerify:
		return max(e.runCfg.Phases.Verify.TimeoutSec, e.cfg.Resources.TimeoutSecDefault)
	case model.PhaseCollect:
		return max(e.runCfg.Phases.Collect.TimeoutSec, e.cfg.Resources.TimeoutSecDefault)
	default:
		return e.cfg.Resources.TimeoutSecDefault
	}
}

func (e Engine) EnforceCapabilities(phase model.Phase) error {
	_ = phase
	if e.runCfg.Platform.RunMode == model.RunModeLocalDirect {
		return nil
	}
	return nil
}

func (e Engine) checkPath(path string, allow []string) error {
	cleaned := filepath.Clean(path)
	workspace := filepath.Clean(e.runCfg.Run.WorkspaceDir)
	artifactDir := filepath.Clean(e.runCfg.Run.ArtifactDir)
	if strings.HasPrefix(cleaned, workspace) || strings.HasPrefix(cleaned, artifactDir) {
		return nil
	}
	for _, denied := range e.cfg.Filesystem.Deny {
		if strings.HasPrefix(cleaned, filepath.Clean(denied)) {
			return PolicyError{Code: model.ErrorCodePolicyFSDeny, Message: fmt.Sprintf("path %s denied", path)}
		}
	}
	for _, prefix := range allow {
		if strings.HasPrefix(cleaned, filepath.Clean(prefix)) {
			return nil
		}
	}
	return PolicyError{Code: model.ErrorCodePolicyFSDeny, Message: fmt.Sprintf("path %s outside allowed roots", path)}
}

type PolicyError struct {
	Code    model.ErrorCode
	Message string
}

func (e PolicyError) Error() string { return e.Message }

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func containsPhase(items []model.Phase, phase model.Phase) bool {
	for _, item := range items {
		if item == phase {
			return true
		}
	}
	return false
}

func phaseNetworkProfile(cfg model.RunConfig, phase model.Phase) string {
	switch phase {
	case model.PhaseSetup:
		return cfg.Phases.Setup.NetworkProfile
	case model.PhaseExecute:
		return cfg.Phases.Execute.NetworkProfile
	case model.PhaseVerify:
		return cfg.Phases.Verify.NetworkProfile
	default:
		return ""
	}
}

func procNormalize(cmd []string) []string {
	if len(cmd) >= 3 && (cmd[0] == "bash" || cmd[0] == "sh") && (cmd[1] == "-lc" || cmd[1] == "-c") {
		if parsed, err := shlex.Split(cmd[2]); err == nil && len(parsed) > 0 {
			return parsed
		}
	}
	return cmd
}

func matchesPattern(pattern, value string) bool {
	regex := regexp.QuoteMeta(pattern)
	regex = strings.ReplaceAll(regex, "\\*", ".*")
	return regexp.MustCompile("^" + regex + "$").MatchString(value)
}

func getenv(key string) string {
	return strings.TrimSpace(strings.TrimSpace(strings.TrimSpace(getEnv(key))))
}

var getEnv = func(key string) string {
	return os.Getenv(key)
}
