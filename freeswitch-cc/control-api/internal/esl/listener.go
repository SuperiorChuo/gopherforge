package esl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Listener keeps a persistent inbound connection to the FreeSWITCH event
// socket, subscribes to the given events and delivers each parsed event to
// handler (FS-M2). Run reconnects with capped backoff until ctx is cancelled.
type Listener struct {
	addr         string
	fallbackAddr string
	password     string
	events       []string
	handler      func(map[string]string)

	mu          sync.Mutex
	connected   bool
	lastEventAt time.Time
	lastError   string
}

func NewListener(host, port, password string, events []string, handler func(map[string]string)) *Listener {
	return &Listener{
		addr:         net.JoinHostPort(host, port),
		fallbackAddr: eslFallbackAddr(host, port),
		password:     password,
		events:       events,
		handler:      handler,
	}
}

// Status reports connection state for /ready and /v1/esl/status.
func (l *Listener) Status() (connected bool, lastEventAt time.Time, lastError string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.connected, l.lastEventAt, l.lastError
}

func (l *Listener) setState(connected bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.connected = connected
	if err != nil {
		l.lastError = err.Error()
	} else if connected {
		l.lastError = ""
	}
}

// Run blocks until ctx is cancelled, redialing with capped backoff.
func (l *Listener) Run(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		start := time.Now()
		err := l.listenOnce(ctx)
		l.setState(false, err)
		if ctx.Err() != nil {
			return
		}
		// A connection that survived a while earns a fresh backoff.
		if time.Since(start) > time.Minute {
			backoff = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (l *Listener) listenOnce(ctx context.Context) error {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", l.addr)
	if err != nil && l.fallbackAddr != "" {
		conn, err = d.DialContext(ctx, "tcp", l.fallbackAddr)
	}
	if err != nil {
		return fmt.Errorf("esl dial: %w", err)
	}
	defer conn.Close()
	// Blocking reads below don't observe ctx; close the socket to unblock.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	br := bufio.NewReader(conn)
	greet, _, err := readMessage(br)
	if err != nil {
		return fmt.Errorf("esl greeting: %w", err)
	}
	if ct := greet["Content-Type"]; ct != "auth/request" {
		return fmt.Errorf("esl unexpected greeting %q", ct)
	}
	if _, err := fmt.Fprintf(conn, "auth %s\n\n", l.password); err != nil {
		return err
	}
	reply, _, err := readMessage(br)
	if err != nil {
		return fmt.Errorf("esl auth reply: %w", err)
	}
	if !strings.HasPrefix(reply["Reply-Text"], "+OK") {
		return fmt.Errorf("esl auth failed: %s", reply["Reply-Text"])
	}
	if _, err := fmt.Fprintf(conn, "event plain %s\n\n", strings.Join(l.events, " ")); err != nil {
		return err
	}
	reply, _, err = readMessage(br)
	if err != nil {
		return fmt.Errorf("esl subscribe reply: %w", err)
	}
	if !strings.HasPrefix(reply["Reply-Text"], "+OK") {
		return fmt.Errorf("esl subscribe failed: %s", reply["Reply-Text"])
	}

	l.setState(true, nil)
	for {
		hdr, body, err := readMessage(br)
		if err != nil {
			return fmt.Errorf("esl read: %w", err)
		}
		switch hdr["Content-Type"] {
		case "text/event-plain":
			ev := parseEventBody(body)
			l.mu.Lock()
			l.lastEventAt = time.Now()
			l.mu.Unlock()
			if l.handler != nil {
				l.handler(ev)
			}
		case "text/disconnect-notice":
			return fmt.Errorf("esl disconnect notice")
		}
	}
}

// readMessage reads one ESL frame: MIME-ish headers up to a blank line plus
// an optional Content-Length body.
func readMessage(r *bufio.Reader) (map[string]string, []byte, error) {
	headers := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	var body []byte
	if n, _ := strconv.Atoi(headers["Content-Length"]); n > 0 {
		body = make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, nil, err
		}
	}
	return headers, body, nil
}

// parseEventBody decodes a text/event-plain body: "Key: value" lines with
// URL-encoded values (PathUnescape — literal '+' must survive in numbers).
func parseEventBody(body []byte) map[string]string {
	ev := make(map[string]string, 64)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if dec, err := url.PathUnescape(v); err == nil {
			v = dec
		}
		ev[strings.TrimSpace(k)] = v
	}
	return ev
}
