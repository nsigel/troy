package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"troy/sandbox"
	"troy/tools"
)

// Tools are the agent's handles on user id's jobs, for the conversation chat (a web chat id, or
// "" for the iMessage thread). Schedules come back to the conversation they were set in.
func (r *Runner) Tools(id, chat string) []tools.Tool {
	return []tools.Tool{r.scheduleTool(id, chat), r.monitorTool(id), r.jobsTool(id)}
}

func (r *Runner) add(id, name string, put func(u *userJobs)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := r.user(id)
	u.remove(name)
	put(u)
	return r.save(id)
}

// AddMonitor runs command in id's sandbox once as the baseline and, only if that works, saves
// it as monitor name, replacing any job called that. It returns what the baseline saw, for
// the agent.
func (r *Runner) AddMonitor(ctx context.Context, id, name, command string, every int, prompt string) (string, error) {
	sb, err := r.sandbox(id)
	if err != nil {
		return "", err
	}
	res, logs, err := check(ctx, sb, command, nil)
	if err != nil {
		return "", fmt.Errorf("first run failed, so the monitor isn't saved: %v", err)
	}
	m := &Monitor{Name: name, Command: command, Every: every, Prompt: prompt, State: res.State,
		Next: time.Now().Add(time.Duration(every) * time.Minute)}
	existing := m.record(res.Events)
	if err := r.add(id, m.Name, func(u *userJobs) { u.Monitors = append(u.Monitors, m) }); err != nil {
		return "", err
	}
	sample, _ := json.Marshal(existing[:min(3, len(existing))])
	return fmt.Sprintf("saved monitor %q, next run %s.\nbaseline: %d existing items recorded, none will be sent. "+
		"Sample of these already-existing items, only to check the script works; don't tell the student about them: %s\nstate: %s\nstderr: %s",
		m.Name, show(m.Next), len(existing), sample, res.State, tail(logs)), nil
}

// Cancel drops id's schedule or monitor called name and reports whether there was one.
func (r *Runner) Cancel(id, name string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.users[id] == nil || !r.users[id].remove(name) {
		return false, nil
	}
	return true, r.save(id)
}

func (r *Runner) scheduleTool(id, chat string) tools.Tool {
	return tools.Tool{
		Name: "schedule",
		Description: "Have something delivered to you later, like a reminder. At that time you get a system notice with the prompt, " +
			"in this conversation, and your reply goes to the user here. Give exactly one of at (one time, like 2026-11-14 08:50) or cron " +
			"(repeating, 5-field standard cron, like `0 9 * * 1-5`). " +
			"A job with the same name is replaced. Returns the next time it fires.",
		Schema: `{"type":"object","properties":{"name":{"type":"string","description":"short, unique"},` +
			`"prompt":{"type":"string","description":"what to do or say then, written to your future self"},` +
			`"at":{"type":"string"},"cron":{"type":"string"}},"required":["name","prompt"]}`,
		Run: func(_ context.Context, _ sandbox.Sandbox, raw json.RawMessage) (string, error) {
			var a struct{ Name, Prompt, At, Cron string }
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			s := &Schedule{Name: strings.TrimSpace(a.Name), Prompt: a.Prompt, Cron: a.Cron, Chat: chat}
			switch {
			case s.Name == "" || strings.TrimSpace(a.Prompt) == "":
				return "", errors.New("name and prompt are required")
			case (a.At == "") == (a.Cron == ""):
				return "", errors.New("give exactly one of at or cron")
			case a.At != "":
				t, err := time.ParseInLocation("2006-01-02 15:04", a.At, time.Local)
				if err != nil {
					return "", fmt.Errorf("at must look like 2026-11-14 08:50: %v", err)
				}
				if !t.After(time.Now()) {
					return "", fmt.Errorf("%s has already passed", show(t))
				}
				s.Next = t
			default:
				next, err := cronNext(a.Cron, time.Now())
				if err != nil {
					return "", fmt.Errorf("bad cron: %v", err)
				}
				s.Next = next
			}
			if err := r.add(id, s.Name, func(u *userJobs) { u.Schedules = append(u.Schedules, s) }); err != nil {
				return "", err
			}
			return fmt.Sprintf("scheduled %q, next at %s", s.Name, show(s.Next)), nil
		},
	}
}

func (r *Runner) monitorTool(id string) tools.Tool {
	return tools.Tool{
		Name: "monitor",
		Description: "Watch something for changes (new grades, announcements, a page updating) with a script you write, kept under monitors/. " +
			"Every every_minutes, command runs in your shell with the state it printed last time on stdin (JSON; null on the first run). " +
			`It must print exactly one JSON object on stdout: {"state": <any JSON to get back next time, under 64KB>, "events": [{"id": "...", "text": "..."}]} ` +
			"(logs go to stderr). Emit every current item on every run, each with a stable id (e.g. the item's own id); " +
			"only ids not seen before are delivered, each once. State is optional bookkeeping. " +
			"This runs the first check now as the baseline: everything it emits is recorded as already existing and never delivered. " +
			"The monitor is saved only if that run succeeds. Later new events reach you as a system notice with your prompt; your reply is texted to the user. " +
			"A job with the same name is replaced and starts from null state. After 3 failed runs in a row you get a notice. " +
			"Before creating a grade monitor, check jobs: connecting USC normally installs a grades monitor already.",
		Schema: `{"type":"object","properties":{"name":{"type":"string","description":"short, unique"},` +
			`"command":{"type":"string","description":"e.g. python3 monitors/brightspace.py"},` +
			`"every_minutes":{"type":"integer","minimum":1},` +
			`"prompt":{"type":"string","description":"what to do with new events, written to your future self"}},` +
			`"required":["name","command","every_minutes","prompt"]}`,
		Run: func(ctx context.Context, _ sandbox.Sandbox, raw json.RawMessage) (string, error) {
			var a struct {
				Name, Command, Prompt string
				Every                 int `json:"every_minutes"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			if a.Name = strings.TrimSpace(a.Name); a.Name == "" || strings.TrimSpace(a.Command) == "" || strings.TrimSpace(a.Prompt) == "" {
				return "", errors.New("name, command and prompt are required")
			}
			if a.Every < 1 {
				return "", errors.New("every_minutes must be at least 1")
			}
			return r.AddMonitor(ctx, id, a.Name, a.Command, a.Every, a.Prompt)
		},
	}
}

func (r *Runner) jobsTool(id string) tools.Tool {
	return tools.Tool{
		Name:        "jobs",
		Description: "List the user's schedules and monitors. With cancel, first cancel the one with that name.",
		Schema:      `{"type":"object","properties":{"cancel":{"type":"string"}}}`,
		Run: func(_ context.Context, _ sandbox.Sandbox, raw json.RawMessage) (string, error) {
			var a struct{ Cancel string }
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			var lines []string
			if a.Cancel != "" {
				if ok, err := r.Cancel(id, a.Cancel); err != nil {
					return "", err
				} else if !ok {
					return "", fmt.Errorf("nothing called %q", a.Cancel)
				}
				lines = append(lines, fmt.Sprintf("cancelled %q", a.Cancel))
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			u := r.user(id)
			for _, s := range u.Schedules {
				if s.Cron != "" {
					lines = append(lines, fmt.Sprintf("schedule %q: cron %q, next %s", s.Name, s.Cron, show(s.Next)))
				} else {
					lines = append(lines, fmt.Sprintf("schedule %q: once at %s", s.Name, show(s.Next)))
				}
			}
			for _, m := range u.Monitors {
				line := fmt.Sprintf("monitor %q: every %d min, next %s, command %q", m.Name, m.Every, show(m.Next), m.Command)
				if m.Failures > 0 {
					line += fmt.Sprintf(", failed %d in a row, last error: %s", m.Failures, m.LastError)
				}
				lines = append(lines, line)
			}
			if len(u.Schedules)+len(u.Monitors) == 0 {
				lines = append(lines, "no schedules or monitors")
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}
