package envsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type Planner struct{}

func NewPlanner() Planner { return Planner{} }

func (Planner) Plan(ctx context.Context, workspaceDir string) (model.SetupPlan, model.EnvironmentFingerprint, error) {
	projectType, lockfiles := detectProject(workspaceDir)
	plan := model.SetupPlan{
		ProjectType: projectType,
		Runtime:     projectTypeRuntime(projectType),
		Lockfiles:   lockfiles,
		Steps:       defaultSteps(projectType, workspaceDir),
	}
	fingerprint, err := buildFingerprint(ctx, workspaceDir, projectType, lockfiles)
	return plan, fingerprint, err
}

func detectProject(workspaceDir string) (string, []string) {
	candidates := map[string][]string{
		"node":   {"package.json"},
		"python": {"requirements.txt", "pyproject.toml", "poetry.lock"},
		"java":   {"pom.xml", "build.gradle"},
		"go":     {"go.mod"},
	}
	order := []string{"node", "python", "java", "go"}
	for _, kind := range order {
		files := []string{}
		for _, name := range candidates[kind] {
			if _, err := os.Stat(filepath.Join(workspaceDir, name)); err == nil {
				files = append(files, name)
			}
		}
		if len(files) > 0 {
			return kind, files
		}
	}
	return "shell", nil
}

func defaultSteps(projectType, workspaceDir string) []model.SetupStep {
	switch projectType {
	case "python":
		return []model.SetupStep{
			{ID: "venv-create", Cmd: []string{"python", "-m", "venv", ".venv"}},
			{ID: "pip-install", Cmd: []string{filepath.Join(workspaceDir, ".venv", "bin", "pip"), "install", "-r", "requirements.txt"}},
		}
	case "node":
		if _, err := os.Stat(filepath.Join(workspaceDir, "pnpm-lock.yaml")); err == nil {
			return []model.SetupStep{{ID: "pnpm-install", Cmd: []string{"pnpm", "install", "--frozen-lockfile"}}}
		}
		return []model.SetupStep{{ID: "npm-install", Cmd: []string{"npm", "install"}}}
	case "java":
		return []model.SetupStep{{ID: "mvn-deps", Cmd: []string{"mvn", "-q", "-DskipTests", "dependency:resolve"}}}
	case "go":
		return []model.SetupStep{{ID: "go-mod-download", Cmd: []string{"go", "mod", "download"}}}
	default:
		return nil
	}
}

func buildFingerprint(ctx context.Context, workspaceDir, projectType string, lockfiles []string) (model.EnvironmentFingerprint, error) {
	workspaceHash, err := hashFiles(workspaceDir, lockfiles)
	if err != nil {
		return model.EnvironmentFingerprint{}, err
	}
	lockfileHashes := map[string]string{}
	for _, name := range lockfiles {
		hash, err := hashOne(filepath.Join(workspaceDir, name))
		if err == nil {
			lockfileHashes[name] = hash
		}
	}

	runtimeInfo := model.RuntimeInfo{Name: projectType, Version: commandVersion(ctx, versionCommandFor(projectType))}
	pmInfo := model.RuntimeInfo{Name: packageManagerFor(projectType), Version: commandVersion(ctx, packageManagerVersionCommand(projectType))}
	fp := model.EnvironmentFingerprint{
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Runtime:        runtimeInfo,
		PackageManager: pmInfo,
		GitSHA:         gitSHA(ctx, workspaceDir),
		WorkspaceHash:  workspaceHash,
		LockfileHashes: lockfileHashes,
	}
	if projectType == "go" {
		fp.GoEnv = goEnvSummary(ctx, workspaceDir)
	}
	return fp, nil
}

func hashFiles(workspaceDir string, lockfiles []string) (string, error) {
	if len(lockfiles) == 0 {
		return "sha256:empty", nil
	}
	sort.Strings(lockfiles)
	h := sha256.New()
	for _, name := range lockfiles {
		data, err := os.ReadFile(filepath.Join(workspaceDir, name))
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(name))
		_, _ = h.Write(data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func hashOne(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func projectTypeRuntime(projectType string) string {
	switch projectType {
	case "python":
		return "python3"
	case "node":
		return "node"
	case "java":
		return "java"
	case "go":
		return "go"
	default:
		return "shell"
	}
}

func packageManagerFor(projectType string) string {
	switch projectType {
	case "python":
		return "pip"
	case "node":
		return "npm"
	case "java":
		return "mvn"
	case "go":
		return "go"
	default:
		return "shell"
	}
}

func versionCommandFor(projectType string) []string {
	switch projectType {
	case "python":
		return []string{"python", "--version"}
	case "node":
		return []string{"node", "--version"}
	case "java":
		return []string{"java", "-version"}
	case "go":
		return []string{"go", "version"}
	default:
		return nil
	}
}

func packageManagerVersionCommand(projectType string) []string {
	switch projectType {
	case "python":
		return []string{"pip", "--version"}
	case "node":
		return []string{"npm", "--version"}
	case "java":
		return []string{"mvn", "-version"}
	case "go":
		return []string{"go", "version"}
	default:
		return nil
	}
}

func commandVersion(ctx context.Context, cmdline []string) string {
	if len(cmdline) == 0 {
		return ""
	}
	cmd := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
}

func gitSHA(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func goEnvSummary(ctx context.Context, dir string) map[string]string {
	cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOOS", "GOARCH", "GOVERSION", "GOMOD", "GOMODCACHE")
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	_ = json.Unmarshal(data, &out)
	return out
}
