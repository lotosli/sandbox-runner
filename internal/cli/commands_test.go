package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lotosli/sandbox-runner/internal/artifact"
	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestFormatRunSummaryIncludesArtifactPathsAndTails(t *testing.T) {
	summary := formatRunSummary(runSummary{
		RunID:       "r-123",
		Status:      model.StatusSucceeded,
		Phase:       model.PhaseCollect,
		ExitCode:    0,
		ArtifactDir: "/tmp/run-artifacts",
		Files:       runSummaryFiles("/tmp/run-artifacts"),
		StdoutTail:  []string{"hello stdout", "done"},
		StderrTail:  []string{"warning: demo"},
	})

	for _, want := range []string{
		"run_id: r-123",
		"status: SUCCEEDED",
		"index: /tmp/run-artifacts/index.json",
		"results: /tmp/run-artifacts/results.json",
		"commands: /tmp/run-artifacts/commands.jsonl",
		"replay: /tmp/run-artifacts/replay.json",
		"stdout: /tmp/run-artifacts/stdout.jsonl",
		"stderr: /tmp/run-artifacts/stderr.jsonl",
		"- hello stdout",
		"- warning: demo",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestReadStructuredLogTailReturnsLatestLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, artifact.StdoutFileName)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	for _, line := range []string{"first", "second", "third"} {
		entry, err := json.Marshal(model.StructuredLog{Line: line})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if _, err := file.Write(append(entry, '\n')); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	got, err := readStructuredLogTail(path, 2)
	if err != nil {
		t.Fatalf("readStructuredLogTail() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "second" || got[1] != "third" {
		t.Fatalf("tail = %#v, want [second third]", got)
	}
}

func TestPrintRunSummaryJSON(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, artifact.StdoutFileName)
	if err := os.WriteFile(stdoutPath, []byte("{\"line\":\"hello\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	printRunSummary(&out, dir, &model.RunResult{
		RunID:          "r-123",
		Status:         model.StatusSucceeded,
		Phase:          model.PhaseCollect,
		ExitCode:       0,
		BackendKind:    "direct",
		ProviderName:   "native",
		RuntimeProfile: "default",
	}, true)

	var summary runSummary
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatalf("Unmarshal() error = %v\npayload=%s", err, out.String())
	}
	if summary.Files["index"] != filepath.Join(dir, artifact.IndexFileName) {
		t.Fatalf("index = %q, want %q", summary.Files["index"], filepath.Join(dir, artifact.IndexFileName))
	}
	if len(summary.StdoutTail) != 1 || summary.StdoutTail[0] != "hello" {
		t.Fatalf("stdout_tail = %#v, want [hello]", summary.StdoutTail)
	}
	if len(summary.ReadOrder) == 0 || summary.ReadOrder[0] != "index" {
		t.Fatalf("read_order = %#v, want index first", summary.ReadOrder)
	}
}

func TestEnsureK8sRequestIdentity(t *testing.T) {
	req := model.RunRequest{}
	got := ensureK8sRequestIdentity(req)
	if got.RunConfig.Run.RunID == "" {
		t.Fatal("run_id = empty, want generated value")
	}
	if !strings.HasPrefix(got.RunConfig.Run.RunID, "r-") {
		t.Fatalf("run_id = %q, want r- prefix", got.RunConfig.Run.RunID)
	}

	req.RunConfig.Run.RunID = "fixed-id"
	got = ensureK8sRequestIdentity(req)
	if got.RunConfig.Run.RunID != "fixed-id" {
		t.Fatalf("run_id = %q, want fixed-id", got.RunConfig.Run.RunID)
	}
}
