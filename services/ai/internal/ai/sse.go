package ai

import (
	"bufio"
	"io"
	"strings"
)

// sseEvent is one parsed Server-Sent Event.
type sseEvent struct {
	event string
	data  string
}

// readSSE parses a text/event-stream body and invokes handle once per event.
// Returning an error from handle stops the read and propagates the error.
// Multi-line data fields are joined with newlines per the SSE specification.
func readSSE(r io.Reader, handle func(sseEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var event string
	var data []string

	flush := func() error {
		if event == "" && len(data) == 0 {
			return nil
		}
		err := handle(sseEvent{event: event, data: strings.Join(data, "\n")})
		event = ""
		data = data[:0]
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// Comment line; ignore.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}
