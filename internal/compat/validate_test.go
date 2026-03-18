package compat

import (
	"testing"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestValidateCompatibility(t *testing.T) {
	tests := []struct {
		name  string
		cfg   model.ExecutionConfig
		level model.SupportLevel
	}{
		{
			name: "supported direct native default",
			cfg: model.ExecutionConfig{
				Backend:        model.ExecutionBackendDirect,
				Provider:       model.ProviderNative,
				RuntimeProfile: model.ExecutionRuntimeProfileDefault,
			},
			level: model.SupportSupported,
		},
		{
			name: "conditional k8s native kata",
			cfg: model.ExecutionConfig{
				Backend:        model.ExecutionBackendK8s,
				Provider:       model.ProviderNative,
				RuntimeProfile: model.ExecutionRuntimeProfileKata,
			},
			level: model.SupportConditional,
		},
		{
			name: "unsupported direct gke kata",
			cfg: model.ExecutionConfig{
				Backend:        model.ExecutionBackendDirect,
				Provider:       model.ProviderGKE,
				RuntimeProfile: model.ExecutionRuntimeProfileKata,
			},
			level: model.SupportUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCompatibility(tt.cfg)
			if result.Level != tt.level {
				t.Fatalf("ValidateCompatibility() level = %s, want %s", result.Level, tt.level)
			}
		})
	}
}
