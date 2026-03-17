# pkg

Scope: public package surface intended for external reuse.

Current packages:

- `helper`: lightweight instrumentation helpers that attach run context from env vars
- `sdk`: public wrappers for Kubernetes render and submit helpers

Rules:

- preserve API stability more carefully than `internal/`
- avoid leaking internal package structure through public APIs
