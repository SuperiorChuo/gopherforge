package mailer

import (
	"strings"
	"testing"
)

func TestBuildMessageEncodesUnicodeSubject(t *testing.T) {
	message := string(buildMessage(
		"alerts@example.com",
		[]string{"ops@example.com"},
		"严重告警: CPU 使用率",
		"告警内容",
	))
	if !strings.Contains(message, "Subject: =?UTF-8?b?") {
		t.Fatalf("message subject is not RFC 2047 encoded: %q", message)
	}
	if !strings.Contains(message, "Content-Transfer-Encoding: 8bit\r\n") {
		t.Fatalf("message is missing UTF-8 transfer encoding: %q", message)
	}
}

func TestBuildMessageSanitizesHeaderNewlines(t *testing.T) {
	message := string(buildMessage(
		"alerts@example.com\r\nBcc: hidden@example.com",
		[]string{"ops@example.com"},
		"Alert\r\nBcc: hidden@example.com",
		"body",
	))
	if strings.Contains(message, "\r\nBcc:") {
		t.Fatalf("message contains an injected header: %q", message)
	}
}
