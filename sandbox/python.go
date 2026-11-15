package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Python is one live interpreter, owned by a single agent task. No code is replayed.
type Python struct {
	cmd     *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
	kill    func()
	timeout time.Duration
	once    sync.Once
}

type pythonStarter interface {
	StartPython(context.Context) (*Python, error)
}

func OpenPython(ctx context.Context, sb Sandbox) (*Python, error) {
	starter, ok := sb.(pythonStarter)
	if !ok {
		return nil, fmt.Errorf("sandbox does not support Python sessions")
	}
	return starter.StartPython(ctx)
}

func startPython(ctx context.Context, cmd *exec.Cmd, timeout time.Duration, kill func(int)) (*Python, error) {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	// Startup failures are reported without mixing stderr into the framed protocol.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		return nil, err
	}
	p := &Python{cmd: cmd, input: input, output: bufio.NewReader(output), timeout: timeout}
	p.kill = func() { cmd.Process.Kill() }
	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	line, err := p.read(readyCtx)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("start Python session: %w", err)
	}
	var ready struct {
		PID int `json:"pid"`
	}
	if err = json.Unmarshal(line, &ready); err != nil || ready.PID <= 0 {
		p.Close()
		return nil, fmt.Errorf("invalid Python session handshake")
	}
	p.kill = func() { kill(ready.PID); cmd.Process.Kill() }
	return p, nil
}

func (p *Python) read(ctx context.Context) ([]byte, error) {
	type response struct {
		data []byte
		err  error
	}
	done := make(chan response, 1)
	go func() { b, e := p.output.ReadBytes('\n'); done <- response{b, e} }()
	select {
	case r := <-done:
		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run executes code and returns its output; raised reports that the code raised an exception,
// which leaves the session usable.
func (p *Python) Run(ctx context.Context, code string) (output string, raised bool, err error) {
	if p.timeout <= 0 {
		p.timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	request, _ := json.Marshal(map[string]string{"code": code})
	// Writes also need cancellation: a large cell can fill a broken worker's pipe.
	done := make(chan error, 1)
	go func() { _, err := p.input.Write(append(request, '\n')); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			p.Close()
			return "", false, fmt.Errorf("Python session lost: %w; variables are gone; use reset=true to start fresh", err)
		}
	case <-ctx.Done():
		p.Close()
		return "", false, fmt.Errorf("Python session stopped: %w; variables are gone; use reset=true to start fresh", ctx.Err())
	}
	line, err := p.read(ctx)
	if err != nil {
		p.Close()
		return "", false, fmt.Errorf("Python session lost: %w; variables are gone; use reset=true to start fresh; prior code may have partially executed", err)
	}
	var result struct {
		Output string `json:"output"`
		OK     bool   `json:"ok"`
	}
	if err = json.Unmarshal(line, &result); err != nil {
		p.Close()
		return "", false, fmt.Errorf("Python session protocol failed; variables are gone; use reset=true to start fresh")
	}
	if !result.OK {
		return result.Output + "\n[Python error; variables assigned before the error remain available.]", true, nil
	}
	if result.Output == "" {
		return "(no output)", false, nil
	}
	return result.Output, false, nil
}

func (p *Python) Close() {
	p.once.Do(func() {
		p.input.Close()
		p.kill()
		p.cmd.Wait()
	})
}

func (l Local) StartPython(ctx context.Context) (*Python, error) {
	cmd := exec.Command("python3", "-u", "-m", "troy", "--session")
	cmd.Dir = l.Dir
	cmd.Env = append(os.Environ(), "USC_CONFIG_DIR="+l.USC)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return startPython(ctx, cmd, l.Timeout, func(pid int) { syscall.Kill(-pid, syscall.SIGKILL) })
}

func (d Docker) StartPython(ctx context.Context) (*Python, error) {
	if err := d.start(ctx); err != nil {
		return nil, err
	}
	cmd := exec.Command("docker", "exec", "-i", d.Name, "setsid", "--wait", "python3", "-u", "-m", "troy", "--session")
	return startPython(ctx, cmd, d.Timeout, func(pid int) {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Kill the interpreter's process group, including in-flight USC/subprocess work.
		exec.CommandContext(cleanup, "docker", "exec", d.Name, "python3", "-c",
			"import os,signal; os.killpg("+strconv.Itoa(pid)+",signal.SIGKILL)").Run()
	})
}
