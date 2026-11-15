package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"troy/model"
	"troy/sandbox"
	"troy/tools"
)

const maxSteps = 20

// typingEvery refreshes the typing indicator before it expires (Sendblue's lasts a minute).
const typingEvery = 50 * time.Second

const noticePrefix = "[system notice, not typed by the user] "

// noReply is what the model writes to send nothing; models won't reliably answer with empty text.
const noReply = "NO_REPLY"

// If generation fails entirely, log the failure without sending canned text.
const unreachable = noReply

// markup is tool-call syntax leaking into a reply's text (DeepSeek's fullwidth-bar tokens, or
// XML-style calls), which is never sent to the student.
var markup = regexp.MustCompile(`｜|<(tool_call|function_calls|invoke)\b`)

type Event struct {
	Text   string // typed by the user
	Notice string // from Troy's own system (e.g. the web app), shown to the model as such
}

// Agent is one user's assistant. Events go into its inbox and are handled one at a time.
type Agent struct {
	model   model.Model
	sandbox sandbox.Sandbox
	tools   []tools.Tool
	session *Session
	extra   func(context.Context) Context
	reply   func(ctx context.Context, text string) error
	typing  func(ctx context.Context) error
	stream  Stream
	retype  chan struct{}
	inbox   chan Event
	done    chan struct{}

	skillTools   []tools.Tool
	more         []tools.Tool
	noDelegate   bool
	childModel   model.Model
	onError      func(error)
	onDelegate   func(Delegated)
	suggestModel model.Model
	suggest      func(context.Context, string) error
	tasks        atomic.Int64
}

// Option changes how New sets up an agent.
type Option func(*Agent)

// WithTools adds tools to the agent alongside its default tools.
func WithTools(more ...tools.Tool) Option {
	return func(a *Agent) { a.more = append(a.more, more...) }
}

// OnError observes turn failures, including those followed by a partial-answer recovery.
func OnError(f func(error)) Option { return func(a *Agent) { a.onError = f } }

var commands = map[string]func(a *Agent) string{
	"/new": func(a *Agent) string {
		if err := a.session.Reset(); err != nil {
			return "The conversation reset failed. Briefly tell the user it did not work."
		}
		return "The conversation was reset at the user's request. Briefly acknowledge the fresh start."
	},
}

// New starts an agent. extra supplies trusted application instructions and separate student data before each turn.
// typing, if not nil, shows the user that Troy is working on a reply.
func New(ctx context.Context, m model.Model, sb sandbox.Sandbox, session *Session, extra func(context.Context) Context,
	reply func(context.Context, string) error, typing func(context.Context) error, opts ...Option) (*Agent, error) {
	if err := seedWorkspace(ctx, sb); err != nil {
		return nil, err
	}
	a := &Agent{model: m, sandbox: sb, session: session, extra: extra, reply: reply, typing: typing,
		retype: make(chan struct{}, 1), inbox: make(chan Event, 64), done: make(chan struct{}), childModel: m}
	for _, o := range opts {
		o(a)
	}
	var err error
	a.skillTools, err = tools.SkillTools()
	if err != nil {
		return nil, err
	}

	if a.stream == nil {
		a.tools = append(a.tools, a.sendMessage())
	}
	a.tools = append(a.tools, a.workTools()...)
	a.tools = append(a.tools, a.more...)
	if !a.noDelegate {
		a.tools = append(a.tools, a.delegate())
	}
	go a.run(ctx)
	return a, nil
}

func (a *Agent) Deliver(e Event) { a.inbox <- e }

// TryDeliver is Deliver without waiting; it reports false when the inbox is full.
func (a *Agent) TryDeliver(e Event) bool {
	select {
	case a.inbox <- e:
		return true
	default:
		return false
	}
}

// Close stops accepting events and waits for queued ones to finish.
func (a *Agent) Close() {
	close(a.inbox)
	<-a.done
}

// Release is Close without waiting, if no events are queued; it reports whether it closed the
// inbox. Nothing may be delivered during or after it.
func (a *Agent) Release() bool {
	if len(a.inbox) > 0 {
		return false
	}
	close(a.inbox)
	return true
}

func (a *Agent) run(ctx context.Context) {
	defer close(a.done)
	for e := range a.inbox {
		if cmd, ok := commands[strings.TrimSpace(e.Text)]; ok && a.stream == nil {
			e = Event{Notice: cmd(a)}
		}
		lines := []string{e.input()}
		for _, more := range a.drain() {
			lines = append(lines, more.input())
		}
		stop := a.showTyping(ctx)
		a.stream.put(Update{Type: "turn_start", Taken: len(lines)})
		text, err := a.turn(ctx, strings.Join(lines, "\n"), a.tools)
		if err != nil {
			log.Println("turn failed:", err)
			if a.onError != nil {
				a.onError(err)
			}
			text = a.explain(ctx, err)
		}
		if markup.MatchString(text) {
			log.Printf("reply was tool-call markup: %.200q", text)
			err = errors.New("reply contained tool-call markup")
			text = a.explain(ctx, err)
			if markup.MatchString(text) {
				text = noReply
			}
		}
		stop()
		if a.stream == nil {
			a.send(ctx, text)
		}
		a.stream.put(Update{Type: "turn_end", Error: err != nil})
		if a.suggest != nil {
			go a.suggestNext(ctx, append([]model.Message{}, a.session.Messages...))
		}
	}
}

func (a *Agent) suggestNext(ctx context.Context, msgs []model.Message) {
	text, err := Suggest(ctx, a.suggestModel, msgs)
	if err == nil && text != "" {
		err = a.suggest(ctx, text)
	}
	if err != nil {
		log.Println("suggest failed:", err)
	}
}

func (e Event) input() string {
	if e.Notice != "" {
		return noticePrefix + e.Notice
	}
	return e.Text
}

func (a *Agent) drain() (events []Event) {
	for {
		select {
		case e, ok := <-a.inbox:
			if !ok {
				return events
			}
			events = append(events, e)
		default:
			return events
		}
	}
}

// send texts each blank-line-separated part of text as its own message, except noReply.
func (a *Agent) send(ctx context.Context, text string) {
	for _, part := range strings.Split(text, "\n\n") {
		if part = strings.TrimSpace(part); part == "" || part == noReply {
			continue
		}
		if err := a.reply(ctx, part); err != nil {
			log.Println("reply failed:", err)
			return
		}
	}
}

func (a *Agent) showTyping(ctx context.Context) (stop func()) {
	if a.typing == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(typingEvery)
		defer t.Stop()
		for {
			if err := a.typing(ctx); err != nil && ctx.Err() == nil {
				log.Println("typing failed:", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-a.retype:
				t.Reset(typingEvery)
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (a *Agent) sendMessage() tools.Tool {
	return tools.Tool{
		Name:        "send_message",
		Description: "Text the user immediately. Only for a short heads-up before slow work, not for splitting your reply (blank lines in your final reply do that); your final reply is still sent.",
		Schema:      `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
		Run: func(ctx context.Context, _ sandbox.Sandbox, raw json.RawMessage) (string, error) {
			var args struct{ Text string }
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Text) == "" {
				return "", errors.New("empty message")
			}
			if err := a.reply(ctx, args.Text); err != nil {
				return "", err
			}
			select { // sending a text hides the typing indicator
			case a.retype <- struct{}{}:
			default:
			}
			return "sent", nil
		},
	}
}

func (a *Agent) workTools() []tools.Tool {
	return append(append([]tools.Tool{}, tools.Code...), a.skillTools...)
}

// explain has the model tell the user, in its own voice, that its turn failed.
func (a *Agent) explain(ctx context.Context, cause error) string {
	out, err := a.turn(ctx, noticePrefix+"Your last turn failed before you could reply ("+cause.Error()+
		"). Your tools are off now: in plain text, tell the user what you found, if anything, and that the rest didn't work.", nil)
	if err != nil || out == "" {
		return unreachable
	}
	return out
}

// turn runs the model on text until it answers without calling a tool. With no tools it is one call.
func (a *Agent) turn(ctx context.Context, text string, toolset []tools.Tool) (string, error) {
	prompt, err := a.prompt(ctx, false, toolset)
	if err != nil {
		return "", err
	}
	return loop(ctx, a.model, a.sandbox, a.stream, prompt.Messages, a.session, text, toolset, maxSteps)
}

// Inspect returns the live prompt prefix without calling the model or changing conversation history.
func (a *Agent) Inspect(ctx context.Context, child bool) (Prompt, error) {
	toolset := a.tools
	if child {
		toolset = a.workTools()
	}
	return a.prompt(ctx, child, toolset)
}

func (a *Agent) prompt(ctx context.Context, child bool, toolset []tools.Tool) (Prompt, error) {
	extra := Context{}
	if a.extra != nil {
		extra = a.extra(ctx)
	}
	names := []string{"AGENTS.md", "SOUL.md", "IMESSAGE.md"}
	switch {
	case child:
		names = []string{"AGENTS.md", "CHILD.md"}
	case a.stream != nil:
		names[2] = "WEB.md"
	}
	return BuildPrompt(ctx, a.sandbox, names, toolset, extra)
}

func loop(ctx context.Context, m model.Model, sb sandbox.Sandbox, s Stream, prefix []model.Message, session *Session, text string,
	toolset []tools.Tool, steps int) (string, error) {
	toolset, closePython := tools.PythonTask(toolset)
	defer closePython()
	stamp := time.Now().Format("[Mon Jan 2 2006, 3:04 PM MST] ")
	if err := session.Append(model.Message{Role: "user", Content: stamp + text}); err != nil {
		return "", err
	}
	for range steps {
		recent, err := history(ctx, sb, session.Messages)
		if err != nil {
			return "", err
		}
		msgs := append(append([]model.Message{}, prefix...), recent...)
		msgs, err = hydrateDocuments(ctx, sb, msgs)
		if err != nil {
			return "", err
		}
		text := splitter{s: s}
		msg, err := m.Chat(ctx, msgs, tools.Defs(toolset), text.onText())
		if err != nil {
			return "", err
		}
		text.flush()
		if err := session.Append(msg); err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		for _, call := range msg.ToolCalls {
			s.put(Update{Type: "tool_start", ID: call.ID, Name: call.Name, Args: call.Args})
		}
		results := make([]model.Message, len(msg.ToolCalls))
		var wg sync.WaitGroup
		for i, call := range msg.ToolCalls {
			run := func() {
				out := tools.Execute(ctx, toolset, sb, call.Name, call.Args)
				results[i] = skillResult(session, call, out.Content)
				results[i].Documents, results[i].Failed = out.Documents, out.Failed
				s.put(Update{Type: "tool_end", ID: call.ID, Output: results[i].Content, Failed: out.Failed})
			}
			if call.Name == "python" {
				run()
			} else {
				wg.Go(run)
			}
		}
		wg.Wait()
		for _, result := range results {
			if err := session.Append(result); err != nil {
				return "", err
			}
		}
	}
	return "", fmt.Errorf("gave up after %d steps: %w", steps, errSteps)
}
