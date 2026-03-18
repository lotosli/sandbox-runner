package artifact

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/lotosli/sandbox-runner/internal/model"
)

const (
	ContextFileName        = "context.json"
	EnvironmentFileName    = "environment.json"
	SetupPlanFileName      = "setup.plan.json"
	PhasesFileName         = "phases.json"
	CommandsFileName       = "commands.jsonl"
	StdoutFileName         = "stdout.jsonl"
	StderrFileName         = "stderr.jsonl"
	ResultsFileName        = "results.json"
	ReplayFileName         = "replay.json"
	IndexFileName          = "index.json"
	ArtifactsDirName       = "artifacts"
	ProviderFileName       = "provider.json"
	BackendProfileFileName = "backend-profile.json"
	MachineFileName        = "machine.json"
	ContainerFileName      = "container.json"
	SandboxFileName        = "sandbox.json"
	EndpointsFileName      = "endpoints.json"
	RuntimeFileName        = "runtime.json"
	DevContainerFileName   = "devcontainer.json"
)

type Writer struct {
	root             string
	maxArtifactBytes int64
	mu               sync.Mutex
	jsonlFiles       map[string]*jsonlFile
}

type jsonlFile struct {
	file *os.File
	w    *bufio.Writer
}

func NewWriter(root string, maxArtifactBytes int64) (*Writer, error) {
	if err := os.MkdirAll(filepath.Join(root, ArtifactsDirName), 0o755); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	return &Writer{
		root:             root,
		maxArtifactBytes: maxArtifactBytes,
		jsonlFiles:       map[string]*jsonlFile{},
	}, nil
}

func (w *Writer) Root() string { return w.root }

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var firstErr error
	for name, entry := range w.jsonlFiles {
		if err := entry.w.Flush(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("flush %s: %w", name, err)
		}
		if err := entry.file.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close %s: %w", name, err)
		}
	}
	return firstErr
}

func (w *Writer) WriteJSON(name string, value any) error {
	path := filepath.Join(w.root, name)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (w *Writer) AppendJSONL(name string, value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, err := w.getJSONLFile(name)
	if err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := entry.w.Write(append(data, '\n')); err != nil {
		return err
	}
	return entry.w.Flush()
}

func (w *Writer) WriteContext(ctx model.ContextArtifact) error {
	return w.WriteJSON(ContextFileName, ctx)
}

func (w *Writer) WriteEnvironment(value model.EnvironmentFingerprint) error {
	return w.WriteJSON(EnvironmentFileName, value)
}

func (w *Writer) WriteSetupPlan(value model.SetupPlan) error {
	return w.WriteJSON(SetupPlanFileName, value)
}

func (w *Writer) WritePhases(phases []model.PhaseResult) error {
	return w.WriteJSON(PhasesFileName, phases)
}

func (w *Writer) WriteResults(result *model.RunResult) error {
	return w.WriteJSON(ResultsFileName, result)
}

func (w *Writer) WriteReplay(replay model.ReplayManifest) error {
	return w.WriteJSON(ReplayFileName, replay)
}

func (w *Writer) WriteIndex(index model.ArtifactIndex) error {
	return w.WriteJSON(IndexFileName, index)
}

func (w *Writer) WriteProvider(value model.ProviderArtifact) error {
	return w.WriteJSON(ProviderFileName, value)
}

func (w *Writer) WriteBackendProfile(value model.BackendProfileArtifact) error {
	return w.WriteJSON(BackendProfileFileName, value)
}

func (w *Writer) WriteMachine(value model.MachineArtifact) error {
	return w.WriteJSON(MachineFileName, value)
}

func (w *Writer) WriteContainer(value model.ContainerArtifact) error {
	return w.WriteJSON(ContainerFileName, value)
}

func (w *Writer) WriteSandbox(value model.SandboxArtifact) error {
	return w.WriteJSON(SandboxFileName, value)
}

func (w *Writer) WriteEndpoints(value model.EndpointsArtifact) error {
	return w.WriteJSON(EndpointsFileName, value)
}

func (w *Writer) WriteRuntime(value model.RuntimeArtifact) error {
	return w.WriteJSON(RuntimeFileName, value)
}

func (w *Writer) WriteDevContainer(value model.DevContainerArtifact) error {
	return w.WriteJSON(DevContainerFileName, value)
}

func (w *Writer) AppendCommand(value any) error {
	return w.AppendJSONL(CommandsFileName, value)
}

func (w *Writer) AppendStdout(log model.StructuredLog) error {
	return w.AppendJSONL(StdoutFileName, log)
}

func (w *Writer) AppendStderr(log model.StructuredLog) error {
	return w.AppendJSONL(StderrFileName, log)
}

func (w *Writer) ArtifactRefs() ([]model.ArtifactRef, error) {
	entries := []model.ArtifactRef{}
	err := filepath.Walk(w.root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(w.root, path)
		if err != nil {
			return err
		}
		entries = append(entries, model.ArtifactRef{
			Name:      filepath.Base(path),
			Path:      rel,
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (w *Writer) WriteArtifact(name string, data []byte) (string, error) {
	target := filepath.Join(w.root, ArtifactsDirName, name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if w.maxArtifactBytes > 0 && int64(len(data)) > w.maxArtifactBytes {
		return target, fmt.Errorf("artifact %s exceeds max bytes", name)
	}
	return target, os.WriteFile(target, data, 0o644)
}

func (w *Writer) getJSONLFile(name string) (*jsonlFile, error) {
	if entry, ok := w.jsonlFiles[name]; ok {
		return entry, nil
	}
	path := filepath.Join(w.root, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	entry := &jsonlFile{file: file, w: bufio.NewWriter(file)}
	w.jsonlFiles[name] = entry
	return entry, nil
}
