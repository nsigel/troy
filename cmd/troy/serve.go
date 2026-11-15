package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"troy/agent"
	"troy/api"
	"troy/channel"
	"troy/jobs"
	"troy/sendblue"
	"troy/web"
)

// shutdownGrace bounds how long in-flight turns may run after SIGINT/SIGTERM.
const shutdownGrace = time.Minute

const localAddr = "127.0.0.1:8788"

// The built-in grades monitor, registered when a student connects USC. Every 30 minutes is
// quick enough for grades and keeps us to a few Brightspace requests per course per half hour.
const (
	gradesCommand = "python3 /usr/local/lib/troy/grades.py" // in the sandbox image
	gradesEvery   = 30
	gradesPrompt  = "New or changed grades just posted on Brightspace (above, as course item: score). " +
		"Text the student about them casually and briefly, all in one message."
)

func serve() error {
	if err := required("OPENROUTER_API_KEY"); err != nil {
		return err
	}
	u, err := newUsers()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(u.root, 0o755); err != nil {
		return err
	}
	key, err := api.LoadKey(filepath.Join(filepath.Dir(u.root), "web_signing_key"))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	webAPI := &api.API{Key: key, URL: strings.TrimRight(env("TROY_URL", "http://localhost:8787"), "/"), Users: u.root, TroyPhoneNumber: os.Getenv("SENDBLUE_FROM_NUMBER")}
	webAPI.Register(mux)
	mux.Handle("GET /", web.App())
	local := &channel.Local{Addr: env("TROY_LOCAL_ADDR", localAddr)}
	phone, err := sendblueChannel(mux)
	if err != nil {
		return err
	}
	// Turns get their own context so a signal lets them finish; it is cancelled only
	// when they outlast shutdownGrace.
	turns, cancel := context.WithCancel(context.Background())
	defer cancel()
	jb, err := jobs.Load(u.root, u.userSandbox)
	if err != nil {
		return err
	}
	r := &router{ctx: turns, users: u, jobs: jb, local: local, phone: phone, signIn: webAPI.SignInNotice, stream: webAPI.Stream, agents: map[string]*agent.Agent{}}
	webAPI.Notify = r.notify
	webAPI.Say = r.say
	jb.Notify = func(id, chat, notice string) {
		if chat == "" || !webAPI.ChatNotice(id, chat, notice) {
			r.notify(id, notice)
		}
	}
	webAPI.USCChanged = func(id string, connected bool) string { return gradeAlerts(turns, jb, id, connected) }

	sig, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := env("TROY_ADDR", "127.0.0.1:8787")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s", addr)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	srv.RegisterOnShutdown(webAPI.CloseStreams)
	failed := make(chan error, 3)
	go func() { failed <- srv.Serve(ln) }()
	go jb.Run(sig)
	go backfillGradeAlerts(turns, jb, u.root)
	for _, ch := range []channel.Channel{local, phone} {
		if ch != nil {
			go func() { failed <- ch.Listen(sig, r.handle) }()
		}
	}
	select {
	case <-sig.Done():
	case err = <-failed:
	}
	stop()
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	srv.Shutdown(sctx)
	log.Printf("shutting down, waiting up to %s for turns", shutdownGrace)
	done := make(chan struct{})
	go func() { r.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		log.Println("cutting off unfinished turns")
		cancel()
		<-done
	}
	return err
}

// gradeAlerts turns the built-in grades monitor on when id connects USC and off when they
// disconnect. It returns a line for the agent's connect notice once alerts are on.
func gradeAlerts(ctx context.Context, jb *jobs.Runner, id string, connected bool) string {
	if !connected {
		if _, err := jb.Cancel(id, "grades"); err != nil {
			log.Printf("jobs %s: cancelling grades: %v", id, err)
		}
		return ""
	}
	if _, err := jb.AddMonitor(ctx, id, "grades", gradesCommand, gradesEvery, gradesPrompt); err != nil {
		log.Printf("jobs %s: grades monitor: %v", id, err)
		return ""
	}
	log.Printf("jobs %s: grades monitor on", id)
	return fmt.Sprintf("Grade alerts are on: the built-in monitor called grades checks Brightspace every %d minutes and tells you about new grades. "+
		"Mention that in one short line; if they don't want it, cancel it with jobs.", gradesEvery)
}

// backfillGradeAlerts turns on grade alerts for students who connected USC before they
// existed: connected, but no jobs.json yet. A student who cancels keeps a jobs.json, so
// cancelling sticks.
func backfillGradeAlerts(ctx context.Context, jb *jobs.Runner, root string) {
	creds, _ := filepath.Glob(filepath.Join(root, "*", "usc", "credentials.json"))
	for _, c := range creds {
		dir := filepath.Dir(filepath.Dir(c))
		if _, err := os.Stat(filepath.Join(dir, "jobs.json")); errors.Is(err, os.ErrNotExist) {
			gradeAlerts(ctx, jb, filepath.Base(dir), true)
		}
	}
}

// sendblueChannel is the iMessage channel when all its settings are set, nil when none are.
func sendblueChannel(mux *http.ServeMux) (channel.Channel, error) {
	names := []string{"SENDBLUE_API_KEY_ID", "SENDBLUE_API_SECRET_KEY", "SENDBLUE_FROM_NUMBER", "SENDBLUE_WEBHOOK_SECRET"}
	err := required(names...)
	if err != nil {
		for _, n := range names {
			if os.Getenv(n) != "" {
				return nil, err
			}
		}
		log.Println("sendblue off: no SENDBLUE_* settings")
		return nil, nil
	}
	return channel.Sendblue{
		Client: sendblue.Client{KeyID: os.Getenv("SENDBLUE_API_KEY_ID"), Secret: os.Getenv("SENDBLUE_API_SECRET_KEY"), From: os.Getenv("SENDBLUE_FROM_NUMBER")},
		Secret: os.Getenv("SENDBLUE_WEBHOOK_SECRET"),
		Mux:    mux,
	}, nil
}

// router hands each conversation's messages to its agent, creating it on first contact: a
// user's iMessage thread and each of their web chats. Sign-in requests become system notices.
// iMessage replies go out through the channel that owns the user's id: phone numbers are
// Sendblue's, names are local. Web chats stream to the web app.
type router struct {
	ctx    context.Context // the agents' context
	users  users
	jobs   *jobs.Runner
	local  channel.Channel
	phone  channel.Channel // nil without Sendblue
	signIn func(userID, text string) (reply string, ok bool)
	stream func(userID, chatID string) agent.Stream
	mu     sync.Mutex
	agents map[string]*agent.Agent // by user id, and user id/chat id for web chats; nil once closed
}

// handle only enqueues, so it returns at once and never waits on a turn.
func (r *router) handle(msg channel.Message) {
	log.Printf("in  %s %dB", msg.UserID, len(msg.Text))
	r.deliver(msg.UserID, agent.Event{Text: msg.Text})
}

// notify tells a user's agent about something that happened outside the chat.
func (r *router) notify(userID, notice string) {
	log.Printf("in  %s notice %dB", userID, len(notice))
	os.Remove(filepath.Join(r.users.root, userID, studentFile))
	r.deliver(userID, agent.Event{Notice: notice})
}

func (r *router) deliver(id string, e agent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, err := r.thread(id)
	if err != nil {
		log.Printf("in  %s dropped: %v", id, err)
		return
	}
	if notice, ok := r.signIn(id, e.Text); ok {
		e = agent.Event{Notice: notice}
	}
	if !a.TryDeliver(e) {
		log.Printf("in  %s dropped: inbox full", id)
	}
}

func (r *router) say(id, chat, text string, deliver func(*agent.Agent) bool) error {
	log.Printf("in  %s chat %s %dB", id, chat, len(text))
	r.mu.Lock()
	defer r.mu.Unlock()
	a, err := r.webChat(id, chat)
	if err != nil {
		log.Printf("in  %s chat %s dropped: %v", id, chat, err)
		return err
	}
	if !deliver(a) {
		log.Printf("in  %s chat %s dropped: inbox full", id, chat)
		return errors.New("inbox full")
	}
	return nil
}

// channel is the one channel that reaches id; nil when that is Sendblue and it is off.
func (r *router) channel(id string) channel.Channel {
	if channel.IsSendblueUser(id) {
		return r.phone
	}
	return r.local
}

func (r *router) send(ctx context.Context, id, text string) error {
	ch, err := r.channel(id), errors.New("sendblue is off")
	start := time.Now()
	if ch != nil {
		err = ch.Send(ctx, id, text)
	}
	log.Printf("out %s %dB in %s err=%v", id, len(text), time.Since(start).Round(time.Millisecond), err)
	return err
}

func (r *router) agent(key string, open func() (*agent.Agent, error)) (*agent.Agent, error) {
	if r.agents == nil {
		return nil, errors.New("shutting down")
	}
	if a, ok := r.agents[key]; ok {
		return a, nil
	}
	a, err := open()
	if err == nil {
		r.agents[key] = a
	}
	return a, err
}

func (r *router) thread(id string) (*agent.Agent, error) {
	return r.agent(id, func() (*agent.Agent, error) {
		var typing func(context.Context) error
		if t, ok := r.channel(id).(channel.Typer); ok {
			typing = func(ctx context.Context) error { return t.Typing(ctx, id) }
		}
		opts := []agent.Option{agent.WithTools(r.jobs.Tools(id, "")...)}
		if s, ok := r.channel(id).(channel.Suggester); ok {
			opts = append(opts, agent.WithSuggest(r.users.suggest, func(ctx context.Context, text string) error { return s.Suggest(ctx, id, text) }))
		}
		reply := func(ctx context.Context, text string) error { return r.send(ctx, id, text) }
		return r.users.open(r.ctx, id, "session.jsonl", reply, typing, opts...)
	})
}

// webChat's agent is freed when a turn ends with nothing queued; the next message starts a new one.
func (r *router) webChat(id, chat string) (*agent.Agent, error) {
	key := id + "/" + chat
	return r.agent(key, func() (*agent.Agent, error) {
		var a *agent.Agent
		stream := r.stream(id, chat)
		free := func(u agent.Update) {
			stream(u)
			if u.Type != "turn_end" {
				return
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.agents[key] == a && a.Release() {
				delete(r.agents, key)
			}
		}
		var err error
		suggest := func(_ context.Context, text string) error {
			stream(agent.Update{Type: "suggestion", Text: text})
			return nil
		}
		a, err = r.users.open(r.ctx, id, filepath.Join("chats", chat+".jsonl"), nil, nil,
			agent.WithTools(r.jobs.Tools(id, chat)...), agent.Streaming(free), agent.WithSuggest(r.users.suggest, suggest))
		return a, err
	})
}

// close stops taking messages and waits for every agent to drain its inbox.
func (r *router) close() {
	r.mu.Lock()
	agents := r.agents
	r.agents = nil
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, a := range agents {
		wg.Go(a.Close)
	}
	wg.Wait()
}
