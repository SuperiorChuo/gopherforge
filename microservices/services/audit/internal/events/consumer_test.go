package events

import (
	"testing"
	"time"
)

func TestBuildLoginInfoSuccess(t *testing.T) {
	payload := []byte(`{
		"user_id": 42,
		"username": "admin",
		"ip": "10.0.0.8",
		"user_agent": "Mozilla/5.0",
		"login_type": "account",
		"timestamp": "2026-07-14T10:30:00+08:00"
	}`)

	info, err := buildLoginInfo(SubjectLoginSuccess, payload)
	if err != nil {
		t.Fatalf("buildLoginInfo returned error: %v", err)
	}
	if info.UserID != 42 || info.Username != "admin" {
		t.Fatalf("unexpected identity: %+v", info)
	}
	if info.TenantID != 1 {
		t.Fatalf("expected default tenant_id=1 when event omits it, got %d", info.TenantID)
	}
	if info.Status != loginStatusSuccess {
		t.Fatalf("expected success status, got %d", info.Status)
	}
	if info.LoginType != LoginTypePassword {
		t.Fatalf("expected password login type, got %d", info.LoginType)
	}
	if info.Message != "" {
		t.Fatalf("success events must not set message, got %q", info.Message)
	}
	want := time.Date(2026, 7, 14, 10, 30, 0, 0, time.FixedZone("", 8*3600))
	if !info.OccurredAt.Equal(want) {
		t.Fatalf("expected occurred at %v, got %v", want, info.OccurredAt)
	}
}

func TestBuildLoginInfoCarriesTenantID(t *testing.T) {
	payload := []byte(`{
		"user_id": 7,
		"tenant_id": 3,
		"username": "bob",
		"login_type": "account",
		"timestamp": "2026-07-14T10:30:00Z"
	}`)
	info, err := buildLoginInfo(SubjectLoginSuccess, payload)
	if err != nil {
		t.Fatalf("buildLoginInfo returned error: %v", err)
	}
	if info.TenantID != 3 {
		t.Fatalf("expected tenant_id=3, got %d", info.TenantID)
	}
}

func TestBuildLoginInfoFailedSetsMessage(t *testing.T) {
	payload := []byte(`{
		"username": "admin",
		"ip": "10.0.0.8",
		"reason": "invalid_credentials",
		"timestamp": "2026-07-14T10:31:00Z"
	}`)

	info, err := buildLoginInfo(SubjectLoginFailed, payload)
	if err != nil {
		t.Fatalf("buildLoginInfo returned error: %v", err)
	}
	if info.Status != loginStatusFailed {
		t.Fatalf("expected failed status, got %d", info.Status)
	}
	if info.Message != "invalid_credentials" {
		t.Fatalf("expected reason as message, got %q", info.Message)
	}
	if info.UserID != 0 {
		t.Fatalf("failed logins carry no user id, got %d", info.UserID)
	}
}

func TestBuildLoginInfoRejectsMalformedPayload(t *testing.T) {
	if _, err := buildLoginInfo(SubjectLoginSuccess, []byte("{not json")); err == nil {
		t.Fatal("expected error for malformed payload")
	}
}

func TestBuildLoginInfoRejectsUnknownSubject(t *testing.T) {
	if _, err := buildLoginInfo("auth.logout", []byte(`{}`)); err == nil {
		t.Fatal("expected error for unhandled subject")
	}
}

func TestBuildLoginInfoTruncatesLongReason(t *testing.T) {
	long := make([]byte, messageMaxLen+50)
	for i := range long {
		long[i] = 'x'
	}
	payload := []byte(`{"reason": "` + string(long) + `"}`)

	info, err := buildLoginInfo(SubjectLoginFailed, payload)
	if err != nil {
		t.Fatalf("buildLoginInfo returned error: %v", err)
	}
	if len(info.Message) != messageMaxLen {
		t.Fatalf("expected message truncated to %d, got %d", messageMaxLen, len(info.Message))
	}
}

func TestLoginTypeCode(t *testing.T) {
	cases := []struct {
		in   string
		want int8
	}{
		{"account", LoginTypePassword},
		{"console", LoginTypePassword},
		{"totp", LoginTypeTOTP},
		{"oauth:github", LoginTypeGithub},
		{"oauth:wechat", LoginTypeWechat},
		{"", LoginTypePassword},
		{"something-new", LoginTypePassword},
	}
	for _, tc := range cases {
		if got := loginTypeCode(tc.in); got != tc.want {
			t.Errorf("loginTypeCode(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseEventTime(t *testing.T) {
	if got := parseEventTime(""); !got.IsZero() {
		t.Fatalf("empty timestamp must map to zero time, got %v", got)
	}
	if got := parseEventTime("not-a-time"); !got.IsZero() {
		t.Fatalf("invalid timestamp must map to zero time, got %v", got)
	}
	if got := parseEventTime("2026-07-14T09:00:00Z"); got.IsZero() {
		t.Fatal("valid timestamp must parse")
	}
}

func TestConsumerNilSafeClose(t *testing.T) {
	var c *Consumer
	c.Close() // must not panic
}
