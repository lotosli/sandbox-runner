# docs

Scope: repository-facing documentation and navigation pages.

Rules:

- keep [`README.md`](../README.md) as the concise project entrypoint, not the full manual
- move detailed explanations into focused files under `docs/` and keep README links current
- when build or artifact behavior changes, update [`docs/build-and-release.md`](./build-and-release.md)
- when architecture or execution semantics change, update [`docs/architecture.md`](./architecture.md) and [`docs/execution-model.md`](./execution-model.md)
- when backend behavior or platform caveats change, update [`docs/backends-and-platforms.md`](./backends-and-platforms.md)
- when examples, validation commands, or sample configs change, update [`docs/getting-started.md`](./getting-started.md)
