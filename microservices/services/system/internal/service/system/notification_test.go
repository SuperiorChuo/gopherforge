package system

import (
	"testing"
	"time"

	"github.com/go-admin-kit/services/system/internal/model"
)

func TestNotificationMessageFromNoticeUsesAnnouncementPayload(t *testing.T) {
	createdAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	msg := NotificationMessageFromNotice(&model.Notice{
		ID:        42,
		Title:     "系统维护",
		Content:   "今晚 23:00 发布新版本",
		Type:      2,
		Status:    1,
		CreatedAt: createdAt,
	})

	if msg.ID != "notice:42" || msg.Type != "announcement" || msg.Title != "系统维护" || msg.Content != "今晚 23:00 发布新版本" {
		t.Fatalf("message = %#v, want notice announcement payload", msg)
	}
	if msg.UserID != 0 {
		t.Fatalf("message user id = %d, want broadcast target", msg.UserID)
	}
	if !msg.CreatedAt.Equal(createdAt) {
		t.Fatalf("message created_at = %s, want %s", msg.CreatedAt, createdAt)
	}
}

func TestNotificationBroadcasterDeliversBroadcastAndUserScopedMessages(t *testing.T) {
	broadcaster := NewNotificationBroadcaster()
	userSeven, unsubscribeSeven := broadcaster.Subscribe(7)
	defer unsubscribeSeven()
	userEight, unsubscribeEight := broadcaster.Subscribe(8)
	defer unsubscribeEight()

	broadcaster.PublishLocal(NotificationMessage{ID: "all", UserID: 0, Content: "广播"})
	expectNotification(t, userSeven, "all")
	expectNotification(t, userEight, "all")

	broadcaster.PublishLocal(NotificationMessage{ID: "only-seven", UserID: 7, Content: "个人提醒"})
	expectNotification(t, userSeven, "only-seven")
	expectNoNotification(t, userEight)
}

func expectNotification(t *testing.T, ch <-chan NotificationMessage, wantID string) {
	t.Helper()
	select {
	case got := <-ch:
		if got.ID != wantID {
			t.Fatalf("notification id = %q, want %q", got.ID, wantID)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for notification %q", wantID)
	}
}

func expectNoNotification(t *testing.T, ch <-chan NotificationMessage) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("unexpected notification: %#v", got)
	case <-time.After(30 * time.Millisecond):
	}
}
