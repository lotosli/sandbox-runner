const fs = require("fs");
const path = require("path");

function writeProof(phase) {
  const proofPath = path.join(".sandbox-runner", "artifacts", "proof.json");
  fs.mkdirSync(path.dirname(proofPath), { recursive: true });
  fs.writeFileSync(
    proofPath,
    JSON.stringify(
      {
        language: "node",
        phase,
        nodeOtel: process.env.SAMPLE_NODE_OTEL_WRAPPED || "",
        otelServiceName: process.env.OTEL_SERVICE_NAME || "",
      },
      null,
      2
    ) + "\n"
  );
}

function main() {
  const phase = process.argv[2] || "execute";
  const proofPath = path.join(".sandbox-runner", "artifacts", "proof.json");
  if (phase === "verify" && !fs.existsSync(proofPath)) {
    console.error("missing proof artifact");
    process.exit(1);
  }

  writeProof(phase);
  if (phase === "execute") {
    console.log("__NODE_EXECUTE__");
    console.log(`NODE_OTEL=${process.env.SAMPLE_NODE_OTEL_WRAPPED || ""}`);
    console.error("__NODE_STDERR__");
    return;
  }
  if (phase === "verify") {
    console.log("__NODE_VERIFY__");
    console.log(`OTEL_SERVICE_NAME=${process.env.OTEL_SERVICE_NAME || ""}`);
    return;
  }

  console.error(`unsupported phase ${phase}`);
  process.exit(2);
}

main();
