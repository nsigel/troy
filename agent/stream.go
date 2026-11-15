package agent

import "strings"

// Update is one change to a web chat, as the web app receives it (docs/http-api.md).
type Update struct {
	Type   string `json:"type"` // user, turn_start, text, tool_start, tool_end, turn_end or suggestion
	Text   string `json:"text,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Args   string `json:"args,omitempty"`
	Output string `json:"output,omitempty"`
	Failed bool   `json:"failed,omitempty"`
	Taken  int    `json:"taken,omitempty"`
	Error  bool   `json:"error,omitempty"`
}

// Stream receives a web chat's turns as they happen. A tool_end update may come from several
// goroutines at once; the rest come in order. turn_end comes after the turn's last message is
// in the session.
type Stream func(Update)

// Streaming sends the agent's turns to s, for a web chat. A streaming agent never calls reply
// and has no send_message and no slash commands.
func Streaming(s Stream) Option { return func(a *Agent) { a.stream = s } }

func (s Stream) put(u Update) {
	if s != nil {
		s(u)
	}
}

// Split cuts a model message's Markdown into the text updates the web app joins with blank
// lines: at blank lines outside fenced code, leaving out empty parts, NO_REPLY and tool-call
// markup. Splitting the text piece by piece as it streams gives the same parts.
func Split(text string) (parts []string) {
	for {
		part, rest, ok := cut(text)
		if !ok {
			return keep(parts, text)
		}
		parts, text = keep(parts, part), rest
	}
}

// cut splits text at its first complete blank line outside a ``` or ~~~ fence. text starts
// outside a fence, and so does rest.
func cut(text string) (part, rest string, ok bool) {
	fence := ""
	for i := 0; ; {
		n := strings.IndexByte(text[i:], '\n')
		if n < 0 {
			return "", text, false
		}
		line, next := text[i:i+n], i+n+1
		t := strings.TrimLeft(line, " \t")
		switch {
		case fence != "":
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fence = t[:3]
		case strings.TrimSpace(line) == "":
			return text[:i], text[next:], true
		}
		i = next
	}
}

func keep(parts []string, part string) []string {
	part = strings.TrimRight(part, " \t\r\n")
	if t := strings.TrimSpace(part); t == "" || t == noReply || markup.MatchString(part) {
		return parts
	}
	return append(parts, part)
}

// splitter streams one model call's text as Split's parts, each once the blank line ending it
// arrives.
type splitter struct {
	s   Stream
	buf string
}

func (p *splitter) onText() func(string) {
	if p.s == nil {
		return nil
	}
	return p.write
}

func (p *splitter) write(delta string) {
	p.buf += delta
	if !strings.Contains(delta, "\n") {
		return
	}
	for {
		part, rest, ok := cut(p.buf)
		if !ok {
			return
		}
		p.emit(part)
		p.buf = rest
	}
}

func (p *splitter) flush() {
	p.emit(p.buf)
	p.buf = ""
}

func (p *splitter) emit(part string) {
	for _, t := range keep(nil, part) {
		p.s.put(Update{Type: "text", Text: t})
	}
}
