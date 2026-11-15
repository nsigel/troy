package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"troy/sandbox"
)

// PythonTask binds interpreter lifetime to one agent loop, including a child loop.
// It shares files, never Python state, with other tasks.
func PythonTask(list []Tool) ([]Tool, func()) {
	list = append([]Tool{}, list...)
	var mu sync.Mutex
	var session *sandbox.Python
	var lost bool
	closeSession := func() {
		if session != nil {
			session.Close()
			session = nil
		}
	}
	for i := range list {
		if list[i].Name != "python" {
			continue
		}
		list[i].RunResult = func(ctx context.Context, sb sandbox.Sandbox, raw json.RawMessage) (Result, error) {
			mu.Lock()
			defer mu.Unlock()
			var args struct {
				Code  string
				Reset bool
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return Result{}, err
			}
			notice := ""
			if args.Reset {
				closeSession()
				lost = false
				notice = "[Python session reset; previous variables and imports cleared.]\n"
			}
			if lost {
				return Result{}, fmt.Errorf("Python session is unavailable; use reset=true to start fresh. Do not assume prior code had no side effects")
			}
			if session == nil {
				var err error
				session, err = sandbox.OpenPython(ctx, sb)
				if err != nil {
					return Result{}, err
				}
				if notice == "" {
					notice = "[New Python task session; variables and imports persist until this task ends.]\n"
				}
			}
			output, raised, err := session.Run(ctx, args.Code)
			if err != nil {
				closeSession()
				lost = true
			}
			return Result{Content: notice + output, Failed: raised}, err
		}
	}
	return list, func() { mu.Lock(); defer mu.Unlock(); closeSession() }
}
