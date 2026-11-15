package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"

	"troy/model"
	"troy/sandbox"
)

// Tool is something the model can call. It only reaches the world through the sandbox.
type Tool struct {
	Name        string
	Description string
	Schema      string
	Run         func(ctx context.Context, sb sandbox.Sandbox, args json.RawMessage) (string, error)
	RunResult   func(ctx context.Context, sb sandbox.Sandbox, args json.RawMessage) (Result, error)
}

// Code is the default tool set: the model writes one python program per step instead of
// making one call per command. It needs the Docker sandbox, whose image has the troy module.
var Code = []Tool{python, readDocuments}

const maxOutput = 16000

func Defs(list []Tool) []model.ToolDef {
	defs := make([]model.ToolDef, len(list))
	for i, t := range list {
		defs[i] = model.ToolDef{Name: t.Name, Description: t.Description, Schema: json.RawMessage(t.Schema)}
	}
	return defs
}

type Result struct {
	Content   string
	Documents []model.Document
	Failed    bool // the tool returned an error, or its Python code raised
}

// Execute preserves typed attachments separately from bounded textual output.
func Execute(ctx context.Context, list []Tool, sb sandbox.Sandbox, name, args string) Result {
	for _, t := range list {
		if t.Name != name {
			continue
		}
		var result Result
		var err error
		if t.RunResult != nil {
			result, err = t.RunResult(ctx, sb, json.RawMessage(args))
		} else {
			result.Content, err = t.Run(ctx, sb, json.RawMessage(args))
		}
		out := result.Content
		if err != nil {
			result.Failed = true
			out = strings.TrimPrefix(out+"\nerror: "+err.Error(), "\n")
		}
		if len(out) > maxOutput {
			path := "notes/tool-output/" + rand.Text() + ".txt"
			if err := sb.WriteFile(ctx, path, []byte(out)); err != nil {
				out = out[:maxOutput/2] + "\n[output truncated; could not save full output: " + err.Error() + "]\n" + out[len(out)-maxOutput/2:]
			} else {
				out = out[:maxOutput/2] + "\n[output truncated; full output saved at " + path + "]\n" + out[len(out)-maxOutput/2:]
			}
		}
		result.Content = out
		return result
	}
	return Result{Content: "error: unknown tool " + name, Failed: true}
}

var python = Tool{
	Name: "python",
	Description: `Run Python 3 in /workspace; return stdout/stderr. Large output is saved to notes/tool-output/ with a bounded preview and its path.
Variables and imports persist across calls within this task, including after ordinary Python errors. A new user turn or delegated task starts a separate session. Files persist across tasks. Use reset=true to explicitly clear the session. A timeout or process loss requires reset=true; code is never automatically replayed. Python calls execute in order; use parallel inside a call for concurrent I/O.
Available names and contracts:
- usc.<command>(*args, **flags) -> parsed JSON (dict/list/scalar), or text for non-JSON output. Raises usc.Error. Underscores become hyphens; True emits a flag, False/None omit it.
- parallel(*zero_argument_callables) -> list of results in input order. Failed calls return Exception objects; check isinstance(result, Exception). Pass functions, not already-evaluated results.
- text(path: str) -> str: extract PPTX/DOCX or read text. For PDFs use read_documents instead. Raises on failure.
- json: Python JSON module.
Paths: /workspace is the persistent working directory. Activate skills with load_skill; read supporting files with open() using the returned base directory. Keep raw results and scripts under notes/; create directories before writing. /tmp is temporary.
Example: parallel(lambda: usc.brightspace.assignments(123), lambda: usc.brightspace.content(123)) returns [assignment_list, content_dict], or an Exception in either position.
Fetch only what the question needs; reuse results while fresh and refresh when the requested action requires current availability. Batch independent fetches with parallel, then chain dependent fetches and parsing in the same program. Inspect existing variables instead of refetching. Save raw results only when needed beyond this task. Print bounded evidence and source IDs, not entire API responses.`,
	Schema: `{"type":"object","properties":{"code":{"type":"string"},"reset":{"type":"boolean","description":"Discard this task’s Python state and start a fresh interpreter before executing code."}},"required":["code"]}`,
	Run: func(ctx context.Context, sb sandbox.Sandbox, raw json.RawMessage) (string, error) {
		return "", errors.New("Python tool requires an active task session")
	},
}
