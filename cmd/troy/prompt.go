package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"troy/agent"
	"troy/jobs"
	"troy/sandbox"
)

// inspectPrompt renders the same prefix/tool definitions as a live agent without API calls or account access.
func inspectPrompt(mode string) error {
	if mode != "default" && mode != "code" && mode != "child" {
		return fmt.Errorf("unknown prompt mode %q", mode)
	}
	dir, err := os.MkdirTemp("", "troy-prompt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	sb := sandbox.Local{Dir: dir, Timeout: time.Minute}
	child := mode == "child"
	policy, err := uscPrompt()
	if err != nil {
		return err
	}
	// Build an agent to get the actual registered tool set, not a parallel list maintained for inspection.
	extra := func(context.Context) agent.Context {
		return agent.Context{Instructions: policy, Student: "Synthetic inspection account; USC not connected. No registered course data."}
	}
	runner, err := jobs.Load(dir, func(string) (sandbox.Sandbox, error) { return sb, nil })
	if err != nil {
		return err
	}
	opts := []agent.Option{agent.WithTools(runner.Tools("preview", "")...)}
	if mode == "code" {
		opts = append(opts, agent.WithoutDelegate)
	}
	a, err := agent.New(context.Background(), nil, sb, &agent.Session{}, extra, nil, nil, opts...)
	if err != nil {
		return err
	}
	defer a.Close()
	p, err := a.Inspect(context.Background(), child)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
