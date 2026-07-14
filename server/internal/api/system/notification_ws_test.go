package system

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	systemsvc "github.com/go-admin-kit/server/internal/service/system"
	"github.com/gorilla/websocket"
)

func TestNotificationTicketFromRequestUsesTicketQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/ws/notifications?ticket=query-ticket", nil)
	c.Request.Header.Set("Authorization", "Bearer bearer-token")

	if got := notificationTicketFromRequest(c); got != "query-ticket" {
		t.Fatalf("ticket = %q, want query-ticket", got)
	}
}

func TestNotificationTicketFromRequestIgnoresBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/ws/notifications", nil)
	c.Request.Header.Set("Authorization", "Bearer bearer-token")

	if got := notificationTicketFromRequest(c); got != "" {
		t.Fatalf("ticket = %q, want empty", got)
	}
}

func TestNotificationOriginAllowedRequiresSameOriginOrConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sameOrigin := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/v1/ws/notifications", nil)
	sameOrigin.Header.Set("Origin", "http://api.example.com")
	if !notificationOriginAllowed(sameOrigin) {
		t.Fatal("same-origin websocket request should be allowed")
	}

	crossOrigin := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/v1/ws/notifications", nil)
	crossOrigin.Header.Set("Origin", "https://evil.example")
	if notificationOriginAllowed(crossOrigin) {
		t.Fatal("unexpected cross-origin websocket request should be rejected")
	}
}

func TestStartNotificationRedisBridgeStartsOnlyOnce(t *testing.T) {
	var starts int32
	restoreNotificationRedisBridgeStart(t, func(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error) {
		atomic.AddInt32(&starts, 1)
		return &fakeNotificationRedisBridgeSubscriber{closed: make(chan struct{})}, nil
	})

	if err := StartNotificationRedisBridge(context.Background(), systemsvc.NewNotificationBroadcaster()); err != nil {
		t.Fatalf("StartNotificationRedisBridge() error = %v", err)
	}
	if err := StartNotificationRedisBridge(context.Background(), systemsvc.NewNotificationBroadcaster()); err != nil {
		t.Fatalf("second StartNotificationRedisBridge() error = %v", err)
	}

	if got := atomic.LoadInt32(&starts); got != 1 {
		t.Fatalf("bridge starts = %d, want 1", got)
	}
}

func TestStopNotificationRedisBridgeClosesSubscriber(t *testing.T) {
	subscriber := &fakeNotificationRedisBridgeSubscriber{closed: make(chan struct{})}
	restoreNotificationRedisBridgeStart(t, func(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error) {
		return subscriber, nil
	})

	if err := StartNotificationRedisBridge(context.Background(), systemsvc.NewNotificationBroadcaster()); err != nil {
		t.Fatalf("StartNotificationRedisBridge() error = %v", err)
	}
	if err := StopNotificationRedisBridge(); err != nil {
		t.Fatalf("StopNotificationRedisBridge() error = %v", err)
	}

	select {
	case <-subscriber.closed:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not closed")
	}
	if got := atomic.LoadInt32(&subscriber.closes); got != 1 {
		t.Fatalf("subscriber closes = %d, want 1", got)
	}
}

func TestStartNotificationRedisBridgeContextCancelClosesSubscriber(t *testing.T) {
	subscriber := &fakeNotificationRedisBridgeSubscriber{closed: make(chan struct{})}
	restoreNotificationRedisBridgeStart(t, func(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error) {
		return subscriber, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := StartNotificationRedisBridge(ctx, systemsvc.NewNotificationBroadcaster()); err != nil {
		t.Fatalf("StartNotificationRedisBridge() error = %v", err)
	}
	cancel()

	select {
	case <-subscriber.closed:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not closed after context cancellation")
	}
}

func TestNotificationWebSocketConsumesTicketSendsActiveNoticesAndPublishesTargetedMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setNotificationWebSocketTestJWTConfig(t)
	restoreStore := jwt.SetTokenBlacklistStore(newNotificationWebSocketTicketStore())
	t.Cleanup(restoreStore)
	var bridgeStarts int32
	restoreNotificationRedisBridgeStart(t, func(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error) {
		atomic.AddInt32(&bridgeStarts, 1)
		return &fakeNotificationRedisBridgeSubscriber{closed: make(chan struct{})}, nil
	})

	createdAt := time.Date(2026, 5, 23, 9, 30, 0, 0, time.UTC)
	db, mock := setupNoticeAPITestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "notices" WHERE status = 1 AND ((start_time IS NULL OR start_time <= NOW())) AND ((end_time IS NULL OR end_time >= NOW())) ORDER BY created_at DESC`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "status", "created_at", "updated_at"}).
			AddRow(uint(9), "Maintenance", "Maintenance window tonight", int8(2), int8(1), createdAt, createdAt))

	broadcaster := systemsvc.NewNotificationBroadcaster()
	api := &NotificationAPI{
		noticeService: systemsvc.NewNoticeServiceWithDB(db),
		broadcaster:   broadcaster,
	}
	router := gin.New()
	router.GET("/api/v1/ws/notifications", api.Connect)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	ticket, err := jwt.GenerateWebSocketTicket(42, "alice", time.Minute)
	if err != nil {
		t.Fatalf("GenerateWebSocketTicket() error = %v", err)
	}

	conn := dialNotificationWebSocket(t, server.URL, ticket)
	defer conn.Close()

	activeNotice := readNotificationWebSocketMessage(t, conn)
	if activeNotice.ID != "notice:9" {
		t.Fatalf("active notice id = %q, want notice:9", activeNotice.ID)
	}
	if activeNotice.Type != "announcement" {
		t.Fatalf("active notice type = %q, want announcement", activeNotice.Type)
	}
	if activeNotice.Title != "Maintenance" {
		t.Fatalf("active notice title = %q, want Maintenance", activeNotice.Title)
	}

	_, _, err = websocket.DefaultDialer.Dial(notificationWebSocketURL(t, server.URL, ticket), nil)
	if err == nil {
		t.Fatal("second websocket dial with the same ticket succeeded, want failure")
	}
	if !errors.Is(err, websocket.ErrBadHandshake) {
		t.Fatalf("second websocket dial error = %v, want bad handshake", err)
	}

	wantPublished := systemsvc.NotificationMessage{
		ID:      "targeted-message",
		Type:    "notification",
		Title:   "Build finished",
		Content: "Your export is ready",
		UserID:  42,
	}
	if err := broadcaster.PublishContext(context.Background(), wantPublished); err != nil {
		t.Fatalf("PublishContext() error = %v", err)
	}

	published := readNotificationWebSocketMessage(t, conn)
	if published.ID != wantPublished.ID {
		t.Fatalf("published id = %q, want %q", published.ID, wantPublished.ID)
	}
	if published.UserID != 42 {
		t.Fatalf("published user_id = %d, want 42", published.UserID)
	}
	if published.Content != wantPublished.Content {
		t.Fatalf("published content = %q, want %q", published.Content, wantPublished.Content)
	}
	if got := atomic.LoadInt32(&bridgeStarts); got != 0 {
		t.Fatalf("Connect started notification redis bridge %d times, want 0", got)
	}
}

func dialNotificationWebSocket(t *testing.T, serverURL, ticket string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(notificationWebSocketURL(t, serverURL, ticket), nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	return conn
}

func notificationWebSocketURL(t *testing.T, serverURL, ticket string) string {
	t.Helper()

	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	u.Scheme = "ws"
	u.Path = "/api/v1/ws/notifications"
	q := u.Query()
	q.Set("ticket", ticket)
	u.RawQuery = q.Encode()
	return u.String()
}

func readNotificationWebSocketMessage(t *testing.T, conn *websocket.Conn) systemsvc.NotificationMessage {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var message systemsvc.NotificationMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return message
}

func setNotificationWebSocketTestJWTConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.Cfg.JWT
	config.Cfg.JWT = config.JWTConfig{
		Secret:               "notification-websocket-test-secret",
		AccessTokenExpire:    3600,
		RefreshTokenExpire:   7200,
		RefreshTokenRotation: true,
		Issuer:               "notification-websocket-test",
	}
	t.Cleanup(func() {
		config.Cfg.JWT = oldConfig
	})
}

type fakeNotificationRedisBridgeSubscriber struct {
	closed chan struct{}
	once   sync.Once
	closes int32
}

func (s *fakeNotificationRedisBridgeSubscriber) Close() error {
	s.once.Do(func() {
		atomic.AddInt32(&s.closes, 1)
		close(s.closed)
	})
	return nil
}

func restoreNotificationRedisBridgeStart(t *testing.T, start notificationRedisBridgeStartFunc) {
	t.Helper()

	_ = StopNotificationRedisBridge()
	notificationBridgeMu.Lock()
	oldStart := notificationBridgeStart
	notificationBridgeStart = start
	notificationBridgeMu.Unlock()
	t.Cleanup(func() {
		_ = StopNotificationRedisBridge()
		notificationBridgeMu.Lock()
		notificationBridgeStart = oldStart
		notificationBridgeMu.Unlock()
	})
}

type notificationWebSocketTicketStore struct {
	mu       sync.Mutex
	consumed map[string]time.Duration
}

func newNotificationWebSocketTicketStore() *notificationWebSocketTicketStore {
	return &notificationWebSocketTicketStore{
		consumed: make(map[string]time.Duration),
	}
}

func (s *notificationWebSocketTicketStore) SetTokenID(_ context.Context, tokenID string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumed[tokenID] = ttl
	return nil
}

func (s *notificationWebSocketTicketStore) HasTokenID(_ context.Context, tokenID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.consumed[tokenID]
	return ok, nil
}

func (s *notificationWebSocketTicketStore) ConsumeTokenID(_ context.Context, tokenID string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.consumed[tokenID]; ok {
		return false, nil
	}
	s.consumed[tokenID] = ttl
	return true, nil
}
