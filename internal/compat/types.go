package compat

import "github.com/lotosli/sandbox-runner/internal/model"

type CompatibilityRule struct {
	Backend        model.ExecutionBackend
	Provider       model.ProviderKind
	RuntimeProfile model.ExecutionRuntimeProfile
	Level          model.SupportLevel
	Message        string
}
