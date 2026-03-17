package golang

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lotosli/sandbox-runner/internal/model"
)

type Adapter struct{}

func (Adapter) Detect(cmd []string, env map[string]string) bool {
	_ = env
	if len(cmd) == 0 {
		return false
	}
	base := filepath.Base(cmd[0])
	if base == "go" || base == "air" || base == "dlv" {
		return true
	}
	return strings.HasPrefix(base, ".") || strings.HasPrefix(base, "/")
}

func (Adapter) Rewrite(cmd []string, env map[string]string, cfg model.RunConfig) ([]string, map[string]string, error) {
	out := make(map[string]string, len(env)+8)
	for k, v := range env {
		out[k] = v
	}
	out["RUN_ID"] = cfg.Run.RunID
	out["ATTEMPT"] = strconv.Itoa(cfg.Run.Attempt)
	out["SANDBOX_ID"] = cfg.Run.SandboxID
	for k, v := range cfg.Go.ExtraEnv {
		out[k] = v
	}
	return cmd, out, nil
}

func (Adapter) Name() string { return "go" }
