package sample;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;

public final class Main {
  private Main() {}

  public static void main(String[] args) throws IOException {
    String phase = args.length > 0 ? args[0] : "execute";
    Path proof = Path.of(".sandbox-runner", "artifacts", "proof.json");
    if ("verify".equals(phase) && !Files.exists(proof)) {
      System.err.println("missing proof artifact");
      System.exit(1);
    }

    Files.createDirectories(proof.getParent());
    String payload =
        "{\n"
            + "  \"language\": \"java\",\n"
            + "  \"phase\": \""
            + phase
            + "\",\n"
            + "  \"java_tool_options_seen\": \""
            + System.getenv().getOrDefault("SAMPLE_JAVA_TOOL_OPTIONS", "")
            + "\"\n"
            + "}\n";
    Files.writeString(proof, payload);

    if ("execute".equals(phase)) {
      System.out.println("__JAVA_EXECUTE__");
      System.out.println(
          "JAVA_TOOL_OPTIONS_SEEN="
              + System.getenv().getOrDefault("SAMPLE_JAVA_TOOL_OPTIONS", ""));
      System.err.println("__JAVA_STDERR__");
      return;
    }
    if ("verify".equals(phase)) {
      System.out.println("__JAVA_VERIFY__");
      System.out.println(
          "OTEL_SERVICE_NAME=" + System.getenv().getOrDefault("OTEL_SERVICE_NAME", ""));
      return;
    }

    System.err.println("unsupported phase " + phase);
    System.exit(2);
  }
}
