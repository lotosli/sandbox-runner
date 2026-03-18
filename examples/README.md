# Language Samples

These examples are runnable workspaces for the built-in language adapters:

- [go-basic](./go-basic/run.local.sample.yaml), [Docker](./go-basic/run.docker.sample.yaml), [DevContainer](./go-basic/run.devcontainer.sample.yaml), [OpenSandbox](./go-basic/run.opensandbox.sample.yaml), [Apple container](./go-basic/run.apple-container.sample.yaml)
- [python-basic](./python-basic/run.local.sample.yaml), [Docker](./python-basic/run.docker.sample.yaml), [DevContainer](./python-basic/run.devcontainer.sample.yaml), [OpenSandbox](./python-basic/run.opensandbox.sample.yaml), [Apple container](./python-basic/run.apple-container.sample.yaml)
- [node-basic](./node-basic/run.local.sample.yaml), [Docker](./node-basic/run.docker.sample.yaml), [DevContainer](./node-basic/run.devcontainer.sample.yaml), [OpenSandbox](./node-basic/run.opensandbox.sample.yaml), [Apple container](./node-basic/run.apple-container.sample.yaml)
- [java-basic](./java-basic/run.local.sample.yaml), [Docker](./java-basic/run.docker.sample.yaml), [DevContainer](./java-basic/run.devcontainer.sample.yaml), [OpenSandbox](./java-basic/run.opensandbox.sample.yaml), [Apple container](./java-basic/run.apple-container.sample.yaml)
- [shell-basic](./shell-basic/run.local.sample.yaml), [Docker](./shell-basic/run.docker.sample.yaml), [DevContainer](./shell-basic/run.devcontainer.sample.yaml), [OpenSandbox](./shell-basic/run.opensandbox.sample.yaml), [Apple container](./shell-basic/run.apple-container.sample.yaml)

All examples:

- keep the language-specific command, verify contract, and `artifacts/proof.json` output stable across backend variants
- run with `collector.mode=skip` so they work without a local collector
- write `artifacts/proof.json` into the run artifact directory
- emit stable stdout and stderr markers so agent harnesses can assert behavior

Run one manually from the repository root:

```bash
./sandbox-runner validate --config examples/go-basic/run.local.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --json-summary --config examples/go-basic/run.local.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner replay --artifact-dir examples/go-basic/.sandbox-runner
```

Managed backend variants use the same pattern:

```bash
./sandbox-runner validate --config examples/go-basic/run.docker.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --json-summary --config examples/go-basic/run.docker.sample.yaml --policy configs/policy.sample.yaml

./sandbox-runner validate --config examples/go-basic/run.devcontainer.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --json-summary --config examples/go-basic/run.devcontainer.sample.yaml --policy configs/policy.sample.yaml

./sandbox-runner validate --config examples/go-basic/run.opensandbox.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --json-summary --config examples/go-basic/run.opensandbox.sample.yaml --policy configs/policy.sample.yaml

./sandbox-runner validate --config examples/go-basic/run.apple-container.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --json-summary --config examples/go-basic/run.apple-container.sample.yaml --policy configs/policy.sample.yaml
```

Backend prerequisites:

- Docker samples expect a working local Docker daemon; the runner now follows the current Docker CLI context when `docker.context=default`
- DevContainer samples expect a working Docker daemon, the local `devcontainer` CLI, and each example's `.devcontainer/devcontainer.json`
- OpenSandbox samples expect a local `opensandbox-server --example docker` endpoint and a working Docker daemon
- Apple container samples expect macOS arm64, `container` CLI installed, and a configured default kernel:
  `container system kernel set --recommended --arch arm64 --force`
  `container system start --disable-kernel-install`

Expected markers by sample:

- Go: `__GO_EXECUTE__`, `__GO_VERIFY__`, `__GO_STDERR__`
- Python: `__PYTHON_EXECUTE__`, `__PYTHON_VERIFY__`, `__PYTHON_STDERR__`, `WRAPPED=1`
- Node: `__NODE_EXECUTE__`, `__NODE_VERIFY__`, `__NODE_STDERR__`, `NODE_OTEL=1`
- Java: `__JAVA_EXECUTE__`, `__JAVA_VERIFY__`, `__JAVA_STDERR__`, `JAVA_TOOL_OPTIONS_SEEN=-javaagent:/opt/otel/opentelemetry-javaagent.jar`
- Shell: `__SHELL_EXECUTE__`, `__SHELL_VERIFY__`, `__SHELL_STDERR__`

The integration test under `tests/integration/examples` copies each workspace to a temp directory before running it, so CI and local `go test` runs do not dirty the repository with `.sandbox-runner`, `.venv`, `node_modules`, or compiled classes.
