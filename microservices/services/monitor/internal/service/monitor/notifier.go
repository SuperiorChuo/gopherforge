package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	notifyv1 "github.com/go-admin-kit/services/api/gen/notify/v1"
	"github.com/go-admin-kit/services/monitor/internal/config"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
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
		"webhook": NewWebhookNotifier(),
	}}
}

func (m *MultiChannelNotifier) NotifyContext(ctx context.Context, rule *localmodel.MonitorAlertRule, event *localmodel.MonitorAlertEvent) AlertNotification {
	if m == nil {
		return AlertNotification{Status: AlertNotifySkipped, NotifiedAt: time.Now().UTC()}
	}
	names := rule.NotifyChannels
	if len(names) == 0 {
		names = []string{"email", "station", "wecom", "webhook"}
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
// Phase 3 续：gRPC 优先（Consul 发现 notify-service），失败回退 HTTP。
type StationNotifier struct {
	baseURL  string
	token    string
	client   *http.Client
	resolver *grpcx.Resolver
	pool     *grpcx.ConnPool // Phase 4 收尾：gRPC 连接复用
	breaker  *resilience.Breaker
	now      func() time.Time
}

func NewStationNotifier() *StationNotifier {
	var resolver *grpcx.Resolver
	if r, err := grpcx.NewResolver(""); err == nil {
		resolver = r
	} else {
		log.Printf("monitor station: consul resolver 初始化失败（告警 gRPC 禁用，仅 HTTP）: %v", err)
	}
	return &StationNotifier{
		baseURL:  strings.TrimRight(config.Cfg.Notification.Alert.StationBaseURL, "/"),
		token:    config.Cfg.Notification.Alert.StationToken,
		client:   &http.Client{Timeout: 10 * time.Second},
		resolver: resolver,
		pool:     grpcx.NewConnPool(resolver, "notify-service"),
		breaker:  resilience.NewBreaker(5, 10*time.Second),
		now:      time.Now,
	}
}

func (n *StationNotifier) NotifyContext(ctx context.Context, _ *localmodel.MonitorAlertRule, event *localmodel.MonitorAlertEvent) AlertNotification {
	result := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: n.now().UTC()}
	if event == nil {
		return result
	}
	// 完全未配置（无 resolver 也无 HTTP base/token）→ 跳过，不算失败。
	if n.resolver == nil && (n.baseURL == "" || n.token == "") {
		return result
	}
	// Phase 3 续：gRPC 优先（Consul 发现 notify-service）；失败回退 HTTP。
	if n.resolver != nil {
		if err := n.notifyGRPC(ctx, event); err == nil {
			result.Status = AlertNotifySent
			return result
		}
	}
	if n.baseURL == "" || n.token == "" {
		// 已尝试 gRPC 但失败、HTTP 未配置——如实报失败，不伪装跳过。
		result.Status = AlertNotifyFailed
		result.Error = "station gRPC 失败且 HTTP 未配置（缺 baseURL/token）"
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

// notifyGRPC 经 Consul 发现 + gRPC 调 notify InternalAlert。
// 告警发送有服务端 ref 幂等（fingerprint-status-startsAt），重试安全。
func (n *StationNotifier) notifyGRPC(ctx context.Context, event *localmodel.MonitorAlertEvent) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return resilience.Do(ctx, resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: n.breaker,
	}, func(ctx context.Context) error {
		conn, err := n.pool.Get(ctx)
		if err != nil {
			return err
		}
		_, err = notifyv1.NewNotifyServiceClient(conn).InternalAlert(ctx, &notifyv1.InternalAlertRequest{
			Status: event.Status,
			Alerts: []*notifyv1.InternalAlertItem{{
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
		})
		if err == nil {
			return nil
		}
		if grpcx.Retryable(err) {
			n.pool.Invalidate()
			return err
		}
		if grpcx.IsTransient(err) {
			return fmt.Errorf("%w: %w", resilience.ErrTransient, err)
		}
		return fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
	})
}

func stationFingerprint(event *localmodel.MonitorAlertEvent) string {
	ruleID := uint64(0)
	if event.RuleID != nil {
		ruleID = uint64(*event.RuleID)
	}
	return fmt.Sprintf("monitor-%d-%d", ruleID, event.ID)
}

type stationWebhookPayload struct {
	Status string                `json:"status"`
	Alerts []stationWebhookAlert `json:"alerts"`
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

func (n *WeComNotifier) NotifyContext(ctx context.Context, _ *localmodel.MonitorAlertRule, event *localmodel.MonitorAlertEvent) AlertNotification {
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
		"msgtype":  "markdown",
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

// WebhookNotifier sends alerts to a user-supplied HTTP endpoint as JSON.
type WebhookNotifier struct {
	webhook string
	client  *http.Client
	now     func() time.Time
}

func NewWebhookNotifier() *WebhookNotifier {
	return &WebhookNotifier{
		webhook: config.Cfg.Notification.Alert.WebhookURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}
}

func (n *WebhookNotifier) NotifyContext(ctx context.Context, _ *localmodel.MonitorAlertRule, event *localmodel.MonitorAlertEvent) AlertNotification {
	result := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: n.now().UTC()}
	if event == nil || n.webhook == "" {
		return result
	}
	payload := map[string]any{
		"status": event.Status,
		"alerts": []map[string]any{{
			"status": event.Status,
			"labels": map[string]string{
				"alertname": event.RuleName,
				"severity":  event.Severity,
				"metric":    event.Metric,
			},
			"annotations": map[string]string{
				"summary":     event.Message,
				"description": fmt.Sprintf("%s: %s (%.4f / %.4f)", event.RuleName, event.Metric, event.Value, event.Threshold),
			},
			"startsAt":    event.CreatedAt.UTC().Format(time.RFC3339),
			"fingerprint": fmt.Sprintf("monitor-webhook-%d", event.ID),
		}},
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
		result.Error = fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)
		return result
	}
	result.Status = AlertNotifySent
	return result
}
