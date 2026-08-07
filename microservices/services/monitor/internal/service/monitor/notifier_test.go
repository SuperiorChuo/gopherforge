package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/model"
)

type stubNotifier struct {
	status string
	err    string
	called int
}

func (s *stubNotifier) NotifyContext(_ context.Context, _ *model.MonitorAlertRule, _ *model.MonitorAlertEvent) AlertNotification {
	s.called++
	return AlertNotification{Status: s.status, Error: s.err, NotifiedAt: time.Now().UTC()}
}

func alertEventFixture() *model.MonitorAlertEvent {
	now := time.Now().UTC()
	ruleID := uint(7)
	return &model.MonitorAlertEvent{
		ID:       11,
		RuleID:   &ruleID,
		RuleName: "High CPU",
		Metric:   "system.cpu.used_percent",
		Severity: "critical",
		Status:   AlertEventFiring,
		Value:    95,
		Threshold: 90,
		Message:  "cpu above threshold",
		CreatedAt: now,
	}
}

func TestMultiChannelNotifierFansOutToRuleChannels(t *testing.T) {
	email := &stubNotifier{status: AlertNotifySkipped}
	wecom := &stubNotifier{status: AlertNotifySent}
	notifier := &MultiChannelNotifier{channels: map[string]AlertNotifier{"email": email, "wecom": wecom}}

	rule := &model.MonitorAlertRule{NotifyChannels: model.NotifyChannelList{"wecom"}}
	result := notifier.NotifyContext(context.Background(), rule, alertEventFixture())

	if email.called != 0 || wecom.called != 1 {
		t.Fatalf("email called %d times, wecom %d; want 0 and 1", email.called, wecom.called)
	}
	if result.Status != AlertNotifySent {
		t.Fatalf("result = %#v, want sent", result)
	}
}

func TestMultiChannelNotifierEmptyChannelsUsesAll(t *testing.T) {
	email := &stubNotifier{status: AlertNotifySkipped}
	wecom := &stubNotifier{status: AlertNotifySent}
	notifier := &MultiChannelNotifier{channels: map[string]AlertNotifier{"email": email, "wecom": wecom}}

	result := notifier.NotifyContext(context.Background(), &model.MonitorAlertRule{}, alertEventFixture())

	if email.called != 1 || wecom.called != 1 {
		t.Fatalf("email called %d, wecom %d; want both once", email.called, wecom.called)
	}
	if result.Status != AlertNotifySent {
		t.Fatalf("result = %#v, want sent when any channel delivers", result)
	}
}

func TestMultiChannelNotifierAggregates(t *testing.T) {
	t.Run("all skipped -> skipped", func(t *testing.T) {
		notifier := &MultiChannelNotifier{channels: map[string]AlertNotifier{
			"email": &stubNotifier{status: AlertNotifySkipped},
			"wecom": &stubNotifier{status: AlertNotifySkipped},
		}}
		result := notifier.NotifyContext(context.Background(), &model.MonitorAlertRule{}, alertEventFixture())
		if result.Status != AlertNotifySkipped {
			t.Fatalf("result = %#v, want skipped", result)
		}
	})

	t.Run("failed wins over skipped", func(t *testing.T) {
		notifier := &MultiChannelNotifier{channels: map[string]AlertNotifier{
			"email": &stubNotifier{status: AlertNotifySkipped},
			"wecom": &stubNotifier{status: AlertNotifyFailed, err: "webhook 500"},
		}}
		result := notifier.NotifyContext(context.Background(), &model.MonitorAlertRule{}, alertEventFixture())
		if result.Status != AlertNotifyFailed || result.Error == "" {
			t.Fatalf("result = %#v, want failed with error", result)
		}
	})

	t.Run("sent beats failed", func(t *testing.T) {
		notifier := &MultiChannelNotifier{channels: map[string]AlertNotifier{
			"email": &stubNotifier{status: AlertNotifyFailed, err: "smtp down"},
			"wecom": &stubNotifier{status: AlertNotifySent},
		}}
		result := notifier.NotifyContext(context.Background(), &model.MonitorAlertRule{}, alertEventFixture())
		if result.Status != AlertNotifySent {
			t.Fatalf("result = %#v, want sent", result)
		}
	})
}

func TestStationNotifier(t *testing.T) {
	var gotPath, gotToken string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Internal-Token")
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &StationNotifier{baseURL: server.URL, token: "secret-token", client: server.Client(), now: time.Now}
	result := notifier.NotifyContext(context.Background(), nil, alertEventFixture())

	if result.Status != AlertNotifySent {
		t.Fatalf("result = %#v, want sent", result)
	}
	if gotPath != "/api/v1/notify/internal/alerts" {
		t.Errorf("path = %q, want notify internal alerts", gotPath)
	}
	if gotToken != "secret-token" {
		t.Errorf("token = %q, want secret-token", gotToken)
	}
	alerts, ok := gotPayload["alerts"].([]any)
	if !ok || len(alerts) != 1 {
		t.Fatalf("payload alerts = %#v, want one alert", gotPayload["alerts"])
	}
}

func TestStationNotifierSkippedWhenUnconfigured(t *testing.T) {
	notifier := &StationNotifier{baseURL: "", token: "", client: http.DefaultClient, now: time.Now}
	result := notifier.NotifyContext(context.Background(), nil, alertEventFixture())
	if result.Status != AlertNotifySkipped {
		t.Fatalf("result = %#v, want skipped when unconfigured", result)
	}
}

func TestStationNotifierFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	notifier := &StationNotifier{baseURL: server.URL, token: "t", client: server.Client(), now: time.Now}
	result := notifier.NotifyContext(context.Background(), nil, alertEventFixture())
	if result.Status != AlertNotifyFailed {
		t.Fatalf("result = %#v, want failed on HTTP 502", result)
	}
}

func TestWeComNotifier(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &WeComNotifier{webhook: server.URL, client: server.Client(), now: time.Now}
	result := notifier.NotifyContext(context.Background(), nil, alertEventFixture())

	if result.Status != AlertNotifySent {
		t.Fatalf("result = %#v, want sent", result)
	}
	if gotPayload["msgtype"] != "markdown" {
		t.Fatalf("payload msgtype = %v, want markdown", gotPayload["msgtype"])
	}
}

func TestWeComNotifierSkippedWhenUnconfigured(t *testing.T) {
	notifier := &WeComNotifier{webhook: "", client: http.DefaultClient, now: time.Now}
	result := notifier.NotifyContext(context.Background(), nil, alertEventFixture())
	if result.Status != AlertNotifySkipped {
		t.Fatalf("result = %#v, want skipped", result)
	}
}

func TestRateSourceReportsPerSecondDelta(t *testing.T) {
	current := 1000.0
	raw := func(context.Context) (float64, error) { return current, nil }
	rs := &rateSource{raw: raw, prev: 1000, prevAt: time.Now().Add(-5 * time.Second), init: true}

	current = 1500 // 5s 内 +500 → ~100/s
	got, err := rs.value(context.Background())
	if err != nil {
		t.Fatalf("value error = %v", err)
	}
	if got < 90 || got > 110 {
		t.Fatalf("rate = %v, want ~100 (500 over 5s)", got)
	}

	// 计数回退（重启/重置）时记 0，不产生负速率
	current = 200
	got, err = rs.value(context.Background())
	if err != nil || got < 0 {
		t.Fatalf("after counter reset rate = %v err %v, want >= 0", got, err)
	}
}
