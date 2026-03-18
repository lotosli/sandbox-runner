package compat

import (
	"fmt"
	"strings"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func ValidateCompatibility(cfg model.ExecutionConfig) model.CompatibilityResult {
	for _, rule := range AllowedRules {
		if rule.Backend == cfg.Backend && rule.Provider == cfg.Provider && rule.RuntimeProfile == cfg.RuntimeProfile {
			return model.CompatibilityResult{
				Level:       rule.Level,
				MatchedRule: formatRule(rule.Backend, rule.Provider, rule.RuntimeProfile),
				Message:     rule.Message,
			}
		}
	}

	recommendations := []string{}
	for _, rule := range AllowedRules {
		if rule.Backend == cfg.Backend {
			recommendations = append(recommendations, formatRule(rule.Backend, rule.Provider, rule.RuntimeProfile))
		}
	}
	msg := fmt.Sprintf("unsupported execution combo: backend=%s provider=%s runtime_profile=%s", cfg.Backend, cfg.Provider, cfg.RuntimeProfile)
	if len(recommendations) > 0 {
		msg += "; supported combinations: " + strings.Join(recommendations, ", ")
	}
	return model.CompatibilityResult{
		Level:   model.SupportUnsupported,
		Message: msg,
	}
}

func formatRule(backend model.ExecutionBackend, provider model.ProviderKind, runtime model.ExecutionRuntimeProfile) string {
	return fmt.Sprintf("%s/%s/%s", backend, provider, runtime)
}
