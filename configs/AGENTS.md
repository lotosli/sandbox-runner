# configs

Scope: sample configuration files and local collector config.

Owns:

- runnable sample run configs for supported modes
- sample policy config
- local OTel collector config

Rules:

- treat these files as documentation plus smoke-test fixtures
- keep samples aligned with `internal/model`, `internal/config`, and README
- if a mode is added or renamed, update or add a sample here
- sample naming should be platform-first and user-facing, such as `run.local.sample.yaml`, `run.k3s.sample.yaml`, or `run.k3s.isolated.sample.yaml`
- do not expose `backend`, `provider`, or `runtime_profile` terminology directly in sample file names
- internal semantics must still map cleanly to `backend/provider/runtime_profile`
