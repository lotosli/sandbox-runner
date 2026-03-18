# internal

Scope: all non-public runtime implementation.

Package map:

- `cli`: CLI parsing, subcommand dispatch, request assembly
- `config`: defaults, env overrides, normalization, validation
- `compat`: planned static compatibility matrix for `backend/provider/runtime_profile`
- `capability`: planned backend-specific environment probes before execution
- `model`: enums and shared DTO contracts
- `phase`: run state machine and end-to-end orchestration
- `backend`: sandbox interface plus Dev Container and OpenSandbox providers
- `executor`: direct, Docker, and managed-backend command execution
- `proc`: local process running, streaming, redaction, command classification
- `policy`: filesystem, tool, network, secret, and timeout guardrails
- `platform`: host detection, feature gates, doctor report
- `envsync`: setup-plan generation and environment fingerprinting
- `collector`: collector health check and local bootstrap fallback
- `artifact`: structured JSON and JSONL artifact persistence and upload
- `telemetry`: OTel spans, metrics, and logs
- `kubernetes`: Job and ConfigMap rendering plus submit helpers
- `opensandbox/client`: raw OpenSandbox HTTP transport layer
- `adapter` and `lang`: language-specific command and env rewriting

Rules:

- keep cross-package dependencies flowing toward `phase`, not away from it
- public reuse belongs in `pkg/`, not `internal/`
- when changing shared contracts, update `internal/model` first
- keep provider lifecycle in `backend`, command execution in `executor`, and phase semantics in `phase`
- execution must converge on the standard triple `backend/provider/runtime_profile`
- the required validation order is schema -> compatibility matrix -> capability probe -> RunEngine
- `provider` refines backend behavior; it does not determine backend class
- if a user-visible run concept changes, expect coordinated updates across `model`, `config`, `phase`, `artifact`, `telemetry`, samples, and tests
