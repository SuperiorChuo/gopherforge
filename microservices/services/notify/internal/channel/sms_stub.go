package channel

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// StubSMS logs outbound SMS without calling a provider — 它从不真发短信。
// 是否启用由 config.EffectiveSMSMode 决定：NOTIFY_SMS_MODE=stub 且处于演示模式
// （NOTIFY_DEMO_MODE，生产默认关）才会被传入非空 Mode。真实 provider
// （阿里云/腾讯云）尚未实现，将来按同一接口替换。
type StubSMS struct {
	// Mode: "" | "off" = disabled; "stub" = log only
	Mode string
}

func NewStubSMS(mode string) *StubSMS {
	return &StubSMS{Mode: strings.ToLower(strings.TrimSpace(mode))}
}

func (s *StubSMS) Name() string { return "stub" }

func (s *StubSMS) Enabled() bool {
	return s != nil && (s.Mode == "stub" || s.Mode == "log" || s.Mode == "1" || s.Mode == "true")
}

func (s *StubSMS) Send(_ context.Context, phone string, p Payload) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return fmt.Errorf("empty phone")
	}
	log.Printf("notify sms stub → %s | %s | %s", phone, p.Title, truncate(p.Content, 80))
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
