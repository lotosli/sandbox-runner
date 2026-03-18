package examples

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type runSummary struct {
	Status      model.RunStatus   `json:"status"`
	Phase       model.Phase       `json:"phase"`
	ExitCode    int               `json:"exit_code"`
	ArtifactDir string            `json:"artifact_dir"`
	Files       map[string]string `json:"files"`
	StdoutTail  []string          `json:"stdout_tail"`
	StderrTail  []string          `json:"stderr_tail"`
	ReadOrder   []string          `json:"suggested_read_order"`
}

func TestLocalLanguageSamples(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("example workspaces use POSIX shell scripts")
	}

	root := repoRoot(t)
	binary := buildBinary(t, root)
	policyPath := filepath.Join(root, "configs", "policy.sample.yaml")

	samples := []struct {
		name          string
		dir           string
		requiredTools []string
		stdout        []string
		stderr        []string
	}{
		{
			name:          "go",
			dir:           filepath.Join(root, "examples", "go-basic"),
			requiredTools: []string{"go"},
			stdout:        []string{"__GO_EXECUTE__", "__GO_VERIFY__", "RUN_ID=", "SANDBOX_ID="},
			stderr:        []string{"__GO_STDERR__"},
		},
		{
			name:          "python",
			dir:           filepath.Join(root, "examples", "python-basic"),
			requiredTools: []string{"python3"},
			stdout:        []string{"__PYTHON_EXECUTE__", "__PYTHON_VERIFY__", "WRAPPED=1"},
			stderr:        []string{"__PYTHON_STDERR__"},
		},
		{
			name:          "node",
			dir:           filepath.Join(root, "examples", "node-basic"),
			requiredTools: []string{"node", "npm"},
			stdout:        []string{"__NODE_EXECUTE__", "__NODE_VERIFY__", "NODE_OTEL=1"},
			stderr:        []string{"__NODE_STDERR__"},
		},
		{
			name:          "java",
			dir:           filepath.Join(root, "examples", "java-basic"),
			requiredTools: []string{"java", "javac"},
			stdout:        []string{"__JAVA_EXECUTE__", "__JAVA_VERIFY__", "JAVA_TOOL_OPTIONS_SEEN=-javaagent:/opt/otel/opentelemetry-javaagent.jar"},
			stderr:        []string{"__JAVA_STDERR__", "sample mvn shim: skipping dependency resolution"},
		},
		{
			name:          "shell",
			dir:           filepath.Join(root, "examples", "shell-basic"),
			requiredTools: []string{"sh"},
			stdout:        []string{"__SHELL_EXECUTE__", "__SHELL_VERIFY__", "OTEL_SERVICE_NAME=sandbox-runner-shell-sample", "PROOF_OK=1"},
			stderr:        []string{"__SHELL_STDERR__"},
		},
	}

	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			requireTools(t, sample.requiredTools...)

			workDir := t.TempDir()
			copiedDir := filepath.Join(workDir, filepath.Base(sample.dir))
			copyTree(t, sample.dir, copiedDir)
			configPath := filepath.Join(copiedDir, "run.local.sample.yaml")

			runCommand(t, root, binary, "validate", "--config", configPath, "--policy", policyPath)
			runOutput := runCommand(t, root, binary, "run", "--json-summary", "--config", configPath, "--policy", policyPath)

			var summary runSummary
			if err := json.Unmarshal([]byte(runOutput), &summary); err != nil {
				t.Fatalf("Unmarshal(run summary) error = %v\npayload=%s", err, runOutput)
			}
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
			for _, marker := range sample.stdout {
				assertContainsLine(t, stdoutLines, marker)
			}
			for _, marker := range sample.stderr {
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
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
}

func buildBinary(t *testing.T, root string) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "sandbox-runner")
	runCommandWithTimeout(t, root, 5*time.Minute, "go", "build", "-trimpath", "-o", binaryPath, "./cmd/sandbox-runner")
	return binaryPath
}

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping because %s is unavailable", tool)
		}
	}
}

func runCommand(t *testing.T, dir string, binary string, args ...string) string {
	t.Helper()
	return runCommandWithTimeout(t, dir, 5*time.Minute, binary, args...)
}

func runCommandWithTimeout(t *testing.T, dir string, timeout time.Duration, binary string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %s failed: %v\n%s", binary, strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	skipDirs := map[string]struct{}{
		".sandbox-runner": {},
		".venv":           {},
		"node_modules":    {},
		"out":             {},
	}
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if _, skip := skipDirs[info.Name()]; skip && path != src {
				return filepath.SkipDir
			}
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	}); err != nil {
		t.Fatalf("copyTree(%s, %s) error = %v", src, dst, err)
	}
}

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return value
}

func readStructuredLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	lines := []string{}
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var log model.StructuredLog
		if err := json.Unmarshal([]byte(raw), &log); err != nil {
			t.Fatalf("Unmarshal(structured log) error = %v", err)
		}
		lines = append(lines, log.Line)
	}
	return lines
}

func assertContainsLine(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Fatalf("lines %#v do not contain %q", lines, want)
}
