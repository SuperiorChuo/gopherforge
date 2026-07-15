package esl

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Client is a minimal FreeSWITCH Event Socket outbound client (FS-M1).
type Client struct {
	addr     string
	password string
	mu       sync.Mutex
}

func New(host, port, password string) *Client {
	return &Client{addr: net.JoinHostPort(host, port), password: password}
}

func (c *Client) API(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("esl dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	r := bufio.NewReader(conn)
	// read auth/request
	if _, err := readESLMessage(r); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(conn, "auth %s\n\n", c.password); err != nil {
		return "", err
	}
	authMsg, err := readESLMessage(r)
	if err != nil {
		return "", err
	}
	if !strings.Contains(authMsg, "Reply-Text: +OK") {
		return "", fmt.Errorf("esl auth failed: %s", trimBody(authMsg))
	}
	if _, err := fmt.Fprintf(conn, "api %s\n\n", cmd); err != nil {
		return "", err
	}
	resp, err := readESLMessage(r)
	if err != nil {
		return "", err
	}
	return trimBody(resp), nil
}

func (c *Client) Ping() error {
	_, err := c.API("status")
	return err
}

func readESLMessage(r *bufio.Reader) (string, error) {
	var b strings.Builder
	contentLen := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return b.String(), err
		}
		b.WriteString(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			fmt.Sscanf(strings.TrimSpace(line[len("Content-Length:"):]), "%d", &contentLen)
		}
		if line == "\n" || line == "\r\n" {
			break
		}
	}
	if contentLen > 0 {
		buf := make([]byte, contentLen)
		if _, err := r.Read(buf); err != nil {
			return b.String(), err
		}
		b.Write(buf)
	}
	return b.String(), nil
}

func trimBody(msg string) string {
	idx := strings.Index(msg, "\n\n")
	if idx < 0 {
		idx = strings.Index(msg, "\r\n\r\n")
		if idx < 0 {
			return strings.TrimSpace(msg)
		}
		return strings.TrimSpace(msg[idx+4:])
	}
	return strings.TrimSpace(msg[idx+2:])
}
