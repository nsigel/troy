package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"troy/agent"
	"troy/sandbox"
)

const studentFile = "student.md"

func withStudent(prompt string, sb sandbox.Sandbox, path string) func(context.Context) agent.Context {
	return func(ctx context.Context) agent.Context {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
			if b, err := os.ReadFile(path); err == nil {
				return agent.Context{Instructions: prompt, Student: string(b)}
			}
		}
		text, err := studentSection(ctx, sb)
		if err != nil {
			log.Println("student section:", err)
			return agent.Context{Instructions: prompt, Student: "Student account context unavailable; registered classes and connection state are unknown. Use the account tools if needed, and report sign-in failures honestly."}
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			log.Println("student section:", err)
		}
		return agent.Context{Instructions: prompt, Student: text}
	}
}

func currentTerm(t time.Time) (code, name string) {
	season := 3
	switch {
	case t.Month() <= time.May:
		season = 1
	case t.Month() <= time.July:
		season = 2
	}
	return fmt.Sprintf("%d%d", t.Year(), season), fmt.Sprintf("%s %d", []string{"Spring", "Summer", "Fall"}[season-1], t.Year())
}

func studentSection(ctx context.Context, sb sandbox.Sandbox) (string, error) {
	var me struct {
		First string `json:"first_name"`
		Last  string `json:"last_name"`
	}
	var courses []struct {
		ID            int
		Code, Section *string
	}
	if err := uscJSON(ctx, sb, &me, "brightspace whoami"); err != nil {
		return "", err
	}
	if err := uscJSON(ctx, sb, &courses, "brightspace courses"); err != nil {
		return "", err
	}
	term, name := currentTerm(time.Now())
	var b strings.Builder
	fmt.Fprintf(&b, "# student\n\n%s %s. Registered classes for %s (term %s), from webreg as of %s:\n", me.First, me.Last, name, term, time.Now().Format("Jan 2"))
	var bin struct {
		Courses []struct {
			Code, Title string
			Sections    []struct {
				ID                                  int
				Type                                string
				Times, Days, Locations, Instructors []string
				CourseBin                           struct{ Registered bool } `json:"course_bin"`
			}
		}
	}
	if err := uscJSON(ctx, sb, &bin, "webreg bin --term "+term); err != nil {
		return "", err
	}
	// Codes differ in trailing letters (webreg ECON-352x is Brightspace ECON-352), and a
	// Brightspace section starts with its term, e.g. 20263_14721.
	base := func(code string) string { return strings.TrimRightFunc(code, unicode.IsLetter) }
	for _, c := range bin.Courses {
		var sections []string
		for _, s := range c.Sections {
			if !s.CourseBin.Registered {
				continue
			}
			var meets []string
			for i := range s.Days {
				meet := s.Days[i]
				if i < len(s.Times) {
					meet += " " + s.Times[i]
				}
				if i < len(s.Locations) {
					meet += " in " + s.Locations[i]
				}
				meets = append(meets, meet)
			}
			line := fmt.Sprintf("  - %s %d: %s", s.Type, s.ID, strings.Join(meets, "; "))
			if len(s.Instructors) > 0 {
				line += " (" + strings.Join(s.Instructors, "; ") + ")"
			}
			sections = append(sections, line)
		}
		if sections == nil {
			continue
		}
		site := "no Brightspace site"
		for _, bs := range courses {
			if bs.Code != nil && bs.Section != nil && base(*bs.Code) == base(c.Code) && strings.HasPrefix(*bs.Section, term) {
				site = fmt.Sprintf("Brightspace course %d", bs.ID)
			}
		}
		fmt.Fprintf(&b, "- %s %s, %s\n%s\n", c.Code, c.Title, site, strings.Join(sections, "\n"))
	}
	return b.String() + "\n", nil
}

func uscJSON(ctx context.Context, sb sandbox.Sandbox, v any, args string) error {
	out, code, err := sb.Exec(ctx, "usc "+args+" --compact 2>/dev/null")
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("usc %s: exit %d", args, code)
	}
	return json.Unmarshal([]byte(out), v)
}
