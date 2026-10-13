// Package channel provides multi-channel delivery adapters for notify-service (M2).
package channel

import (
	"context"
	"strings"
)

// Payload is rendered content ready for any channel.
type Payload struct {
	TenantID     uint64
	TemplateCode string
	Params       map[string]string
	Title        string
	Content      string
	Link         string
}

// Contact is the recipient's external contact info (from users table).
type Contact struct {
	UserID uint64
	Email  string
	Phone  string
}

// Result is one channel attempt outcome.
type Result struct {
	Channel string `json:"channel"`
	Status  string `json:"status"` // sent | skipped | failed | stub
	Target  string `json:"target,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// EmailSender delivers email; nil/disabled implementations return skipped.
type EmailSender interface {
	Enabled() bool
	Send(ctx context.Context, to string, p Payload) error
	Name() string
}

// SMSSender delivers SMS; stub logs without sending.
type SMSSender interface {
	Enabled() bool
	Send(ctx context.Context, phone string, p Payload) error
	Name() string
}

// VoiceSender enqueues a TTS outbound call; disabled implementations skip.
type VoiceSender interface {
	Enabled() bool
	Send(ctx context.Context, phone string, p Payload) error
	Name() string
}

// ParseChannels splits "inbox,email,sms" (also accepts legacy single value).
func ParseChannels(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"inbox"}
	}
	return out
}

// Has returns whether channels list includes name.
func Has(channels []string, name string) bool {
	for _, c := range channels {
		if c == name {
			return true
		}
	}
	return false
}
