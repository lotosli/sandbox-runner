package examples

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/capability"
	"github.com/lotosli/sandbox-runner/internal/config"
	"github.com/lotosli/sandbox-runner/internal/model"
)

type backendLanguageSample struct {
	name   string
	dir    string
	stdout []string
	stderr []string
}

func TestOpenSandboxLanguageSamples(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("example workspaces use POSIX shell scripts")
	}
	if _, err := exec.LookPath("opensandbox-server"); err != nil {
		t.Skipf("opensandbox-server not installed: %v", err)
	}
	if err := dockerReadyForExamples(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	}

	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")
	baseURL := startOpenSandboxFixture(t)

	for _, sample := range backendLanguageSamples(root) {
		t.Run(sample.name, func(t *testing.T) {
			workDir := t.TempDir()
			copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
			copyTree(t, sample.dir, copiedDir)

			configPath := filepath.Join(copiedDir, "run.opensandbox.sample.yaml")
			rewriteConfigLine(t, configPath, "base_url: http://127.0.0.1:8080", "base_url: "+baseURL)

			runCommand(t, root, binary, "validate", "--config", configPath, "--policy", policyPath)
			runOutput := runCommand(t, root, binary, "run", "--json-summary", "--config", configPath, "--policy", policyPath)

			summary := decodeRunSummary(t, runOutput)
			assertSuccessfulSampleRun(t, root, binary, summary, sample.stdout, sample.stderr)
		})
	}
}

func TestDockerLanguageSamples(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("example workspaces use POSIX shell scripts")
	}
	if err := dockerReadyForExamples(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	}

	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")

	for _, sample := range backendLanguageSamples(root) {
		t.Run(sample.name, func(t *testing.T) {
			workDir := t.TempDir()
			copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
			copyTree(t, sample.dir, copiedDir)

			configPath := filepath.Join(copiedDir, "run.docker.sample.yaml")
			runCommand(t, root, binary, "validate", "--config", configPath, "--policy", policyPath)
			runOutput := runCommand(t, root, binary, "run", "--json-summary", "--config", configPath, "--policy", policyPath)

			summary := decodeRunSummary(t, runOutput)
			assertSuccessfulSampleRun(t, root, binary, summary, sample.stdout, sample.stderr)
		})
	}
}

func TestDevContainerLanguageSamples(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("example workspaces use POSIX shell scripts")
	}
	if _, err := exec.LookPath("devcontainer"); err != nil {
		t.Skipf("devcontainer CLI not installed: %v", err)
	}
	if err := dockerReadyForExamples(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	}

	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")

	for _, sample := range backendLanguageSamples(root) {
		t.Run(sample.name, func(t *testing.T) {
			workDir := t.TempDir()
			copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
			copyTree(t, sample.dir, copiedDir)

			configPath := filepath.Join(copiedDir, "run.devcontainer.sample.yaml")
			cfg := loadRunConfigForExample(t, configPath)
			if _, err := capability.Probe(context.Background(), cfg.Execution, cfg); err != nil {
				t.Skipf("devcontainer capability probe failed: %v", err)
			}

			runCommand(t, root, binary, "validate", "--config", configPath, "--policy", policyPath)
			runOutput := runCommand(t, root, binary, "run", "--json-summary", "--config", configPath, "--policy", policyPath)

			summary := decodeRunSummary(t, runOutput)
			assertSuccessfulSampleRun(t, root, binary, summary, sample.stdout, sample.stderr)
		})
	}
}

func TestAppleContainerLanguageSamples(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("apple container integration requires darwin/arm64")
	}
	if _, err := exec.LookPath("container"); err != nil {
		t.Skipf("container CLI not installed: %v", err)
	}

	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")

	for _, sample := range backendLanguageSamples(root) {
		t.Run(sample.name, func(t *testing.T) {
			workDir := t.TempDir()
			copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
			copyTree(t, sample.dir, copiedDir)

			configPath := filepath.Join(copiedDir, "run.apple-container.sample.yaml")
			cfg := loadRunConfigForExample(t, configPath)
			if _, err := capability.Probe(context.Background(), cfg.Execution, cfg); err != nil {
				t.Skipf("apple container capability probe failed: %v", err)
			}

			runCommand(t, root, binary, "validate", "--config", configPath, "--policy", policyPath)
			runOutput := runCommand(t, root, binary, "run", "--json-summary", "--config", configPath, "--policy", policyPath)

			summary := decodeRunSummary(t, runOutput)
			assertSuccessfulSampleRun(t, root, binary, summary, sample.stdout, sample.stderr)
		})
	}
}

func backendLanguageSamples(root string) []backendLanguageSample {
	return []backendLanguageSample{
		{
			name:   "go",
			dir:    filepath.Join(root, "examples", "go-basic"),
			stdout: []string{"__GO_EXECUTE__", "__GO_VERIFY__", "RUN_ID=", "SANDBOX_ID="},
			stderr: []string{"__GO_STDERR__"},
		},
		{
			name:   "python",
			dir:    filepath.Join(root, "examples", "python-basic"),
			stdout: []string{"__PYTHON_EXECUTE__", "__PYTHON_VERIFY__", "WRAPPED=1"},
			stderr: []string{"__PYTHON_STDERR__"},
		},
		{
			name:   "node",
			dir:    filepath.Join(root, "examples", "node-basic"),
			stdout: []string{"__NODE_EXECUTE__", "__NODE_VERIFY__", "NODE_OTEL=1"},
			stderr: []string{"__NODE_STDERR__"},
		},
		{
			name:   "java",
			dir:    filepath.Join(root, "examples", "java-basic"),
			stdout: []string{"__JAVA_EXECUTE__", "__JAVA_VERIFY__", "JAVA_TOOL_OPTIONS_SEEN=-javaagent:/opt/otel/opentelemetry-javaagent.jar"},
			stderr: []string{"__JAVA_STDERR__", "sample mvn shim: skipping dependency resolution"},
		},
		{
			name:   "shell",
			dir:    filepath.Join(root, "examples", "shell-basic"),
			stdout: []string{"__SHELL_EXECUTE__", "__SHELL_VERIFY__", "OTEL_SERVICE_NAME=sandbox-runner-shell-", "PROOF_OK=1"},
			stderr: []string{"__SHELL_STDERR__"},
		},
	}
}

func decodeRunSummary(t *testing.T, payload string) runSummary {
	t.Helper()
	var summary runSummary
	if err := json.Unmarshal([]byte(payload), &summary); err != nil {
		t.Fatalf("Unmarshal(run summary) error = %v\npayload=%s", err, payload)
	}
	return summary
}

func assertSuccessfulSampleRun(t *testing.T, root string, binary string, summary runSummary, stdoutMarkers []string, stderrMarkers []string) {
	t.Helper()
	if summary.Status != model.StatusSucceeded {
		t.Fatalf("status = %s, want %s", summary.Status, model.StatusSucceeded)
	}
	if summary.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", summary.ExitCode)
	}
	if len(summary.ReadOrder) == 0 || summary.ReadOrder[0] != "index" {
		t.Fatalf("read order = %#v, want index first", summary.ReadOrder)
	}

	index := readJSON[model.ArtifactIndex](t, summary.Files["index"])
	if index.Status != model.StatusSucceeded {
		t.Fatalf("index.status = %s, want %s", index.Status, model.StatusSucceeded)
	}
	if index.Files["results"] != "results.json" {
		t.Fatalf("index.files[results] = %q, want results.json", index.Files["results"])
	}

	results := readJSON[model.RunResult](t, summary.Files["results"])
	if results.Status != model.StatusSucceeded {
		t.Fatalf("results.status = %s, want %s", results.Status, model.StatusSucceeded)
	}
	if results.Phase != model.PhaseCollect {
		t.Fatalf("results.phase = %s, want %s", results.Phase, model.PhaseCollect)
	}

	phases := readJSON[[]model.PhaseResult](t, summary.Files["phases"])
	if len(phases) != 5 {
		t.Fatalf("len(phases) = %d, want 5", len(phases))
	}
	for _, phase := range phases {
		if phase.Status != model.StatusSucceeded {
			t.Fatalf("phase %s status = %s, want %s", phase.Phase, phase.Status, model.StatusSucceeded)
		}
	}

	stdoutLines := append(readStructuredLogLines(t, summary.Files["stdout"]), summary.StdoutTail...)
	stderrLines := append(readStructuredLogLines(t, summary.Files["stderr"]), summary.StderrTail...)
	for _, marker := range stdoutMarkers {
		assertContainsLine(t, stdoutLines, marker)
	}
	for _, marker := range stderrMarkers {
		assertContainsLine(t, stderrLines, marker)
	}

	proofPath := filepath.Join(summary.ArtifactDir, "artifacts", "proof.json")
	if _, err := os.Stat(proofPath); err != nil {
		t.Fatalf("proof artifact missing: %v", err)
	}

	replayOutput := runCommand(t, root, binary, "replay", "--artifact-dir", summary.ArtifactDir)
	var replay model.ReplayManifest
	if err := json.Unmarshal([]byte(replayOutput), &replay); err != nil {
		t.Fatalf("Unmarshal(replay) error = %v\npayload=%s", err, replayOutput)
	}
	foundProof := false
	for _, output := range replay.ExpectedOutputs {
		if output == "artifacts/proof.json" {
			foundProof = true
			break
		}
	}
	if !foundProof {
		t.Fatalf("replay expected outputs = %#v, want artifacts/proof.json", replay.ExpectedOutputs)
	}
}

func loadRunConfigForExample(t *testing.T, path string) model.RunConfig {
	t.Helper()
	cfg, err := config.LoadRunConfig(path)
	if err != nil {
		t.Fatalf("LoadRunConfig(%s) error = %v", path, err)
	}
	return cfg
}

func dockerReadyForExamples() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run()
}

func isOpenSandboxHostProxyLimitation(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "Could not connect to the backend sandbox endpoint") ||
		strings.Contains(message, "NETWORK_MODE_ENDPOINT_UNAVAILABLE") ||
		strings.Contains(message, "has no assigned IP address")
}

func startOpenSandboxFixture(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sandbox.toml")
	if output, err := exec.Command("opensandbox-server", "init-config", configPath, "--example", "docker", "--force").CombinedOutput(); err != nil {
		t.Fatalf("init-config failed: %v\n%s", err, output)
	}

	port, err := freeTCPPortForExamples()
	if err != nil {
		t.Fatalf("freeTCPPort() error = %v", err)
	}
	rewriteConfigLine(t, configPath, "port = 8080", fmt.Sprintf("port = %d", port))

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command("opensandbox-server", "--config", configPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start opensandbox-server: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	if err := waitForOpenSandboxFixture(baseURL, 20*time.Second); err != nil {
		t.Fatalf("opensandbox-server did not become ready: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return baseURL
}

func rewriteConfigLine(t *testing.T, path string, old string, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	updated := strings.Replace(string(data), old, new, 1)
	if updated == string(data) {
		t.Fatalf("rewriteConfigLine(%s) did not find %q", path, old)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func freeTCPPortForExamples() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForOpenSandboxFixture(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/v1/sandboxes?page=1&pageSize=1")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", baseURL)
}
