package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/runtimeconfig"
)

// MultiChannelNotifier fans a firing/resolved event out to every channel the
// rule opted into (rule.NotifyChannels; empty = all configured). The aggregate
// result is one of sent / failed / skipped. Delivery metadata per event stays
// in monitor_alert_events, exactly like the single email channel before.
type MultiChannelNotifier struct {
	channels map[string]AlertNotifier
}

func NewMultiChannelNotifier() *MultiChannelNotifier {
	return &MultiChannelNotifier{channels: map[string]AlertNotifier{
		"email":   NewAlertEmailNotifier(nil, runtimeconfig.DefaultEmailNotificationReader()),
		"station": NewStationNotifier(),
		"wecom":   NewWeComNotifier(),
	}}
}

func (m *MultiChannelNotifier) NotifyContext(ctx context.Context, rule *model.MonitorAlertRule, event *model.MonitorAlertEvent) AlertNotification {
	if m == nil {
		return AlertNotification{Status: AlertNotifySkipped, NotifiedAt: time.Now().UTC()}
	}
	names := rule.NotifyChannels
	if len(names) == 0 {
		names = []string{"email", "station", "wecom"}
	}
	sent, failed := false, false
	errs := make([]string, 0, 2)
	for _, name := range names {
		notifier := m.channels[name]
		if notifier == nil {
			continue
		}
		result := notifier.NotifyContext(ctx, rule, event)
		switch result.Status {
		case AlertNotifySent:
			sent = true
		case AlertNotifyFailed:
			failed = true
			if result.Error != "" {
				errs = append(errs, fmt.Sprintf("%s: %s", name, result.Error))
			}
		}
	}
	now := time.Now().UTC()
	switch {
	case sent:
		return AlertNotification{Status: AlertNotifySent, NotifiedAt: now}
	case failed:
		return AlertNotification{Status: AlertNotifyFailed, Error: strings.Join(errs, "; "), NotifiedAt: now}
	default:
		return AlertNotification{Status: AlertNotifySkipped, NotifiedAt: now}
	}
}

// StationNotifier delivers to the 站内信 inbox via notify-service's internal
// alert endpoint (the same one Alertmanager webhooks to).
type StationNotifier struct {
	baseURL string
	token   string
	client  *http.Client
	now     func() time.Time
}

func NewStationNotifier() *StationNotifier {
	return &StationNotifier{
		baseURL: strings.TrimRight(config.Cfg.Notification.Alert.StationBaseURL, "/"),
		token:   config.Cfg.Notification.Alert.StationToken,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}
}

func (n *StationNotifier) NotifyContext(ctx context.Context, _ *model.MonitorAlertRule, event *model.MonitorAlertEvent) AlertNotification {
	result := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: n.now().UTC()}
	if event == nil || n.baseURL == "" || n.token == "" {
		return result
	}
	payload := stationWebhookPayload{
		Status: event.Status,
		Alerts: []stationWebhookAlert{{
			Status: event.Status,
			Labels: map[string]string{
				"alertname": event.RuleName,
				"severity":  event.Severity,
				"metric":    event.Metric,
			},
			Annotations: map[string]string{
				"summary":     event.Message,
				"description": event.Message,
			},
			StartsAt:    event.CreatedAt.UTC().Format(time.RFC3339),
			Fingerprint: stationFingerprint(event),
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/api/v1/notify/internal/alerts", bytes.NewReader(body))
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", n.token)
	resp, err := n.client.Do(req)
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		result.Status = AlertNotifyFailed
		result.Error = fmt.Sprintf("notify returned HTTP %d", resp.StatusCode)
		return result
	}
	result.Status = AlertNotifySent
	return result
}

func stationFingerprint(event *model.MonitorAlertEvent) string {
	ruleID := uint64(0)
	if event.RuleID != nil {
		ruleID = uint64(*event.RuleID)
	}
	return fmt.Sprintf("monitor-%d-%d", ruleID, event.ID)
}

type stationWebhookPayload struct {
	Status string                 `json:"status"`
	Alerts []stationWebhookAlert  `json:"alerts"`
}

type stationWebhookAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// WeComNotifier delivers to a 企业微信 robot webhook.
type WeComNotifier struct {
	webhook string
	client  *http.Client
	now     func() time.Time
}

func NewWeComNotifier() *WeComNotifier {
	return &WeComNotifier{
		webhook: config.Cfg.Notification.Alert.WeComWebhook,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}
}

func (n *WeComNotifier) NotifyContext(ctx context.Context, _ *model.MonitorAlertRule, event *model.MonitorAlertEvent) AlertNotification {
	result := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: n.now().UTC()}
	if event == nil || n.webhook == "" {
		return result
	}
	content := fmt.Sprintf(
		"**GopherForge 告警**\n> 规则：%s\n> 状态：%s\n> 级别：%s\n> 指标：%s\n> 值：%.4f / 阈值：%.4f\n> 详情：%s\n> 时间：%s",
		event.RuleName, event.Status, event.Severity, event.Metric,
		event.Value, event.Threshold, event.Message,
		event.CreatedAt.UTC().Format(time.RFC3339),
	)
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{"content": content},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhook, bytes.NewReader(body))
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		result.Status = AlertNotifyFailed
		result.Error = fmt.Sprintf("wecom webhook returned HTTP %d", resp.StatusCode)
		return result
	}
	result.Status = AlertNotifySent
	return result
}
