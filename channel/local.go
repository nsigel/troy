package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

var localUser = regexp.MustCompile(`^[a-z]{1,32}$`)

// Local is a line protocol over TCP for clients on the same machine, like troy chat. There
// is no auth, so Addr must be loopback. A client sends a user id (a lowercase name), then one
// message per line; every reply to that user goes to each of its connections as one line
// holding a JSON string, and a predicted next message as a line holding {"suggestion": text}. A line that isn't a JSON string is an error, and the server hangs up.
type Local struct {
	Addr  string
	mu    sync.Mutex
	conns map[string]map[net.Conn]bool
}

func (l *Local) Listen(ctx context.Context, handle func(Message)) error {
	ln, err := net.Listen("tcp", l.Addr)
	if err != nil {
		return err
	}
	log.Printf("local channel on %s", l.Addr)
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		conn, err := ln.Accept()
		if ctx.Err() != nil {
			return nil
		} else if err != nil {
			return err
		}
		go l.serve(conn, handle)
	}
}

func (l *Local) serve(conn net.Conn, handle func(Message)) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !sc.Scan() {
		return
	}
	id := strings.TrimSpace(sc.Text())
	if !localUser.MatchString(id) {
		fmt.Fprintf(conn, "invalid local user id %q: use a lowercase name\n", id)
		return
	}
	l.mu.Lock()
	if l.conns == nil {
		l.conns = map[string]map[net.Conn]bool{}
	}
	if l.conns[id] == nil {
		l.conns[id] = map[net.Conn]bool{}
	}
	l.conns[id][conn] = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.conns[id], conn)
		l.mu.Unlock()
	}()
	for sc.Scan() {
		if text := strings.TrimSpace(sc.Text()); text != "" {
			handle(Message{UserID: id, Text: text})
		}
	}
}

// Send writes text to every connection userID has open; with none, the reply is dropped.
func (l *Local) Send(_ context.Context, userID, text string) error {
	line, _ := json.Marshal(text)
	return l.write(userID, line)
}

func (l *Local) Suggest(_ context.Context, userID, text string) error {
	line, _ := json.Marshal(map[string]string{"suggestion": text})
	return l.write(userID, line)
}

func (l *Local) write(userID string, line []byte) error {
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.conns[userID]) == 0 {
		return errors.New("no connection open")
	}
	var errs []error
	for conn := range l.conns[userID] {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(line); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
