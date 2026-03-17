package backend

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var defaultWorkspaceExcludes = map[string]struct{}{
	".git":            {},
	"node_modules":    {},
	".venv":           {},
	"dist":            {},
	".artifacts":      {},
	".sandbox-runner": {},
	".DS_Store":       {},
}

func (b *OpenSandboxBackend) syncInTar(ctx context.Context, sandboxID string, localDir string, remoteDir string) error {
	tarPath, err := archiveDirectory(localDir)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)

	remoteTar := remoteTempPath(remoteDir, "workspace-in.tar")
	if err := b.client.MakeDirs(ctx, sandboxID, map[string]int{
		path.Dir(remoteTar): 0o755,
		remoteDir:           0o755,
	}); err != nil {
		return err
	}
	if err := b.client.UploadFile(ctx, sandboxID, tarPath, remoteTar); err != nil {
		return err
	}
	command := fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s && rm -f %s",
		shellQuote(remoteDir),
		shellQuote(remoteTar),
		shellQuote(remoteDir),
		shellQuote(remoteTar),
	)
	exitCode, stderr, err := b.RunSimpleCommand(ctx, sandboxID, command, remoteDir, nil, 2*time.Minute)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("extract uploaded workspace failed with code %d: %s", exitCode, stderr)
	}
	return nil
}

func (b *OpenSandboxBackend) syncOutTar(ctx context.Context, sandboxID string, remoteDir string, localDir string) error {
	exists, err := b.remotePathExists(ctx, sandboxID, remoteDir)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	tarPath := remoteTempPath(b.cfg.OpenSandbox.WorkspaceRoot, "workspace-out.tar")
	command := fmt.Sprintf("tar -cf %s -C %s .", shellQuote(tarPath), shellQuote(remoteDir))
	exitCode, stderr, err := b.RunSimpleCommand(ctx, sandboxID, command, b.cfg.OpenSandbox.WorkspaceRoot, nil, 2*time.Minute)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("archive remote workspace failed with code %d: %s", exitCode, stderr)
	}

	localTar, err := os.CreateTemp("", "sandbox-runner-out-*.tar")
	if err != nil {
		return err
	}
	localTarPath := localTar.Name()
	localTar.Close()
	defer os.Remove(localTarPath)

	if err := b.client.DownloadFile(ctx, sandboxID, tarPath, localTarPath); err != nil {
		return err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	if err := extractTar(localTarPath, localDir); err != nil {
		return err
	}

	_, _, _ = b.RunSimpleCommand(ctx, sandboxID, "rm -f "+shellQuote(tarPath), b.cfg.OpenSandbox.WorkspaceRoot, nil, 15*time.Second)
	return nil
}

func (b *OpenSandboxBackend) remotePathExists(ctx context.Context, sandboxID string, remotePath string) (bool, error) {
	command := "test -e " + shellQuote(remotePath)
	exitCode, _, err := b.RunSimpleCommand(ctx, sandboxID, command, b.cfg.OpenSandbox.WorkspaceRoot, nil, 15*time.Second)
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}

func archiveDirectory(root string) (string, error) {
	file, err := os.CreateTemp("", "sandbox-runner-in-*.tar")
	if err != nil {
		return "", err
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	err = filepath.Walk(root, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if shouldExclude(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		src, err := os.Open(current)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return "", err
	}
	return file.Name(), nil
}

func extractTar(tarPath string, dest string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Clean(header.Name))
		cleanDest := filepath.Clean(dest)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive path: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func shouldExclude(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		if _, ok := defaultWorkspaceExcludes[part]; ok {
			return true
		}
	}
	return false
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
