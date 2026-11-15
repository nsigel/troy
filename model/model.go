package model

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role       string           `json:"role"` // system, user, assistant, tool
	Content    string           `json:"content"`
	ToolCalls  []ToolCall       `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Documents  []Document       `json:"documents,omitempty"`
	Skill      *SkillActivation `json:"skill_activation,omitempty"`
	Failed     bool             `json:"failed,omitempty"` // a tool result that failed; local, never sent on the wire
}

// SkillActivation is local session metadata, never accepted from model output or sent on the wire.
type SkillActivation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolCall is the model asking to run a tool with JSON arguments.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Model is anything that can produce the next assistant message. If onText is not nil, Chat
// calls it from the calling goroutine with each piece of text content, in order, as it arrives.
// The returned Message is always the complete assistant message.
type Model interface {
	Chat(ctx context.Context, msgs []Message, tools []ToolDef, onText func(delta string)) (Message, error)
}
