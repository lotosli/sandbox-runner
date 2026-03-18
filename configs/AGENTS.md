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
- sample naming should converge on `backend[.provider][.runtime].sample.yaml`
- legacy mixed-layer names may be kept only as compatibility aliases; internal semantics must still map to `backend/provider/runtime_profile`
