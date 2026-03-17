# examples

Scope: example integrations and demonstration apps.

Current layout:

- `go-http-service/`: a minimal Go HTTP service that uses `pkg/helper` for request spans and events

Rules:

- examples should show intended usage, not carry production-only complexity
- keep example code in sync with public APIs exposed from `pkg/`
- do not let example-specific hacks leak back into runtime packages
