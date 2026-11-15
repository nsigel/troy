package api

import (
	"bufio"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"troy/agent"
	"troy/model"
)

const (
	maxMessageLength = 4000 // characters
	subscriberBuffer = 256  // events a slow browser may fall behind before it's dropped
	heartbeat        = 25 * time.Second
	maxTitle         = 60
	maxPreview       = 120
)

var (
	validChatID = regexp.MustCompile(`^[a-z2-7]{10}$`)
	stamp       = regexp.MustCompile(`^\[\w{3} \w{3} \d{1,2} \d{4}, \d{1,2}:\d{2} [AP]M [^\]]*\] `)
)

// A chatMessage is the web app's view of the conversation: a user's text, or one assistant
// reply as ordered text and tool parts.
type chatMessage struct {
	Role  string  `json:"role"` // user or assistant
	Text  string  `json:"text,omitempty"`
	Parts []*part `json:"parts,omitempty"`
}

type part struct {
	Type   string `json:"type"` // text or tool
	Text   string `json:"text,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Args   string `json:"args,omitempty"`
	Output string `json:"output,omitempty"`
	Failed bool   `json:"failed,omitempty"`
	Status string `json:"status,omitempty"` // running or done
}

// chat is a conversation the hub is following: its transcript, the messages said but not yet
// taken by a turn, and the browsers watching it.
type chat struct {
	messages []chatMessage
	pending  []string
	busy     bool
	fresh    bool // the turn's reply starts a new assistant message
	subs     map[chan []byte]struct{}
}

// hub holds every chat with a subscriber, a turn running or a message waiting. Applying an
// update, broadcasting it and subscribing happen under mu, so a snapshot and the updates after
// it never overlap or leave a gap. A chat's predicted next message outlives the chat being
// followed, since it arrives after the turn, often with no one watching.
type hub struct {
	mu          sync.Mutex
	chats       map[string]*chat
	suggestions map[string]string
	released    int
}

func (a *API) chatPath(phoneNumber, id string) string {
	return filepath.Join(a.Users, phoneNumber, "chats", id+".jsonl")
}

// lock locks the hub and returns the chat at path, loading it from its file first if the hub
// isn't following it. A chat's agent writes the file only during a turn, and the hub follows a
// chat from the message that starts a turn until the turn ends, so if no chat was released
// while the file loaded, the file is still current.
func (h *hub) lock(path string) *chat {
	for {
		h.mu.Lock()
		if c, ok := h.chats[path]; ok {
			return c
		}
		released := h.released
		h.mu.Unlock()
		messages := load(path)
		h.mu.Lock()
		if c, ok := h.chats[path]; ok {
			return c
		}
		if h.released == released {
			if h.chats == nil {
				h.chats = map[string]*chat{}
			}
			c := &chat{messages: messages, pending: []string{}, subs: map[chan []byte]struct{}{}}
			h.chats[path] = c
			return c
		}
		h.mu.Unlock()
	}
}

func (h *hub) release(path string, c *chat) {
	if len(c.subs) == 0 && !c.busy && len(c.pending) == 0 {
		delete(h.chats, path)
		h.released++
	}
}

// say hands text to the chat's agent and, if it took it, publishes it. Doing both under mu
// keeps pending messages in inbox order, before the turn that takes them starts.
func (h *hub) say(path, text string, a *agent.Agent) bool {
	c := h.lock(path)
	defer h.mu.Unlock()
	if !a.TryDeliver(agent.Event{Text: text}) {
		h.release(path, c)
		return false
	}
	delete(h.suggestions, path)
	c.apply(agent.Update{Type: "user", Text: text})
	h.broadcast(path, c, agent.Update{Type: "user", Text: text})
	return true
}

// notice hands a system notice to the chat's agent. Marking the chat busy keeps the hub
// following it until the turn that takes the notice ends.
func (h *hub) notice(path, notice string, a *agent.Agent) bool {
	c := h.lock(path)
	defer h.mu.Unlock()
	ok := a.TryDeliver(agent.Event{Notice: notice})
	c.busy = c.busy || ok
	h.release(path, c)
	return ok
}

// publish applies an update from the chat's agent. A turn that failed may have streamed text
// that never reached the file, so then subscribers get a fresh snapshot from the file instead.
func (h *hub) publish(path string, u agent.Update) {
	var fresh []chatMessage
	if u.Type == "turn_end" && u.Error {
		fresh = load(path)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.chats[path]
	if u.Type == "suggestion" {
		if ok && (c.busy || len(c.pending) > 0) {
			return
		}
		if h.suggestions == nil {
			h.suggestions = map[string]string{}
		}
		h.suggestions[path] = u.Text
	}
	if !ok {
		return
	}
	c.apply(u)
	if fresh == nil {
		h.broadcast(path, c, u)
		return
	}
	c.messages = fresh
	h.broadcast(path, c, c.snapshot(h.suggestions[path], true))
}

func (h *hub) broadcast(path string, c *chat, v any) {
	data, _ := json.Marshal(v)
	for ch := range c.subs {
		select {
		case ch <- data:
		default:
			close(ch)
			delete(c.subs, ch)
		}
	}
	h.release(path, c)
}

func (h *hub) subscribe(path string) chan []byte {
	c := h.lock(path)
	defer h.mu.Unlock()
	ch := make(chan []byte, subscriberBuffer)
	data, _ := json.Marshal(c.snapshot(h.suggestions[path], false))
	ch <- data
	c.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(path string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.chats[path]; ok {
		if _, ok := c.subs[ch]; ok {
			close(ch)
			delete(c.subs, ch)
		}
		h.release(path, c)
	}
}

func (h *hub) busy(path string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.chats[path]
	return ok && c.busy
}

func (c *chat) snapshot(suggestion string, failed bool) any {
	return map[string]any{"type": "snapshot", "busy": c.busy, "messages": c.messages, "pending": c.pending, "suggestion": suggestion, "error": failed}
}

// forget drops a deleted chat's suggestion and reports whether Troy is replying in it.
func (h *hub) forget(path string) (busy bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.suggestions, path)
	c, ok := h.chats[path]
	return ok && (c.busy || len(c.pending) > 0)
}

// apply is the web app's reducer: the browser applies the same updates to its snapshot.
func (c *chat) apply(u agent.Update) {
	switch u.Type {
	case "user":
		c.pending = append(c.pending, u.Text)
	case "turn_start":
		n := min(u.Taken, len(c.pending))
		if n > 0 {
			c.messages = append(c.messages, chatMessage{Role: "user", Text: strings.Join(c.pending[:n], "\n")})
			c.pending = c.pending[n:]
		}
		c.busy, c.fresh = true, true
	case "text":
		r := c.reply()
		if n := len(r.Parts); n > 0 && r.Parts[n-1].Type == "text" {
			r.Parts[n-1].Text += "\n\n" + u.Text
		} else {
			r.Parts = append(r.Parts, &part{Type: "text", Text: u.Text})
		}
	case "tool_start":
		c.reply().Parts = append(c.reply().Parts, &part{Type: "tool", ID: u.ID, Name: u.Name, Args: u.Args, Status: "running"})
	case "tool_end":
		if p := c.tool(u.ID); p != nil {
			p.Output, p.Failed, p.Status = u.Output, u.Failed, "done"
		}
	case "turn_end":
		c.busy = false
	}
}

func (c *chat) reply() *chatMessage {
	if n := len(c.messages); n > 0 && c.messages[n-1].Role == "assistant" && !c.fresh {
		return &c.messages[n-1]
	}
	c.fresh = false
	c.messages = append(c.messages, chatMessage{Role: "assistant"})
	return &c.messages[len(c.messages)-1]
}

func (c *chat) tool(id string) *part {
	for i := len(c.messages) - 1; i >= 0; i-- {
		for _, p := range c.messages[i].Parts {
			if p.Type == "tool" && p.ID == id {
				return p
			}
		}
	}
	return nil
}

// load builds a transcript from a session file, stopping at a line it can't parse.
func load(path string) []chatMessage {
	c := &chat{messages: []chatMessage{}}
	readSession(path, func(m model.Message) bool {
		switch m.Role {
		case "user":
			if text, ok := userText(m); ok {
				c.messages = append(c.messages, chatMessage{Role: "user", Text: text})
			}
			c.fresh = true
		case "assistant":
			for _, t := range agent.Split(m.Content) {
				c.apply(agent.Update{Type: "text", Text: t})
			}
			for _, call := range m.ToolCalls {
				c.apply(agent.Update{Type: "tool_start", ID: call.ID, Name: call.Name, Args: call.Args})
			}
		case "tool":
			c.apply(agent.Update{Type: "tool_end", ID: m.ToolCallID, Output: m.Content, Failed: m.Failed})
		}
		return true
	})
	return c.messages
}

func readSession(path string, each func(model.Message) bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64*1024*1024)
	for sc.Scan() {
		var m model.Message
		if json.Unmarshal(sc.Bytes(), &m) != nil || !each(m) {
			return
		}
	}
}

func userText(m model.Message) (string, bool) {
	text := stamp.ReplaceAllString(m.Content, "")
	return text, !strings.HasPrefix(text, "[system notice")
}

// ChatNotice hands a system notice to the agent of a user's web chat, whose reply streams into
// the chat. It reports false if there is no such chat or the agent couldn't take it.
func (a *API) ChatNotice(phoneNumber, chatID, notice string) bool {
	path := a.chatPath(phoneNumber, chatID)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return a.Say(phoneNumber, chatID, notice, func(ag *agent.Agent) bool { return a.chats.notice(path, notice, ag) }) == nil
}

// Stream is where the agent of a user's web chat sends its turns.
func (a *API) Stream(phoneNumber, chatID string) agent.Stream {
	path := a.chatPath(phoneNumber, chatID)
	return func(u agent.Update) { a.chats.publish(path, u) }
}

// CloseStreams ends every open chat event stream, for shutting down.
func (a *API) CloseStreams() { close(a.closing) }

type chatSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview"`
	UpdatedAt time.Time `json:"updated_at"`
	Busy      bool      `json:"busy"`
}

func (a *API) listChats(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	entries, _ := os.ReadDir(filepath.Join(a.Users, phoneNumber, "chats"))
	chats := []chatSummary{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".jsonl")
		info, err := e.Info()
		if !ok || !validChatID.MatchString(id) || err != nil {
			continue
		}
		path := a.chatPath(phoneNumber, id)
		s := chatSummary{ID: id, UpdatedAt: info.ModTime(), Busy: a.chats.busy(path)}
		for _, m := range load(path) {
			if m.Role == "user" && s.Title == "" {
				s.Title = title(m.Text)
			}
			for _, p := range m.Parts {
				if p.Type == "text" {
					s.Preview = preview(p.Text)
				}
			}
		}
		chats = append(chats, s)
	}
	sort.Slice(chats, func(i, j int) bool { return chats[i].UpdatedAt.After(chats[j].UpdatedAt) })
	writeJSON(w, http.StatusOK, chats)
}

func title(text string) string { return clip(text, maxTitle) }

// preview is the start of Troy's latest text, as plain words.
func preview(text string) string {
	return clip(strings.NewReplacer("*", "", "_", "", "`", "", "#", "", ">", "", "|", "").Replace(text), maxPreview)
}

func clip(text string, n int) string {
	t := []rune(strings.Join(strings.Fields(text), " "))
	if len(t) > n {
		return strings.TrimSpace(string(t[:n-1])) + "…"
	}
	return string(t)
}

func (a *API) createChat(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	raw := make([]byte, 7)
	rand.Read(raw)
	id := strings.ToLower(base32.StdEncoding.EncodeToString(raw))[:10]
	path := a.chatPath(phoneNumber, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Could not create the chat.")
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Could not create the chat.")
		return
	}
	f.Close()
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// chat returns the id in the request's path if the signed-in user has that chat, else answers 404.
func (a *API) chat(w http.ResponseWriter, r *http.Request, phoneNumber string) (string, bool) {
	id := r.PathValue("id")
	if validChatID.MatchString(id) {
		if _, err := os.Stat(a.chatPath(phoneNumber, id)); err == nil {
			return id, true
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "There is no such chat.")
	return "", false
}

func (a *API) deleteChat(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	id, ok := a.chat(w, r, phoneNumber)
	if !ok {
		return
	}
	path := a.chatPath(phoneNumber, id)
	if a.chats.forget(path) {
		writeError(w, http.StatusConflict, "busy", "Troy is still replying in this chat.")
		return
	}
	if err := os.Remove(path); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Could not delete the chat.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) sendChatMessage(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	id, ok := a.chat(w, r, phoneNumber)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body)
	text := strings.TrimSpace(body.Text)
	if text == "" || len([]rune(text)) > maxMessageLength {
		writeError(w, http.StatusBadRequest, "invalid_request", "Write a message of at most 4000 characters.")
		return
	}
	path := a.chatPath(phoneNumber, id)
	if err := a.Say(phoneNumber, id, text, func(ag *agent.Agent) bool { return a.chats.say(path, text, ag) }); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Troy couldn't take the message; try again.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) chatEvents(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	id, ok := a.chat(w, r, phoneNumber)
	if !ok {
		return
	}
	path := a.chatPath(phoneNumber, id)
	ch := a.chats.subscribe(path)
	defer a.chats.unsubscribe(path, ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case <-a.closing:
			return
		case <-tick.C:
			_, err = w.Write([]byte(": ping\n\n"))
		case data, ok := <-ch:
			if !ok {
				return
			}
			_, err = w.Write([]byte("data: " + string(data) + "\n\n"))
		}
		if err != nil || rc.Flush() != nil {
			return
		}
	}
}
