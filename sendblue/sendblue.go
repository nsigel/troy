// Package sendblue is a minimal client for the Sendblue iMessage/SMS API
// (https://docs.sendblue.com): send a text, show typing, and decode an authenticated webhook.
package sendblue

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxLen is the longest message content Sendblue accepts, in characters.
const MaxLen = 18995

// SecretHeader carries the webhook's configured secret on every delivery.
const SecretHeader = "sb-signing-secret"

// ErrUnauthorized means a webhook request did not carry the expected secret.
var ErrUnauthorized = errors.New("sendblue: webhook secret mismatch")

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Client sends messages from one Sendblue line.
type Client struct {
	KeyID  string // sb-api-key-id
	Secret string // sb-api-secret-key
	From   string // your Sendblue line, E.164
}

// Message is a Sendblue message object, as returned by send and posted to webhooks.
type Message struct {
	Handle       string `json:"message_handle"`
	Content      string `json:"content"`
	Outbound     bool   `json:"is_outbound"`
	Status       string `json:"status"` // RECEIVED for inbound; QUEUED, SENT, DELIVERED, ERROR, ... for outbound
	From         string `json:"from_number"`
	To           string `json:"to_number"`
	Number       string `json:"number"`          // the end user
	Line         string `json:"sendblue_number"` // the Sendblue line involved
	GroupID      string `json:"group_id"`        // empty unless a group chat
	MediaURL     string `json:"media_url"`
	Service      string `json:"service"`       // iMessage, SMS, RCS
	ErrorMessage string `json:"error_message"` // on failure: error key, e.g. PRE_REPLY_LENGTH_EXCEEDED
	ErrorReason  string `json:"error_reason"`  // on failure: human-readable explanation
}

// Send texts content to the E.164 number to and returns the accepted message.
func (c Client) Send(ctx context.Context, to, content string) (Message, error) {
	code, raw, err := c.post(ctx, "send-message", map[string]string{"number": to, "from_number": c.From, "content": content})
	if err != nil {
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil || code/100 != 2 || m.Status == "ERROR" || m.Status == "DECLINED" {
		if m.ErrorMessage != "" {
			return m, fmt.Errorf("sendblue: send: %s %s: %s", m.Status, m.ErrorMessage, m.ErrorReason)
		}
		return m, fmt.Errorf("sendblue: send: %d: %.300s", code, raw)
	}
	return m, nil
}

// Typing shows the typing indicator to the E.164 number to for up to a minute.
// It works only in an existing iMessage conversation.
func (c Client) Typing(ctx context.Context, to string) error {
	code, raw, err := c.post(ctx, "send-typing-indicator", map[string]string{"number": to, "from_number": c.From})
	if err == nil && code/100 != 2 {
		err = fmt.Errorf("sendblue: typing: %d: %.300s", code, raw)
	}
	return err
}

func (c Client) post(ctx context.Context, endpoint string, body any) (int, []byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.sendblue.com/api/"+endpoint, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("sb-api-key-id", c.KeyID)
	req.Header.Set("sb-api-secret-key", c.Secret)
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// ParseWebhook authenticates a webhook request against secret and decodes its message.
// It returns ErrUnauthorized when the secret header is missing or wrong.
func ParseWebhook(r *http.Request, secret string) (Message, error) {
	got := r.Header.Get(SecretHeader)
	if secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		return Message{}, ErrUnauthorized
	}
	var m Message
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m)
	return m, err
}
