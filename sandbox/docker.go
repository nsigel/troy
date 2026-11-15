package sandbox

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

//go:embed Dockerfile *.py recipes
var files embed.FS

//go:embed recipes
var Recipes embed.FS

// uscVersion is the usc-cli version Troy was built with; the image installs the same one, so
// the sandbox's usc matches the skill in Troy's system prompt.
var uscVersion = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/nsigel/usc-cli" {
				return dep.Version
			}
		}
	}
	return "latest"
}()

var image = func() string {
	h := crc32.NewIEEE()
	fs.WalkDir(files, ".", func(path string, _ fs.DirEntry, _ error) error {
		b, _ := files.ReadFile(path)
		h.Write(append([]byte(path), b...))
		return nil
	})
	h.Write([]byte(uscVersion))
	return fmt.Sprintf("troy-sandbox:%08x", h.Sum32())
}()

// Docker runs commands in a per-user container with Dir bind-mounted at /workspace and USC,
// the student's usc sign-in, at /home/troy/.usc. Files are read and written on the host, so
// only Exec needs the container. It is created on first Exec and removes itself after Idle with
// no Exec; the next Exec creates it again. Bin, if set, is mounted at /opt/troy-eval and put
// first on PATH (troy eval's dry-run usc).
type Docker struct {
	Name    string
	Dir     string
	USC     string
	Bin     string
	Timeout time.Duration
	Idle    time.Duration
}

// name changes with anything the container is created from, so a changed mount or image
// gets a new container instead of restarting a stale one.
func (d Docker) Exec(ctx context.Context, command string) (string, int, error) {
	out, code, err := d.exec(ctx, command)
	if code != 0 && !d.running(ctx) {
		if err := d.start(ctx); err != nil {
			return "", -1, err
		}
		out, code, err = d.exec(ctx, command)
	}
	return out, code, err
}

// starting holds a mutex per container name, so an agent's command and a monitor's check that
// both find the container stopped don't remove the one the other just started.
var starting sync.Map

func (d Docker) start(ctx context.Context) error {
	v, _ := starting.LoadOrStore(d.Name, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if d.running(ctx) {
		return nil
	}
	return d.up(ctx)
}

func (d Docker) running(ctx context.Context) bool {
	out, _ := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", d.Name).Output()
	return strings.TrimSpace(string(out)) == "true"
}

func (d Docker) exec(ctx context.Context, command string) (string, int, error) {
	// timeout runs inside the container and signals the whole process group, so the
	// command dies there too. The host-side deadline is only a backstop for the client.
	ctx, cancel := context.WithTimeout(ctx, d.Timeout+10*time.Second)
	defer cancel()
	secs := fmt.Sprint(int(d.Timeout.Seconds()))
	cmd := exec.CommandContext(ctx, "docker", "exec", d.Name, "timeout", "-k", "1", secs, "sh", "-c", "touch /tmp/.troy-last\n"+command)
	cmd.WaitDelay = time.Second
	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil && time.Since(start) >= d.Timeout {
		return string(out), -1, fmt.Errorf("timed out after %s", d.Timeout)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), 0, err
}

func (d Docker) ReadFile(_ context.Context, path string) ([]byte, error) {
	return readFile(d.Dir, path)
}

func (d Docker) WriteFile(_ context.Context, path string, data []byte) error {
	return writeFile(d.Dir, path, data)
}

func (d Docker) up(ctx context.Context) error {
	exec.CommandContext(ctx, "docker", "rm", "-f", d.Name).Run() // one left stopped from before --rm
	if exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() != nil {
		if err := build(ctx); err != nil {
			return err
		}
	}
	dir, err := filepath.Abs(d.Dir)
	if err != nil {
		return err
	}
	usc, err := filepath.Abs(d.USC)
	if err != nil {
		return err
	}
	// The mount needs the directory before the student connects USC. The website then
	// replaces the files inside it, never the directory, so the mount keeps working.
	if err := os.MkdirAll(usc, 0o700); err != nil {
		return err
	}
	var bin []string
	if d.Bin != "" {
		bin = []string{"--mount", "type=bind,source=" + d.Bin + ",target=/opt/troy-eval",
			"--env", "PATH=/opt/troy-eval:/workspace/.local/bin:/usr/local/bin:/usr/bin:/bin"}
	}
	watchdog := fmt.Sprintf(`touch /tmp/.troy-last; while [ $(( $(date +%%s) - $(stat -c %%Y /tmp/.troy-last) )) -lt %d ] || pgrep -x timeout >/dev/null; do sleep 5; done`, int(d.Idle.Seconds()))
	args := append([]string{}, "run", "-d", "--rm", "--name", d.Name, "--init",
		"--user", "troy", "--cap-drop", "ALL", "--memory", "1g", "--cpus", "2", "--pids-limit", "256",
		"--mount", "type=bind,source="+dir+",target=/workspace", "--workdir", "/workspace",
		"--mount", "type=bind,source="+usc+",target=/home/troy/.usc", "--env", "USC_CONFIG_DIR=/home/troy/.usc")
	args = append(append(args, bin...), image, "sh", "-c", watchdog)
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting container: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// build builds the image from the embedded Dockerfile and Python modules.
// It runs go in the current directory, so Troy has to run from its module.
func build(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "troy-sandbox-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.CopyFS(dir, files); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "docker", "build", "-t", image, "--build-arg", "USC_VERSION="+uscVersion, dir).CombinedOutput(); err != nil {
		return fmt.Errorf("building %s: %v\n%s", image, err, out)
	}
	return nil
}
