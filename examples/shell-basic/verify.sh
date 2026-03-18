#!/bin/sh
set -eu

proof_path=".sandbox-runner/artifacts/proof.json"
if [ ! -f "$proof_path" ]; then
  printf 'missing proof artifact\n' >&2
  exit 1
fi

printf '{\n  "language": "shell",\n  "phase": "verify",\n  "otel_service_name": "%s"\n}\n' "${OTEL_SERVICE_NAME:-}" > "$proof_path"
printf '__SHELL_VERIFY__\n'
printf 'PROOF_OK=1\n'
