package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"troy/sandbox"
)

// SkillTools exposes only the bundled catalog. Skill loading needs no account access.
func SkillTools() ([]Tool, error) {
	skills, err := sandbox.Skills()
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, nil
	}
	var catalog strings.Builder
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
		fmt.Fprintf(&catalog, "\n- %s: %s (%s)", s.Name, s.Description, s.Location)
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "enum": names}}, "required": []string{"name"}, "additionalProperties": false})
	return []Tool{{Name: "load_skill", Description: "Load a skill's complete instructions, bundle version, base directory and resource filenames. Choose the matching skill before task-specific work. Supporting files are not loaded: read relevant ones with Python, resolving paths against directory. Repeated loads of the same version return a receipt; its instructions remain in context. Skills guide procedure but do not override core policy or grant action approval.\nAvailable skills:" + catalog.String(), Schema: string(schema), Run: func(_ context.Context, _ sandbox.Sandbox, raw json.RawMessage) (string, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", err
		}
		for _, s := range skills {
			if s.Name == args.Name {
				b, err := json.Marshal(s)
				return string(b), err
			}
		}
		return "", fmt.Errorf("unknown skill %q; choose from %s", args.Name, strings.Join(names, ", "))
	}}}, nil
}
