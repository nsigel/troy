package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Sandbox is the only way tools touch the world. Paths are relative to the workspace root.
type Sandbox interface {
	Exec(ctx context.Context, command string) (output string, exitCode int, err error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
}

// Local runs everything directly on this machine inside a workspace directory, with usc
// (if installed) using the student's sign-in in USC.
type Local struct {
	Dir     string
	USC     string
	Timeout time.Duration
}

func (l Local) Exec(ctx context.Context, command string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = l.Dir
	cmd.Env = append(os.Environ(), "USC_CONFIG_DIR="+l.USC)
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), -1, fmt.Errorf("timed out after %s", l.Timeout)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), 0, err
}

func (l Local) ReadFile(_ context.Context, path string) ([]byte, error) {
	return readFile(l.Dir, path)
}

func (l Local) WriteFile(_ context.Context, path string, data []byte) error {
	return writeFile(l.Dir, path, data)
}

func readFile(dir, path string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(path)
}

func writeFile(dir, path string, data []byte) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return root.WriteFile(path, data, 0o644)
}
