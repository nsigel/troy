package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"troy/model"
)

type modelCalls struct {
	mu    sync.Mutex
	calls []modelCall
}

type modelCall struct {
	label      string
	start, end time.Time
	err        error
}

func (c *modelCalls) wrap(m model.Model, label string) model.Model {
	return countingModel{Model: m, label: label, calls: c}
}

func (c *modelCalls) count(label string) (n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.calls {
		if label == "" || call.label == label {
			n++
		}
	}
	return n
}

func (c *modelCalls) log(t0 time.Time) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	for _, call := range c.calls {
		fmt.Fprintf(&b, "- %s %5.1fs to %5.1fs", call.label, call.start.Sub(t0).Seconds(), call.end.Sub(t0).Seconds())
		if call.err != nil {
			fmt.Fprintf(&b, " error: %v", call.err)
		}
		b.WriteString("\n")
	}
	return b.String()
}

type countingModel struct {
	model.Model
	label string
	calls *modelCalls
}

func (m countingModel) Chat(ctx context.Context, msgs []model.Message, tools []model.ToolDef, onText func(string)) (model.Message, error) {
	start := time.Now()
	msg, err := m.Model.Chat(ctx, msgs, tools, onText)
	m.calls.mu.Lock()
	m.calls.calls = append(m.calls.calls, modelCall{m.label, start, time.Now(), err})
	m.calls.mu.Unlock()
	return msg, err
}

const dryRunUSC = `#!/usr/bin/python3
import json, os, sys
WRITES = [["libcal", "book"], ["libcal", "release"], ["mobileorder", "order"],
          ["webreg", "add"], ["webreg", "schedule"], ["webreg", "unschedule"], ["webreg", "remove"],
          ["webreg", "grade-option"], ["webreg", "submit"], ["handshake", "career-fair-register"],
          ["handshake", "career-fair-cancel"], ["bypass"], ["auth"], ["browser"]]
words = [a for a in sys.argv[1:] if not a.startswith("-")]
if any(words[:len(w)] == w for w in WRITES):
    with open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "writes.jsonl"), "a") as f:
        f.write(json.dumps(["usc", *sys.argv[1:]]) + "\n")
    print(json.dumps({"dry_run": True, "command": ["usc", *sys.argv[1:]]}))
else:
    os.execv("/usr/local/bin/usc", ["usc", *sys.argv[1:]])
`

func dryRunBin(dir string) (string, error) {
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		return "", err
	}
	return bin, os.WriteFile(filepath.Join(bin, "usc"), []byte(dryRunUSC), 0o755)
}

func writes(bin string) string {
	b, _ := os.ReadFile(filepath.Join(bin, "writes.jsonl"))
	return strings.TrimSpace(string(b))
}
