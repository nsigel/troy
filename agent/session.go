package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"troy/model"
)

// Session is a user's conversation, persisted as append-only JSONL. One with no path (a
// delegated task's) lives only in memory.
type Session struct {
	path     string
	Messages []model.Message
}

func LoadSession(path string) (*Session, error) {
	s := &Session{path: path}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var m model.Message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.Messages = append(s.Messages, m)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return s, s.answerDangling()
}

func (s *Session) answerDangling() error {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role != "assistant" {
			continue
		}
		calls, answered := s.Messages[i].ToolCalls, len(s.Messages)-1-i
		for _, c := range calls[min(answered, len(calls)):] {
			if err := s.Append(model.Message{Role: "tool", ToolCallID: c.ID, Content: "error: interrupted, never finished"}); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func (s *Session) Append(m model.Message) error {
	s.Messages = append(s.Messages, m)
	if s.path == "" {
		return nil
	}
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Reset archives the current session file and starts empty.
func (s *Session) Reset() error {
	s.Messages = nil
	err := os.Rename(s.path, fmt.Sprintf("%s.%d", s.path, time.Now().Unix()))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
