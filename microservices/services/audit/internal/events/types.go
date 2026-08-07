package events

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/logger"
)

const (
	LoginTypePassword int8 = 1
	LoginTypeGithub   int8 = 2
	LoginTypeWechat   int8 = 3
	LoginTypeTOTP     int8 = 4

	loginStatusSuccess int8 = 1
	loginStatusFailed  int8 = 0

	recordTimeout = 5 * time.Second
	messageMaxLen = 255
)

type loginEvent struct {
	UserID    uint   `json:"user_id"`
	TenantID  uint   `json:"tenant_id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	LoginType string `json:"login_type"`
	Reason    string `json:"reason"`
	DeviceID  string `json:"device_id"`
	Timestamp string `json:"timestamp"`
}

func loginTypeCode(loginType string) int8 {
	switch strings.ToLower(loginType) {
	case "oauth:github":
		return LoginTypeGithub
	case "oauth:wechat":
		return LoginTypeWechat
	case "totp":
		return LoginTypeTOTP
	default:
		return LoginTypePassword
	}
}

func parseEventTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func warn(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Warn(message, logger.Err(err))
}

// LoginLogRecorder is the persistence surface the consumer needs.
type LoginLogRecorder interface {
	RecordContext(ctx context.Context, info *systemsvc.LoginInfo) error
}

// loginLogEvent wraps the raw payload sent over Redis pub/sub.
type loginLogEvent struct {
	Subject string `json:"subject"`
	Payload json.RawMessage
}
