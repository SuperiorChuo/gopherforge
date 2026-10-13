package esl

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseEventBody(t *testing.T) {
	body := []byte("Event-Name: CHANNEL_ANSWER\r\n" +
		"Unique-ID: abc-123\n" +
		"Caller-Caller-ID-Number: %2B8613800001111\n" +
		"Caller-Destination-Number: 1001\n")
	ev := parseEventBody(body)
	if ev["Event-Name"] != "CHANNEL_ANSWER" {
		t.Errorf("Event-Name = %q", ev["Event-Name"])
	}
	if ev["Caller-Caller-ID-Number"] != "+8613800001111" {
		t.Errorf("url decode failed: %q", ev["Caller-Caller-ID-Number"])
	}
}

func TestFallbackAddressOnlyForComposeServiceName(t *testing.T) {
	if got := eslFallbackAddr("freeswitch", "8021"); got != "host.docker.internal:8021" {
		t.Fatalf("fallback=%q", got)
	}
	if got := eslFallbackAddr("127.0.0.1", "8021"); got != "" {
		t.Fatalf("unexpected fallback=%q", got)
	}
}

func TestListenerHandshakeAndEvent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	eventBody := "Event-Name: CHANNEL_CREATE\nUnique-ID: uuid-1\nCaller-Caller-ID-Number: 1000\n"
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		fmt.Fprint(conn, "Content-Type: auth/request\n\n")
		expectCmd(br, "auth secret")
		fmt.Fprint(conn, "Content-Type: command/reply\nReply-Text: +OK accepted\n\n")
		expectCmd(br, "event plain")
		fmt.Fprint(conn, "Content-Type: command/reply\nReply-Text: +OK event listener enabled plain\n\n")
		fmt.Fprintf(conn, "Content-Length: %d\nContent-Type: text/event-plain\n\n%s", len(eventBody), eventBody)
		time.Sleep(200 * time.Millisecond)
	}()

	got := make(chan map[string]string, 1)
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	l := NewListener(host, port, "secret", []string{"CHANNEL_CREATE"}, func(ev map[string]string) {
		got <- ev
	})
	// 主地址故意拒绝连接，证明 hostnet 回退仍能完成订阅。
	l.addr = "127.0.0.1:1"
	l.fallbackAddr = net.JoinHostPort(host, port)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go l.Run(ctx)

	select {
	case ev := <-got:
		if ev["Unique-ID"] != "uuid-1" || ev["Caller-Caller-ID-Number"] != "1000" {
			t.Errorf("event = %v", ev)
		}
		connected, lastAt, _ := l.Status()
		if !connected || lastAt.IsZero() {
			t.Errorf("status: connected=%v lastAt=%v", connected, lastAt)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

// expectCmd consumes one client command (terminated by a blank line) and
// fails silently if the prefix mismatches — the client side will then time out.
func expectCmd(br *bufio.Reader, prefix string) {
	var first string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if first == "" {
			first = line
		}
	}
	_ = strings.HasPrefix(first, prefix)
}
