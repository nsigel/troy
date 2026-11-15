package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"troy/model"
	"troy/sandbox"
	"troy/tools"
)

const childSteps = 12

const childPrefix = "Task from Troy. Scratch directory: /tmp/task-%d/.\n\n"

var errSteps = errors.New("out of steps")

// WithoutDelegate leaves out the delegate tool.
func WithoutDelegate(a *Agent) { a.noDelegate = true }

// ChildModel runs delegated tasks on m instead of the agent's model.
func ChildModel(m model.Model) Option { return func(a *Agent) { a.childModel = m } }

// OnDelegate calls f after each delegated task, finished or failed.
func OnDelegate(f func(Delegated)) Option { return func(a *Agent) { a.onDelegate = f } }

// Delegated is one delegate call: what it was asked, what it answered, and its whole session.
type Delegated struct {
	Brief, Result string
	Err           error
	Messages      []model.Message
	Start, End    time.Time
}

func (a *Agent) delegate() tools.Tool {
	return tools.Tool{
		Name: "delegate",
		Description: "Hand a sub-task to a fresh agent with your python tool, skills and workspace but none of this chat; " +
			"you get back only its final reply, with source IDs or paths and any missing coverage. The brief must be self-contained: the goal, ids, timeframe, and what to return. " +
			"Several delegate calls in one step run in parallel. Use it for independent sub-tasks that each take several steps, " +
			"like digging through a long document while you do the rest. Handle tasks yourself when one or two Python calls suffice.",
		Schema: `{"type":"object","properties":{"brief":{"type":"string"}},"required":["brief"]}`,
		Run: func(ctx context.Context, sb sandbox.Sandbox, raw json.RawMessage) (string, error) {
			var args struct{ Brief string }
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Brief) == "" {
				return "", errors.New("empty brief")
			}
			d := Delegated{Brief: args.Brief, Start: time.Now()}
			prompt, err := a.prompt(ctx, true, a.workTools())
			if err == nil {
				session := &Session{}
				brief := fmt.Sprintf(childPrefix, a.tasks.Add(1)) + args.Brief
				d.Result, err = loop(ctx, a.childModel, sb, nil, prompt.Messages, session, brief, a.workTools(), childSteps)
				if errors.Is(err, errSteps) {
					d.Result = a.salvage(ctx, prompt.Messages, session)
				}
				d.Messages = session.Messages
			}
			d.Err, d.End = err, time.Now()
			if a.onDelegate != nil {
				a.onDelegate(d)
			}
			return d.Result, err
		},
	}
}

func (a *Agent) salvage(ctx context.Context, prefix []model.Message, session *Session) string {
	session.Append(model.Message{Role: "user", Content: noticePrefix + "You're out of steps. Reply now with what you found and what you couldn't find."})
	recent, err := history(ctx, a.sandbox, session.Messages)
	if err != nil {
		return ""
	}
	msgs, err := hydrateDocuments(ctx, a.sandbox, append(append([]model.Message{}, prefix...), recent...))
	if err != nil {
		return ""
	}
	msg, err := a.childModel.Chat(ctx, msgs, nil, nil)
	if err != nil {
		return ""
	}
	session.Append(msg)
	return msg.Content
}
