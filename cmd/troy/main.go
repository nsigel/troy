package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // the image may have no zoneinfo

	"troy/agent"
	"troy/model"
	"troy/sandbox"
)

// Every time Troy reads, writes or schedules is LA time, where USC is: no time zones to mix up.
func init() {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	time.Local = la
}

func main() {
	var err error
	switch args := os.Args[1:]; {
	case len(args) >= 1 && len(args) <= 2 && args[0] == "prompt":
		mode := "default"
		if len(args) == 2 {
			mode = args[1]
		}
		err = inspectPrompt(mode)
	case len(args) == 1 && args[0] == "serve":
		err = serve()
	case len(args) == 2 && args[0] == "chat":
		err = chat(args[1])
	case len(args) >= 2 && args[0] == "eval":
		err = eval(args[1], args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: troy prompt [default|code|child] | troy serve | troy chat <name> | troy eval <file> [code|default]...")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "troy:", err)
		os.Exit(1)
	}
}

// chat talks to a running troy serve as user name over the local channel. Interactive, it
// prompts and prints replies whenever they come, with Troy's guess at your next message
// dimmed above the prompt (Enter on an empty line sends it); piped, it sends a line at a
// time, waits for its reply and exits at EOF.
func chat(name string) error {
	conn, err := net.Dial("tcp", env("TROY_LOCAL_ADDR", localAddr))
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := fmt.Fprintln(conn, name); err != nil {
		return err
	}
	replies, suggestions, failed := make(chan string), make(chan string), make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var text string
			var s struct{ Suggestion string }
			if json.Unmarshal(sc.Bytes(), &text) == nil {
				replies <- text
			} else if json.Unmarshal(sc.Bytes(), &s) == nil && s.Suggestion != "" {
				suggestions <- s.Suggestion
			} else {
				failed <- errors.New(sc.Text())
				return
			}
		}
		failed <- errors.New("server closed the connection")
	}()
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	stdin, _ := os.Stdin.Stat()
	interactive := stdin.Mode()&os.ModeCharDevice != 0
	show := func(s string) {
		if interactive {
			fmt.Print(s)
		}
	}
	// Piped, a line waits for its reply, then for a few quiet seconds in case more texts follow.
	var waiting <-chan time.Time
	replied, pending, suggestion := false, false, ""
	show("> ")
	for {
		in := lines
		if waiting != nil {
			in = nil
		}
		select {
		case line, ok := <-in:
			if !ok {
				show("\n")
				return nil
			}
			if strings.TrimSpace(line) == "" && suggestion != "" {
				line = suggestion
			}
			if strings.TrimSpace(line) == "" {
				show("> ")
				continue
			}
			if _, err := fmt.Fprintln(conn, line); err != nil {
				return err
			}
			if suggestion != "" { // the dimmed line above this one
				show("\033[2A\r\033[K> " + line + "\n\033[K")
			}
			suggestion, pending = "", true
			show("troy: …")
			if !interactive {
				waiting = time.After(3 * time.Minute)
			}
		case text := <-replies:
			show("\r\033[K")
			if suggestion != "" { // a reply nobody asked for makes it stale
				show("\033[A\r\033[K")
			}
			fmt.Printf("troy: %s\n", text)
			show("> ")
			pending, suggestion = false, ""
			if !interactive {
				waiting, replied = time.After(5*time.Second), true
			}
		case s := <-suggestions:
			if interactive && !pending { // stale once another message is on its way
				suggestion = s
				show("\r\033[K\033[2m  ↵ " + s + "\033[0m\n> ")
			}
		case err := <-failed:
			show("\n")
			return err
		case <-waiting:
			if !replied {
				return errors.New("no reply within 3 minutes")
			}
			waiting, replied = nil, false
		}
	}
}

// A user id is an E.164 phone number, a lowercased Apple ID email, or a lowercase name like
// "alice". Directories use it as is.
var validID = regexp.MustCompile(`^(\+[0-9]{1,15}|[a-z0-9._%+-]{1,64}@[a-z0-9.-]{1,190}\.[a-z]{2,}|[a-z]{1,32})$`)

// containerSafe keeps the characters Docker allows in a name. The workspace hash in the
// name keeps ids that differ only in dropped characters apart.
var containerSafe = regexp.MustCompile(`[^a-z0-9.-]`)

// users opens a user's agents on their own data dir, workspace, sessions and sandbox.
type users struct {
	model   model.Model
	suggest model.Model // predicts the user's next message on channels that show it
	prompt  string      // the usc section of every agent's system prompt
	root    string      // $TROY_HOME/users
	sandbox string      // docker or local
}

func newUsers() (users, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return users{}, err
	}
	u := users{
		model:   model.OpenRouter{Key: os.Getenv("OPENROUTER_API_KEY"), Model: env("TROY_MODEL", model.Default)},
		suggest: model.OpenRouter{Key: os.Getenv("OPENROUTER_API_KEY"), Model: env("TROY_SUGGEST_MODEL", model.Suggest)},
		root:    filepath.Join(env("TROY_HOME", filepath.Join(home, ".troy")), "users"),
		sandbox: env("TROY_SANDBOX", "docker"),
	}
	if u.sandbox != "docker" && u.sandbox != "local" {
		return users{}, fmt.Errorf("unknown TROY_SANDBOX %q (want docker or local)", u.sandbox)
	}
	u.prompt, err = uscPrompt()
	return u, err
}

// open starts an agent for id on the session file at path inside their data dir.
func (u users) open(ctx context.Context, id, path string, reply func(context.Context, string) error, typing func(context.Context) error, opts ...agent.Option) (*agent.Agent, error) {
	sb, err := u.userSandbox(id)
	if err != nil {
		return nil, err
	}
	session, err := agent.LoadSession(filepath.Join(u.root, id, path))
	if err != nil {
		return nil, err
	}
	return agent.New(ctx, u.model, sb, session, withStudent(u.prompt, sb, filepath.Join(u.root, id, studentFile)), reply, typing, opts...)
}

// userSandbox is where id's commands run: the agent's shell and their monitors.
func (u users) userSandbox(id string) (sandbox.Sandbox, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("invalid user id %q", id)
	}
	userDir := filepath.Join(u.root, id)
	workspace := filepath.Join(userDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	uscDir := filepath.Join(userDir, "usc")
	if u.sandbox == "docker" {
		name := fmt.Sprintf("troy-%s-%08x", containerSafe.ReplaceAllString(id, ""), crc32.ChecksumIEEE([]byte(workspace)))
		return sandbox.Docker{Name: name, Dir: workspace, USC: uscDir, Timeout: 2 * time.Minute, Idle: 5 * time.Minute}, nil
	}
	return sandbox.Local{Dir: workspace, USC: uscDir, Timeout: 2 * time.Minute}, nil
}

func required(names ...string) error {
	var missing []string
	for _, n := range names {
		if os.Getenv(n) == "" {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
