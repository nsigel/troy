// Package jobs runs what students' agents set up to happen later: schedules (reminders, one-shot
// or cron) and monitors (scripts in the student's sandbox that report new events). When one is
// due, the agent gets a system notice and texts the student if it's worth it.
package jobs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"troy/sandbox"
)

const (
	wake             = 5 * time.Second
	failureThreshold = 3
	keepSeen         = 1000 // event ids remembered per monitor, or more if the latest run emitted more
	maxState         = 64 * 1024
)

// Schedule delivers Prompt at Next, once, or on every Cron time if it has one, to the
// conversation it was set in: Chat, a web chat, or the iMessage thread if empty.
type Schedule struct {
	Name   string    `json:"name"`
	Prompt string    `json:"prompt"`
	Cron   string    `json:"cron,omitempty"`
	Chat   string    `json:"chat,omitempty"`
	Next   time.Time `json:"next"`
}

// Monitor runs Command every Every minutes with State on stdin and delivers the events it
// hasn't reported before.
type Monitor struct {
	Name      string          `json:"name"`
	Command   string          `json:"command"`
	Every     int             `json:"every_minutes"`
	Prompt    string          `json:"prompt"`
	State     json.RawMessage `json:"state"`
	Seen      []string        `json:"seen,omitempty"`
	Failures  int             `json:"failures,omitempty"`
	LastError string          `json:"last_error,omitempty"`
	Next      time.Time       `json:"next"`
	running   bool
}

type Event struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type userJobs struct {
	Schedules []*Schedule `json:"schedules"`
	Monitors  []*Monitor  `json:"monitors"`
}

// Runner owns every user's jobs: tools change them, Run fires them.
type Runner struct {
	root    string // $TROY_HOME/users
	sandbox func(userID string) (sandbox.Sandbox, error)
	Notify  func(userID, chat, notice string) // chat is a web chat id, or "" for the iMessage thread
	mu      sync.Mutex
	users   map[string]*userJobs
}

// Load reads every users/<id>/jobs.json under root. Missed cron runs are skipped; past-due
// one-shots stay and fire late.
func Load(root string, sb func(userID string) (sandbox.Sandbox, error)) (*Runner, error) {
	r := &Runner{root: root, sandbox: sb, users: map[string]*userJobs{}}
	paths, err := filepath.Glob(filepath.Join(root, "*", "jobs.json"))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		u := &userJobs{}
		if err := json.Unmarshal(data, u); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, s := range u.Schedules {
			if s.Cron != "" && s.Next.Before(now) {
				if s.Next, err = cronNext(s.Cron, now); err != nil {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
			}
		}
		r.users[filepath.Base(filepath.Dir(path))] = u
	}
	return r, nil
}

// Run fires due jobs until ctx is done.
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(wake)
	defer t.Stop()
	for {
		r.runDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Runner) runDue(ctx context.Context) {
	now := time.Now()
	type notice struct{ id, chat, text string }
	var notices []notice
	r.mu.Lock()
	for id, u := range r.users {
		changed := false
		var kept []*Schedule
		for _, s := range u.Schedules {
			if s.Next.After(now) {
				kept = append(kept, s)
				continue
			}
			changed = true
			notices = append(notices, notice{id, s.Chat, fmt.Sprintf("Scheduled %q (set for %s): %s", s.Name, show(s.Next), s.Prompt)})
			if s.Cron != "" {
				next, err := cronNext(s.Cron, now)
				if err != nil {
					log.Printf("jobs %s: dropping schedule %q: %v", id, s.Name, err)
					continue
				}
				s.Next = next
				kept = append(kept, s)
			}
		}
		u.Schedules = kept
		for _, m := range u.Monitors {
			if m.running || m.Next.After(now) {
				continue
			}
			// Keep to the monitor's own cadence, so waking late doesn't make it drift.
			every := time.Duration(m.Every) * time.Minute
			if m.Next = m.Next.Add(every); m.Next.Before(now) {
				m.Next = now.Add(every)
			}
			m.running, changed = true, true
			go r.tick(ctx, id, m)
		}
		if changed {
			r.save(id)
		}
	}
	r.mu.Unlock()
	for _, n := range notices {
		r.Notify(n.id, n.chat, n.text)
	}
}

// tick runs one check of m and delivers what's new. Only one tick of a monitor runs at a time.
func (r *Runner) tick(ctx context.Context, id string, m *Monitor) {
	sb, err := r.sandbox(id)
	var res result
	if err == nil {
		res, _, err = check(ctx, sb, m.Command, m.State)
	}
	notice := ""
	r.mu.Lock()
	m.running = false
	if ctx.Err() != nil || r.users[id] == nil || !slices.Contains(r.users[id].Monitors, m) {
		r.mu.Unlock() // shutting down, or cancelled or replaced meanwhile
		return
	}
	if err != nil {
		m.Failures++
		m.LastError = err.Error()
		log.Printf("jobs %s: monitor %q failed (%d in a row)", id, m.Name, m.Failures)
		if m.Failures == failureThreshold {
			notice = fmt.Sprintf("Monitor %q has failed %d times in a row, so it isn't seeing anything. Last error: %s. Fix its script or tell the student.", m.Name, m.Failures, m.LastError)
		}
	} else {
		m.State, m.Failures, m.LastError = res.State, 0, ""
		if texts := m.record(res.Events); len(texts) > 0 {
			notice = fmt.Sprintf("Monitor %q: %s. Your instructions for it: %s", m.Name, strings.Join(texts, "\n"), m.Prompt)
		}
	}
	r.save(id)
	r.mu.Unlock()
	if notice != "" {
		r.Notify(id, "", notice)
	}
}

// record remembers events' ids and returns the texts of those not seen before. Ids in this run
// move to the end, so only ids the script stopped emitting age out.
func (m *Monitor) record(events []Event) (texts []string) {
	for _, e := range events {
		if i := slices.Index(m.Seen, e.ID); i >= 0 {
			m.Seen = slices.Delete(m.Seen, i, i+1)
		} else {
			texts = append(texts, e.Text)
		}
		m.Seen = append(m.Seen, e.ID)
	}
	if keep := max(keepSeen, len(events)); len(m.Seen) > keep {
		m.Seen = slices.Clone(m.Seen[len(m.Seen)-keep:])
	}
	return texts
}

type result struct {
	State  json.RawMessage `json:"state"`
	Events []Event         `json:"events"`
}

// stdoutMark separates what the command wrote to stderr (Exec combines them) from its stdout.
const stdoutMark = "@@troy-monitor-stdout@@"

// check runs command in sb with state on stdin and parses its stdout. logs is its stderr.
func check(ctx context.Context, sb sandbox.Sandbox, command string, state json.RawMessage) (res result, logs string, err error) {
	if state == nil {
		state = json.RawMessage("null")
	}
	script := fmt.Sprintf("out=$(printf %%s %s | base64 -d | (%s\n)) || exit\nprintf '\\n%%s\\n%%s' %s \"$out\"",
		base64.StdEncoding.EncodeToString(state), command, stdoutMark)
	out, code, err := sb.Exec(ctx, script)
	if err == nil && code != 0 {
		err = fmt.Errorf("exit code %d", code)
	}
	if err != nil {
		return res, out, fmt.Errorf("%v, output: %s", err, tail(out))
	}
	logs, stdout := "", out
	if i := strings.LastIndex(out, "\n"+stdoutMark+"\n"); i >= 0 {
		logs, stdout = out[:i], out[i+len(stdoutMark)+2:]
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		return res, logs, fmt.Errorf(`stdout isn't one JSON object like {"state": ..., "events": [{"id": ..., "text": ...}]} (%v), stdout: %s`, err, tail(stdout))
	}
	if res.State == nil {
		res.State = json.RawMessage("null")
	}
	if len(res.State) > maxState {
		return res, logs, fmt.Errorf("state is %d bytes, keep it under %d", len(res.State), maxState)
	}
	for _, e := range res.Events {
		if e.ID == "" {
			return res, logs, errors.New("every event needs a non-empty id")
		}
	}
	return res, logs, nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = "…" + s[len(s)-500:]
	}
	if s == "" {
		return "(none)"
	}
	return s
}

func cronNext(expr string, after time.Time) (time.Time, error) {
	s, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, err
	}
	return s.Next(after), nil
}

func show(t time.Time) string { return t.Local().Format("Mon Jan 2, 3:04 PM") }

// user is id's jobs, created empty if needed. Call with mu held.
func (r *Runner) user(id string) *userJobs {
	if r.users[id] == nil {
		r.users[id] = &userJobs{}
	}
	return r.users[id]
}

// remove drops the schedule or monitor called name and reports whether there was one.
func (u *userJobs) remove(name string) bool {
	n := len(u.Schedules) + len(u.Monitors)
	u.Schedules = slices.DeleteFunc(u.Schedules, func(s *Schedule) bool { return s.Name == name })
	u.Monitors = slices.DeleteFunc(u.Monitors, func(m *Monitor) bool { return m.Name == name })
	return len(u.Schedules)+len(u.Monitors) < n
}

// save writes id's jobs.json atomically. Call with mu held.
func (r *Runner) save(id string) error {
	data, err := json.MarshalIndent(r.users[id], "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(r.root, id, "jobs.json")
	if err = os.WriteFile(path+".tmp", data, 0o644); err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		log.Printf("jobs %s: saving: %v", id, err)
	}
	return err
}
