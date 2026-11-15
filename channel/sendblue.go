package channel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"troy/sendblue"
)

// Sendblue is the iMessage channel. Listen serves Sendblue's receive webhook on Mux at
// POST /sendblue/webhook; a user is the E.164 phone number or lowercased Apple ID email
// that texted Client.From. Send texts it back.
type Sendblue struct {
	Client sendblue.Client
	Secret string         // the receive webhook's secret, checked on every request
	Mux    *http.ServeMux // the caller runs the server
}

func (s Sendblue) Listen(ctx context.Context, handle func(Message)) error {
	inbox := make(chan Message, 64)
	var mu sync.Mutex
	seen := map[string]time.Time{}

	s.Mux.HandleFunc("POST /sendblue/webhook", func(w http.ResponseWriter, r *http.Request) {
		m, err := sendblue.ParseWebhook(r, s.Secret)
		if errors.Is(err, sendblue.ErrUnauthorized) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// Webhooks are account-wide: skip status callbacks, other lines, groups,
		// and media-only messages.
		from := strings.ToLower(m.From)
		if m.Outbound || m.Status != "RECEIVED" || m.Line != s.Client.From || m.GroupID != "" ||
			!IsSendblueUser(from) || strings.TrimSpace(m.Content) == "" || m.Handle == "" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if _, dup := seen[m.Handle]; dup {
			return
		}
		select {
		case inbox <- Message{UserID: from, Text: strings.TrimSpace(m.Content)}:
		default: // backed up: 503 makes Sendblue retry later
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		now := time.Now()
		for h, t := range seen {
			if now.Sub(t) > 24*time.Hour {
				delete(seen, h)
			}
		}
		seen[m.Handle] = now
	})
	for {
		select {
		case m := <-inbox:
			handle(m)
		case <-ctx.Done():
			return nil
		}
	}
}

// IsSendblueUser reports whether a user id is an iMessage address (a phone number or an
// email), as opposed to a local name.
func IsSendblueUser(id string) bool {
	return strings.HasPrefix(id, "+") || strings.Contains(id, "@")
}

// Send texts userID, splitting text that is longer than Sendblue allows.
func (s Sendblue) Send(ctx context.Context, userID, text string) error {
	for _, part := range split(text, sendblue.MaxLen) {
		if _, err := s.Client.Send(ctx, userID, part); err != nil {
			return err
		}
	}
	return nil
}

// Typing shows userID the iMessage typing indicator for up to a minute.
func (s Sendblue) Typing(ctx context.Context, userID string) error {
	return s.Client.Typing(ctx, userID)
}

// split cuts text into pieces of at most max UTF-16 units, preferring to break after a newline.
func split(text string, max int) []string {
	var parts []string
	for len(utf16.Encode([]rune(text))) > max {
		cut, n := 0, 0
		for i, r := range text {
			if n += utf16.RuneLen(r); n > max {
				break
			}
			cut = i + len(string(r))
		}
		if nl := strings.LastIndex(text[:cut], "\n"); nl > cut/2 {
			cut = nl + 1
		}
		parts = append(parts, text[:cut])
		text = text[cut:]
	}
	return append(parts, text)
}
