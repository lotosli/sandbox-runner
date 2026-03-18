import json
import os
import pathlib
import sys


def write_proof(phase: str) -> None:
    proof_path = pathlib.Path(".sandbox-runner") / "artifacts" / "proof.json"
    proof_path.parent.mkdir(parents=True, exist_ok=True)
    proof_path.write_text(
        json.dumps(
            {
                "language": "python",
                "phase": phase,
                "wrapped": os.getenv("SAMPLE_PYTHON_OTEL_WRAPPED", ""),
                "otel_service_name": os.getenv("OTEL_SERVICE_NAME", ""),
            },
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )


def main() -> int:
    phase = sys.argv[1] if len(sys.argv) > 1 else "execute"
    proof_path = pathlib.Path(".sandbox-runner") / "artifacts" / "proof.json"
    if phase == "verify" and not proof_path.exists():
        print("missing proof artifact", file=sys.stderr)
        return 1

    write_proof(phase)
    if phase == "execute":
        print("__PYTHON_EXECUTE__")
        print(f"WRAPPED={os.getenv('SAMPLE_PYTHON_OTEL_WRAPPED', '')}")
        print("__PYTHON_STDERR__", file=sys.stderr)
        return 0
    if phase == "verify":
        print("__PYTHON_VERIFY__")
        print(f"OTEL_SERVICE_NAME={os.getenv('OTEL_SERVICE_NAME', '')}")
        return 0

    print(f"unsupported phase {phase}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
