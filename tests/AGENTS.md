# tests

Scope: higher-level test suites outside unit tests colocated under `internal/`.

Current layout:

- `contract/`: stable behavior guarantees for backend-facing contracts; currently centered on OpenSandbox backend behavior
- `integration/`: environment-dependent end-to-end tests; currently OpenSandbox integration coverage with external dependencies

Rules:

- keep environment assumptions explicit
- prefer colocated unit tests for narrow package behavior and use this tree for cross-package or external-system coverage
- if external services or binaries are required, keep skip behavior explicit
