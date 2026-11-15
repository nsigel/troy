package agent

import (
	"context"
	"strings"
	"time"

	"troy/model"
)

// suggestTurns is how many recent texts the suggestion model sees.
const suggestTurns = 8

const suggestPrompt = `You predict the next text a USC student will send their assistant, Troy, so they can send it with one key.

Write only what the student would most likely type next, in their voice: short, casual, lowercase, under 12 words. Predict only quick continuations of Troy's last text: picking one of the options it listed, confirming what it asked to confirm, retrying or continuing what it offered, or taking a specific offer it made. Use only details from the conversation; never invent classes, times or requests.

If Troy's last text doesn't set up an obvious reply (an open question only the student can answer, like "what can i help with?" or which class; a plain answer; a sign-off), write exactly NONE. Never write as Troy.`

// WithSuggest calls f with a prediction of the user's next message after each reply.
// It runs after the reply is sent, off the inbox loop, and is skipped when nothing is obvious.
func WithSuggest(m model.Model, f func(ctx context.Context, text string) error) Option {
	return func(a *Agent) { a.suggestModel, a.suggest = m, f }
}

// Suggest predicts the user's next message from the conversation, or "" when nothing is obvious.
func Suggest(ctx context.Context, m model.Model, msgs []model.Message) (string, error) {
	var lines []string
	for _, msg := range msgs {
		switch {
		case msg.Role == "user" && !strings.HasPrefix(msg.Content, "Application context:"):
			if text := trimStamp(msg.Content); strings.HasPrefix(text, noticePrefix) {
				lines = append(lines, "(system notice: "+strings.TrimPrefix(text, noticePrefix)+")")
			} else {
				lines = append(lines, "student: "+text)
			}
		case msg.Role == "assistant" && len(msg.ToolCalls) == 0 && msg.Content != "" && msg.Content != noReply:
			lines = append(lines, "troy: "+msg.Content)
		}
	}
	if len(lines) == 0 || !strings.HasPrefix(lines[len(lines)-1], "troy: ") {
		return "", nil
	}
	lines = lines[max(0, len(lines)-suggestTurns):]
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := m.Chat(ctx, []model.Message{
		{Role: "system", Content: suggestPrompt},
		{Role: "user", Content: strings.Join(lines, "\n\n") + "\n\nThe student's next text:"},
	}, nil, nil)
	if err != nil {
		return "", err
	}
	text := strings.Trim(strings.TrimSpace(out.Content), `"`)
	text = strings.TrimPrefix(text, "student: ")
	if text == "" || strings.EqualFold(strings.Trim(text, "."), "none") || strings.Contains(text, "\n") || len(text) > 120 {
		return "", nil
	}
	return text, nil
}

// trimStamp drops the "[Mon Jan 2 2006, 3:04 PM MST] " loop adds to each user text.
func trimStamp(s string) string {
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "] "); i > 0 && i < 40 {
			s = s[i+2:]
		}
	}
	return s
}
