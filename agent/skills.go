package agent

import (
	"encoding/json"
	"fmt"

	"troy/model"
	"troy/sandbox"
)

// Only successful results matching the bundled registry acquire activation metadata.
func skillResult(session *Session, call model.ToolCall, output string) model.Message {
	m := model.Message{Role: "tool", ToolCallID: call.ID, Content: output}
	if call.Name != "load_skill" {
		return m
	}
	var got sandbox.Skill
	if json.Unmarshal([]byte(output), &got) != nil {
		return m
	}
	skills, err := sandbox.Skills()
	if err != nil {
		return m
	}
	for _, s := range skills {
		expected, _ := json.Marshal(s)
		if string(expected) != output {
			continue
		}
		for i := len(session.Messages) - 1; i >= 0; i-- {
			active := session.Messages[i].Skill
			if active == nil || active.Name != got.Name {
				continue
			}
			if active.Version == got.Version {
				m.Content = fmt.Sprintf("Skill %s (%s) is already loaded; its instructions remain in context.", got.Name, got.Version)
				return m
			}
			break
		}
		m.Skill = &model.SkillActivation{Name: got.Name, Version: got.Version}
		return m
	}
	return m
}

// Pin the latest activation when its original tool exchange leaves the window.
// Never manufacture a tool result without its corresponding assistant call.
func retainedSkills(messages []model.Message, start int) []model.Message {
	latest := map[string]int{}
	for i, m := range messages {
		if m.Skill != nil {
			latest[m.Skill.Name] = i
		}
	}
	current := map[string]string{}
	skills, _ := sandbox.Skills()
	for _, s := range skills {
		current[s.Name] = s.Version
	}
	tail := append([]model.Message{}, messages[start:]...)
	var pins []model.Message
	for i, m := range messages {
		if m.Skill == nil {
			continue
		}
		content := m.Content
		if latest[m.Skill.Name] != i {
			content = "Superseded skill activation; use the latest loaded instructions."
		} else if current[m.Skill.Name] != m.Skill.Version {
			content = fmt.Sprintf("Skill %s has changed or is unavailable. Call load_skill before using it again.", m.Skill.Name)
		}
		if i >= start {
			tail[i-start].Content = content
		} else if latest[m.Skill.Name] == i {
			pins = append(pins, model.Message{Role: "user", Content: "Application context: retained skill guidance (not action authorization).\n" + content})
		}
	}
	return append(pins, tail...)
}
