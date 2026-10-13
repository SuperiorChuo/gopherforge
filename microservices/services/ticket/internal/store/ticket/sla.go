package store

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/services/ticket/internal/model"
)

// Default SLA hours by priority (M1.5). Overridable via env:
// TICKET_SLA_URGENT_HOURS / TICKET_SLA_HIGH_HOURS / TICKET_SLA_NORMAL_HOURS / TICKET_SLA_LOW_HOURS
// Set hour to 0 to disable auto due_at for that priority.
func DefaultSLAHours(priority string) int {
	switch priority {
	case model.PriorityUrgent:
		return envHours("TICKET_SLA_URGENT_HOURS", 4)
	case model.PriorityHigh:
		return envHours("TICKET_SLA_HIGH_HOURS", 24)
	case model.PriorityLow:
		return envHours("TICKET_SLA_LOW_HOURS", 168) // 7d
	default: // normal
		return envHours("TICKET_SLA_NORMAL_HOURS", 72) // 3d
	}
}

// DefaultDueAt returns now+SLA when hours>0; nil when disabled.
func DefaultDueAt(priority string, now time.Time) *time.Time {
	h := DefaultSLAHours(priority)
	if h <= 0 {
		return nil
	}
	t := now.Add(time.Duration(h) * time.Hour)
	return &t
}

// SLAPolicy is exposed for UI /ops to show the active defaults.
type SLAPolicy struct {
	UrgentHours int `json:"urgent_hours"`
	HighHours   int `json:"high_hours"`
	NormalHours int `json:"normal_hours"`
	LowHours    int `json:"low_hours"`
}

func CurrentSLAPolicy() SLAPolicy {
	return SLAPolicy{
		UrgentHours: DefaultSLAHours(model.PriorityUrgent),
		HighHours:   DefaultSLAHours(model.PriorityHigh),
		NormalHours: DefaultSLAHours(model.PriorityNormal),
		LowHours:    DefaultSLAHours(model.PriorityLow),
	}
}

func envHours(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
