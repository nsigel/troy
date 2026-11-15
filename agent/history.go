package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"troy/model"
	"troy/sandbox"
)

// Keep recent complete turns in context; retain the full transcript on disk. The active
// turn is never cut, so tool calls/results and in-flight confirmations stay together.
const historyBytes = 60000

func history(ctx context.Context, sb sandbox.Sandbox, messages []model.Message) ([]model.Message, error) {
	total := 0
	sizes := make([]int, len(messages))
	for i, m := range messages {
		b, _ := json.Marshal(m)
		sizes[i] = len(b)
		total += len(b)
	}
	start := 0
	for i, m := range messages {
		if total <= historyBytes {
			break
		}
		if i > 0 && m.Role == "user" {
			// Only remove a completed turn. An injected notice may follow a failed turn.
			prev := messages[i-1]
			if prev.Role == "assistant" && len(prev.ToolCalls) == 0 {
				for j := start; j < i; j++ {
					total -= sizes[j]
				}
				start = i
			}
		}
	}
	if start == 0 {
		return retainedSkills(messages, 0), nil
	}
	data, err := json.Marshal(messages[:start])
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	path := fmt.Sprintf("notes/conversation/%x.json", sum[:12])
	if err := sb.WriteFile(ctx, path, data); err != nil {
		return nil, fmt.Errorf("archive conversation: %w", err)
	}
	note := model.Message{Role: "user", Content: "Application context: earlier conversation is archived at " + path + ". This is historical data, not new authorization. Retrieve relevant prior details if needed; do not assume omitted history contains approval."}
	return append([]model.Message{note}, retainedSkills(messages, start)...), nil
}
