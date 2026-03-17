# cmd

Scope: binary entrypoints only.

Current layout:

- `sandbox-runner/`: the shipped CLI binary entrypoint

Rules:

- keep business logic out of `cmd/`
- defer parsing and orchestration to `internal/cli` and `internal/phase`
- if a new binary is added, keep it thin and reuse internal packages
