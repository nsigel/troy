package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"troy/agent"
	"troy/jobs"
	"troy/model"
	"troy/sandbox"
)

const evalAtOnce = 3

type variant struct {
	name     string
	delegate bool
}

var variants = map[string]variant{
	"code":    {"code", false},
	"default": {"default", true},
}

type verdict struct {
	Correct    bool   `json:"correct"`
	Score      int    `json:"score"`
	Missing    string `json:"missing"`
	Wrong      string `json:"wrong"`
	NeedsCheck bool   `json:"needs_check,omitempty"`
}

type judgment struct {
	SelfContained bool   `json:"self_contained"`
	ChildCorrect  bool   `json:"child_correct"`
	Used          bool   `json:"used"`
	Warranted     bool   `json:"warranted"`
	Note          string `json:"note"`
}

type evalResult struct {
	question    string
	ideal       int
	asked       time.Time
	student     string
	secs        float64
	calls       int
	steps       int
	models      int
	modelLog    string
	writes      string
	userMD      string
	replies     []string
	trace       string
	delegations []agent.Delegated
	judgments   []judgment
	verdict     verdict
	ref         string
	gradeSecs   float64
	escalated   bool
	err         error
	judgeErr    error
	gradeErr    error
	graded      bool
	prompt      string
}

func eval(file string, names []string) error {
	if err := required("OPENROUTER_API_KEY"); err != nil {
		return err
	}
	if len(names) == 0 {
		names = []string{"default"}
	}
	var vs []variant
	for _, n := range names {
		v, ok := variants[n]
		if !ok {
			return fmt.Errorf("unknown variant %q (want code or default)", n)
		}
		vs = append(vs, v)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	skipGrade := os.Getenv("TROY_EVAL_GRADE") == "0"
	var questions []string
	var ideals []int
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ideal := 0
		if i := strings.LastIndex(line, "|"); i >= 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(line[i+1:])); err == nil {
				line, ideal = strings.TrimSpace(line[:i]), n
			}
		}
		questions, ideals = append(questions, line), append(ideals, ideal)
	}
	if len(questions) == 0 {
		return fmt.Errorf("no eval questions in %s", file)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	usc := env("TROY_EVAL_USC", filepath.Join(home, "Library/Application Support/usc"))
	m := model.OpenRouter{Key: os.Getenv("OPENROUTER_API_KEY"), Model: env("TROY_MODEL", model.Default)}
	stamp := time.Now().Format("20060102-150405.000000000")
	out := filepath.Join("eval-results", stamp)
	day, refresh := time.Now(), os.Getenv("TROY_EVAL_REFRESH") != ""
	refs := map[string]func() (string, error){}
	for _, q := range questions {
		refs[q] = sync.OnceValues(func() (string, error) { return reference(q, day, refresh) })
	}

	results := make([][]evalResult, len(vs))
	slots := make(chan struct{}, evalAtOnce)
	var wg sync.WaitGroup
	for vi, v := range vs {
		prompt, err := uscPrompt()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(out, v.name), 0o755); err != nil {
			return err
		}
		results[vi] = make([]evalResult, len(questions))
		for i, q := range questions {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				if !skipGrade {
					go refs[q]()
				}
				r := ask(m, v, prompt, usc, fmt.Sprintf("troy-eval-%s-%s-%d", stamp, v.name, i), q)
				r.ideal = ideals[i]
				if r.err == nil && !skipGrade {
					start := time.Now()
					if r.ref, r.gradeErr = refs[q](); r.gradeErr == nil {
						r.verdict, r.escalated, r.gradeErr = grade(r)
						r.graded = r.gradeErr == nil
					}
					r.gradeSecs = time.Since(start).Seconds()
				}
				if r.err == nil && !skipGrade && len(r.delegations) > 0 {
					r.judgments, r.judgeErr = judge(r)
				}
				results[vi][i] = r
				os.WriteFile(filepath.Join(out, v.name, fmt.Sprintf("%02d.prompt.json", i+1)), []byte(r.prompt), 0o600)
				fmt.Fprintf(os.Stderr, "%s %d/%d done: %s\n", v.name, i+1, len(questions), q)
				os.WriteFile(filepath.Join(out, v.name, fmt.Sprintf("%02d.md", i+1)), []byte(transcript(r)), 0o644)
			})
		}
	}
	wg.Wait()

	var t strings.Builder
	var averages []string
	for vi, v := range vs {
		fmt.Fprintf(&t, "## %s\n\n%-60s %5s %7s %5s %5s %6s %5s %5s\n", v.name, "question", "score", "secs", "steps", "ideal", "models", "tools", "deleg")
		var score, secs, steps, ideal, models, calls, deleg float64
		correct, graded := 0, 0
		for _, r := range results[vi] {
			s := "n/a"
			if r.graded {
				s = fmt.Sprint(r.verdict.Score)
				score += float64(r.verdict.Score)
				graded++
			}
			if r.err != nil {
				s = "err"
			}
			if r.escalated {
				s += "!"
			}
			fmt.Fprintf(&t, "%-60.60s %5s %7.1f %5d %5d %6d %5d %5d   graded in %.0fs\n", r.question, s, r.secs, r.steps, r.ideal, r.models, r.calls, len(r.delegations), r.gradeSecs)
			secs, calls = secs+r.secs, calls+float64(r.calls)
			steps, ideal, models, deleg = steps+float64(r.steps), ideal+float64(r.ideal), models+float64(r.models), deleg+float64(len(r.delegations))
			if r.graded && r.verdict.Correct {
				correct++
			}
		}
		n := float64(len(questions))
		scoreText := "n/a"
		if graded > 0 {
			scoreText = fmt.Sprintf("%.1f", score/float64(graded))
		}
		avg := fmt.Sprintf("%5s %7.1f %5.1f %5.1f %6.1f %5.1f %5.1f", scoreText, secs/n, steps/n, ideal/n, models/n, calls/n, deleg/n)
		fmt.Fprintf(&t, "%-60s %s\ncorrect: %d/%d graded (%d ungraded)   total: %.0fs\n", "average", avg, correct, graded, len(questions)-graded, secs)
		averages = append(averages, fmt.Sprintf("%-60s %s   correct %d/%d", v.name, avg, correct, graded))
		t.WriteString(delegateSummary(results[vi]) + "\n")
	}
	if len(vs) > 1 {
		fmt.Fprintf(&t, "## averages\n\n%-60s %5s %7s %5s %5s %6s %5s %5s\n%s\n\n", "variant", "score", "secs", "steps", "ideal", "models", "tools", "deleg", strings.Join(averages, "\n"))
	}
	fmt.Fprintf(&t, "results: %s\n", out)
	fmt.Print(t.String())
	return os.WriteFile(filepath.Join(out, "summary.txt"), []byte(t.String()), 0o644)
}

func delegateSummary(results []evalResult) string {
	var calls, failed, partial, contained, correct, used, warranted int
	var secs float64
	for _, r := range results {
		for i, d := range r.delegations {
			calls++
			secs += d.End.Sub(d.Start).Seconds()
			if d.Err != nil && d.Result == "" {
				failed++
			} else if d.Err != nil {
				partial++
			}
			if i < len(r.judgments) {
				j := r.judgments[i]
				contained, correct, used, warranted = contained+b2i(j.SelfContained), correct+b2i(j.ChildCorrect), used+b2i(j.Used), warranted+b2i(j.Warranted)
			}
		}
	}
	if calls == 0 {
		return ""
	}
	return fmt.Sprintf("delegate calls: %d (%d failed, %d out of steps with partial answers), %.1fs each on average\n"+
		"self-contained brief %d/%d, child correct %d/%d, result used (not redone) %d/%d, warranted %d/%d\n",
		calls, failed, partial, secs/float64(calls), contained, calls, correct, calls, used, calls, warranted, calls)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ask(m model.Model, v variant, prompt, usc, name, q string) evalResult {
	r := evalResult{question: q}
	dir, err := os.MkdirTemp("", "troy-eval-")
	if err != nil {
		r.err = err
		return r
	}
	defer os.RemoveAll(dir)
	workspace := filepath.Join(dir, "workspace")
	if r.err = os.Mkdir(workspace, 0o755); r.err != nil {
		return r
	}
	session, err := agent.LoadSession(filepath.Join(dir, "session.jsonl"))
	if err != nil {
		r.err = err
		return r
	}
	bin, err := dryRunBin(dir)
	if err != nil {
		r.err = err
		return r
	}
	sb := sandbox.Docker{Name: name, Dir: workspace, USC: usc, Bin: bin, Timeout: 2 * time.Minute, Idle: 5 * time.Minute}
	defer exec.Command("docker", "rm", "-f", name).Run()
	var mu sync.Mutex
	reply := func(_ context.Context, text string) error {
		mu.Lock()
		defer mu.Unlock()
		r.replies = append(r.replies, text)
		return nil
	}
	extra := withStudent(prompt, sb, filepath.Join(dir, studentFile))
	r.student = extra(context.Background()).Student
	calls := &modelCalls{}
	opts := []agent.Option{agent.OnError(func(err error) { mu.Lock(); defer mu.Unlock(); r.err = err }), agent.ChildModel(calls.wrap(m, "child")), agent.OnDelegate(func(d agent.Delegated) {
		mu.Lock()
		defer mu.Unlock()
		r.delegations = append(r.delegations, d)
	})}
	if !v.delegate {
		opts = append(opts, agent.WithoutDelegate)
	}
	if err := os.MkdirAll(filepath.Join(dir, "eval"), 0700); err != nil {
		r.err = err
		return r
	}
	runner, err := jobs.Load(dir, func(string) (sandbox.Sandbox, error) { return sb, nil })
	if err != nil {
		r.err = err
		return r
	}
	opts = append(opts, agent.WithTools(runner.Tools("eval", "")...))
	a, err := agent.New(context.Background(), calls.wrap(m, "troy"), sb, session, extra, reply, nil, opts...)
	if err != nil {
		r.err = err
		return r
	}
	p, err := a.Inspect(context.Background(), false)
	if err != nil {
		a.Close()
		r.err = err
		return r
	}
	snapshot, _ := json.MarshalIndent(p, "", "  ")
	r.prompt = string(snapshot)
	r.asked = time.Now()
	a.Deliver(agent.Event{Text: q})
	a.Close()
	r.secs = time.Since(r.asked).Seconds()
	r.steps, r.models, r.modelLog = calls.count("troy"), calls.count(""), calls.log(r.asked)
	r.writes = writes(bin)
	user, _ := os.ReadFile(filepath.Join(workspace, "USER.md"))
	r.userMD = string(user)
	var trace strings.Builder
	for _, msg := range session.Messages {
		r.calls += len(msg.ToolCalls)
		fmt.Fprintf(&trace, "### %s\n\n%s\n\n", msg.Role, strings.TrimSpace(msg.Content))
		for _, c := range msg.ToolCalls {
			fmt.Fprintf(&trace, "-> %s %s\n\n", c.Name, c.Args)
		}
	}
	for _, d := range r.delegations {
		for _, msg := range d.Messages {
			r.calls += len(msg.ToolCalls)
		}
	}
	r.trace = trace.String()
	return r
}

const checkFacts = `Check the facts yourself with the usc CLI, which is signed in as the same student: usc brightspace courses, content COURSE_ID, announcements, assignments COURSE_ID, grades COURSE_ID, download COURSE_ID TOPIC_ID --output . (then Read the file); usc webreg terms, bin --term TERM, calendar --term TERM and courses for registered classes, including ones with no Brightspace site. usc libcal categories, spaces, space and schedule for Leavey rooms; usc handshake events, event, career-fairs and career-fair for events and fairs (never career-fair-register or career-fair-cancel); usc mobileorder locations, menu, item and checkout for food (never cart, quote or order). Run usc commands plainly, piping only into jq (other pipes and redirects are denied); Read and Grep work on files. Only read: never run usc auth, bypass, browser, libcal book/release, handshake career-fair-register/career-fair-cancel, webreg add/remove/schedule/unschedule/grade-option/checkout/submit or anything else that changes something.`

const evalTime = "Mon Jan 2 2006, 3:04 PM MST"

const refPrompt = `You are writing the reference answer an expert would give a USC student who texted this on %s:

%s

` + checkFacts + ` If the student asks to change something (book a room, order food, register), don't: describe what a correct proposal looks like, with the exact usc commands to run once the student confirms.

answer: markdown, the key facts as a short list, each with its source (the usc command or file it came from), then anything the student should be offered or warned about. No preamble.`

func reference(q string, day time.Time, refresh bool) (string, error) {
	sum := sha256.Sum256([]byte(q))
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(q), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	path := filepath.Join(".troy", "eval-refs", day.Format("2006-01-02"), fmt.Sprintf("%s-%x.md", slug, sum[:3]))
	if _, err := os.Stat(path); err == nil && !refresh {
		return path, nil
	}
	now := time.Now()
	var v struct{ Answer string }
	if err := claudeJSON(fmt.Sprintf(refPrompt, now.Format(evalTime), q), `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`, &v, true); err != nil {
		return "", fmt.Errorf("reference: %w", err)
	}
	return path, writeRef(path, q, "researched "+now.Format(evalTime), v.Answer)
}

func writeRef(path, q, when, answer string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("# %s\n\n%s\n\n%s\n", q, when, strings.TrimSpace(answer))), 0o644)
}

const gradePrompt = `You are grading Troy, an assistant that answers a USC student's texts. The student texted this on %s:

%s

Troy replied:

%s

Commands Troy ran that would have changed something (this eval fakes them; nothing was changed):

%s

An expert's reference answer, researched with usc the same day:

%s

Troy must confirm with the student before changing anything, so for such requests a concrete, correct proposal ready to confirm is a full answer.`

const gradeAsk = `

Grade the reply against the reference: correct is false if anything in it is wrong; score 0-10 is how well it answers what the student asked (right, complete on what matters, useful). missing: what the reference has that Troy's lacks and matters; wrong: what Troy got wrong. Empty strings if none. needs_check: true if Troy states a fact that contradicts the reference, or one the reference doesn't cover that the grade turns on, so it must be checked against usc; grade as if the reference were right, and say in wrong what conflicts.`

const checkAsk = `

Troy's reply and the reference disagree on: %s

Settle that yourself. ` + checkFacts + ` Then grade the reply: correct is false if anything in it is wrong; score 0-10 is how well it answers what the student asked (right, complete on what matters, useful). missing: what an expert answer would have that Troy's lacks; wrong: what Troy got wrong. Empty strings if none. reference_wrong: true if the reference got something wrong; then corrected_reference is the whole reference answer fixed (same format: key facts as a short list with sources; no title or researched line), otherwise empty.`

const gradeSchema = `{"type":"object","properties":{"correct":{"type":"boolean"},"score":{"type":"integer","minimum":0,"maximum":10},"missing":{"type":"string"},"wrong":{"type":"string"}%s},"required":["correct","score","missing","wrong"%s]}`

func grade(r evalResult) (v verdict, escalated bool, err error) {
	w := r.writes
	if w == "" {
		w = "(none)"
	}
	ref, err := os.ReadFile(r.ref)
	if err != nil {
		return v, false, err
	}
	prompt := fmt.Sprintf(gradePrompt, r.asked.Format(evalTime), r.question, strings.Join(r.replies, "\n\n"), w, ref)
	schema := fmt.Sprintf(gradeSchema, `,"needs_check":{"type":"boolean"}`, `,"needs_check"`)
	if err := claudeJSON(prompt+gradeAsk, schema, &v, false); err != nil || !v.NeedsCheck {
		return v, false, err
	}
	var c struct {
		verdict
		ReferenceWrong     bool   `json:"reference_wrong"`
		CorrectedReference string `json:"corrected_reference"`
	}
	schema = fmt.Sprintf(gradeSchema, `,"reference_wrong":{"type":"boolean"},"corrected_reference":{"type":"string"}`, `,"reference_wrong","corrected_reference"`)
	if err := claudeJSON(prompt+fmt.Sprintf(checkAsk, v.Wrong+" "+v.Missing), schema, &c, true); err != nil {
		return v, true, err
	}
	c.NeedsCheck = true
	fixed := c.CorrectedReference
	if p := strings.SplitN(fixed, "\n\n", 3); strings.HasPrefix(fixed, "# ") && len(p) == 3 {
		fixed = p[2]
	}
	if parts := strings.SplitN(string(ref), "\n\n", 3); c.ReferenceWrong && fixed != "" && len(parts) == 3 {
		err = writeRef(r.ref, r.question, parts[1]+", corrected "+time.Now().Format(evalTime), fixed)
	}
	return c.verdict, true, err
}

const judgePrompt = `You are reviewing how Troy, an assistant that answers a USC student's texts, used its delegate tool. delegate hands a sub-task to a fresh agent with the same tools, workspace and student info but none of the chat; Troy gets back only that agent's final reply. Several delegate calls in one step run in parallel.

The student texted this on %s:

%s

Troy replied:

%s

Troy's own trace (tool outputs shortened):

%s

The delegate calls, in the order they finished:

%s

` + checkFacts + `

Then judge each delegate call, in the same order: self_contained: the brief alone was enough for a fresh agent (ids, timeframe, what to return). child_correct: the delegated agent's reply is right and answers its brief. used: Troy used that reply instead of redoing the same work itself. warranted: delegating was worth it here, versus Troy doing it directly in a python call or two. note: one or two sentences on anything that fell short, empty if nothing did.`

func judge(r evalResult) ([]judgment, error) {
	var calls strings.Builder
	for i, d := range r.delegations {
		fmt.Fprintf(&calls, "## delegate call %d\n\nbrief:\n%s\n\nits tool calls:\n", i+1, d.Brief)
		for _, msg := range d.Messages {
			for _, c := range msg.ToolCalls {
				fmt.Fprintf(&calls, "-> %s %s\n", c.Name, c.Args)
			}
		}
		result := d.Result
		if d.Err != nil {
			result += "\nerror: " + d.Err.Error()
		}
		fmt.Fprintf(&calls, "\nwhat Troy got back:\n%s\n\n", result)
	}
	n := len(r.delegations)
	schema := fmt.Sprintf(`{"type":"object","properties":{"calls":{"type":"array","minItems":%d,"maxItems":%d,"items":{"type":"object","properties":{"self_contained":{"type":"boolean"},"child_correct":{"type":"boolean"},"used":{"type":"boolean"},"warranted":{"type":"boolean"},"note":{"type":"string"}},"required":["self_contained","child_correct","used","warranted","note"]}}},"required":["calls"]}`, n, n)
	parts := strings.Split(r.trace, "### ")
	for i, p := range parts {
		if len(p) > 1500 {
			parts[i] = p[:1500] + "\n...[shortened]\n\n"
		}
	}
	prompt := fmt.Sprintf(judgePrompt, r.asked.Format(evalTime), r.question,
		strings.Join(r.replies, "\n\n"), strings.Join(parts, "### "), calls.String())
	var v struct{ Calls []judgment }
	return v.Calls, claudeJSON(prompt, schema, &v, true)
}

func claudeJSON(prompt, schema string, v any, tools bool) error {
	dir, err := os.MkdirTemp("", "troy-grade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	args := []string{"-p", prompt, "--model", "opus",
		"--output-format", "json", "--json-schema", schema, "--no-session-persistence",
		"--setting-sources", "", "--strict-mcp-config", "--permission-mode", "dontAsk", "--tools"}
	if !tools {
		args = append(args, "")
	} else {
		args = append(args, "Bash,Read,Grep,Glob", "--allowedTools",
			"Bash(usc brightspace:*)", "Bash(usc classes:*)", "Bash(usc handshake events:*)", "Bash(usc handshake event:*)", "Bash(usc handshake career-fairs:*)", "Bash(usc handshake career-fair:*)", "Bash(usc handshake categories:*)", "Bash(usc webreg terms:*)", "Bash(usc webreg bin:*)", "Bash(usc webreg calendar:*)", "Bash(usc webreg courses:*)", "Bash(usc libcal categories:*)", "Bash(usc libcal spaces:*)", "Bash(usc libcal space:*)", "Bash(usc libcal schedule:*)", "Bash(usc mobileorder locations:*)", "Bash(usc mobileorder menu:*)", "Bash(usc mobileorder item:*)", "Bash(usc mobileorder checkout:*)", "Bash(jq:*)", "Bash(unzip -p:*)", "Read", "Grep", "Glob",
			"--disallowedTools", "Bash(usc handshake career-fair-register:*)", "Bash(usc handshake career-fair-cancel:*)")
	}
	cmd := exec.Command(env("TROY_EVAL_CLAUDE", "claude"), args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("claude: %v: %s", err, stderr.String())
	}
	var res struct {
		StructuredOutput json.RawMessage `json:"structured_output"`
		Result           string          `json:"result"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return err
	}
	if len(res.StructuredOutput) == 0 || string(res.StructuredOutput) == "null" {
		return fmt.Errorf("no structured output: %s", res.Result)
	}
	return json.Unmarshal(res.StructuredOutput, v)
}

func transcript(r evalResult) string {
	var b strings.Builder
	v := []byte("Not graded.")
	if r.graded {
		v, _ = json.MarshalIndent(r.verdict, "", "  ")
	}
	if r.gradeErr != nil {
		fmt.Fprintf(&b, "Grading unavailable: %v\n\n", r.gradeErr)
	}
	fmt.Fprintf(&b, "# %s\n\nasked %s, %.1fs, %d model steps (ideal %d), %d model calls, %d tool calls, %d delegate calls\n\n",
		r.question, r.asked.Format(time.RFC3339), r.secs, r.steps, r.ideal, r.models, r.calls, len(r.delegations))
	if r.err != nil {
		fmt.Fprintf(&b, "error: %v\n\n", r.err)
	}
	if r.judgeErr != nil {
		fmt.Fprintf(&b, "judge error: %v\n\n", r.judgeErr)
	}
	fmt.Fprintf(&b, "reference %s, graded in %.1fs", r.ref, r.gradeSecs)
	if r.escalated {
		b.WriteString(", disagreement checked against usc")
	}
	fmt.Fprintf(&b, "\n\n## replies\n\n%s\n\n## verdict\n\n%s\n\n## writes (faked)\n\n%s\n\n## model calls\n\n%s\n## USER.md after\n\n%s\n\n## student section\n\n%s\n\n## trace\n\n%s",
		strings.Join(r.replies, "\n\n"), v, r.writes, r.modelLog, strings.TrimSpace(r.userMD), r.student, r.trace)
	for i, d := range r.delegations {
		calls, tools := 0, 0
		var trace strings.Builder
		for _, msg := range d.Messages {
			if msg.Role == "assistant" {
				calls++
			}
			tools += len(msg.ToolCalls)
			fmt.Fprintf(&trace, "#### %s\n\n%s\n\n", msg.Role, strings.TrimSpace(msg.Content))
			for _, c := range msg.ToolCalls {
				fmt.Fprintf(&trace, "-> %s %s\n\n", c.Name, c.Args)
			}
		}
		fmt.Fprintf(&b, "\n## delegate call %d\n\nran %.1fs to %.1fs after the question (%.1fs), %d model calls, %d tool calls\n\n",
			i+1, d.Start.Sub(r.asked).Seconds(), d.End.Sub(r.asked).Seconds(), d.End.Sub(d.Start).Seconds(), calls, tools)
		if d.Err != nil {
			fmt.Fprintf(&b, "error: %v\n\n", d.Err)
		}
		if i < len(r.judgments) {
			j, _ := json.MarshalIndent(r.judgments[i], "", "  ")
			fmt.Fprintf(&b, "### judgment\n\n%s\n\n", j)
		}
		fmt.Fprintf(&b, "### brief\n\n%s\n\n### result\n\n%s\n\n### trace\n\n%s", d.Brief, d.Result, trace.String())
	}
	return b.String()
}
