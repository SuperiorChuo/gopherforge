package system

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/system/internal/config"
	"github.com/go-admin-kit/services/system/internal/pkg/jwt"
	"github.com/go-admin-kit/services/system/internal/pkg/logger"
	"github.com/go-admin-kit/services/system/internal/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"github.com/gorilla/websocket"
)

type NotificationAPI struct {
	noticeService systemsvc.NoticeService
	broadcaster   *systemsvc.NotificationBroadcaster
}

var (
	notificationBridgeMu         sync.Mutex
	notificationBridgeCancel     context.CancelFunc
	notificationBridgeSubscriber notificationRedisBridgeSubscriber
	notificationBridgeRunning    bool
	notificationBridgeErr        error
	notificationBridgeStart      notificationRedisBridgeStartFunc = func(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error) {
		return broadcaster.StartRedisBridge(ctx)
	}
)

type notificationRedisBridgeSubscriber interface {
	Close() error
}

type notificationRedisBridgeStartFunc func(context.Context, *systemsvc.NotificationBroadcaster) (notificationRedisBridgeSubscriber, error)

func NewNotificationAPI() *NotificationAPI {
	return &NotificationAPI{
		noticeService: systemsvc.NoticeService{},
		broadcaster:   systemsvc.DefaultNotificationBroadcaster(),
	}
}

// NewNotificationAPIWithService creates a NotificationAPI instance from an
// injected notice service. The broadcaster keeps its default implementation.
func NewNotificationAPIWithService(noticeService systemsvc.NoticeService) *NotificationAPI {
	return &NotificationAPI{
		noticeService: noticeService,
		broadcaster:   systemsvc.DefaultNotificationBroadcaster(),
	}
}

func (a *NotificationAPI) CreateTicket(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}
	username, _ := c.Get("username")

	ticket, err := jwt.GenerateWebSocketTicket(userID.(uint), usernameString(username), time.Minute)
	if err != nil {
		internalServerError(c, "failed to create notification ticket", err)
		return
	}
	response.Success(c, gin.H{"ticket": ticket})
}

func (a *NotificationAPI) Connect(c *gin.Context) {
	if !notificationOriginAllowed(c.Request) {
		response.Unauthorized(c, "invalid notification origin")
		return
	}

	claims, err := parseNotificationClaims(c)
	if err != nil {
		response.Unauthorized(c, "invalid notification ticket")
		return
	}

	conn, err := notificationUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		if logger.Logger != nil {
			logger.Warn("failed to upgrade notification websocket", logger.Err(err))
		}
		return
	}
	defer conn.Close()

	stream, unsubscribe := a.broadcaster.Subscribe(claims.UserID)
	defer unsubscribe()

	a.sendActiveNoticeMessages(c.Request.Context(), conn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		case message := <-stream:
			if err := conn.WriteJSON(message); err != nil {
				return
			}
		}
	}
}

func (a *NotificationAPI) sendActiveNoticeMessages(ctx context.Context, conn *websocket.Conn) {
	notices, err := a.noticeService.GetActiveListContext(ctx, nil)
	if err != nil {
		if logger.Logger != nil {
			logger.Warn("failed to load active notices for notification websocket", logger.Err(err))
		}
		return
	}
	for _, notice := range notices {
		if err := conn.WriteJSON(systemsvc.NotificationMessageFromNotice(&notice)); err != nil {
			return
		}
	}
}

func parseNotificationClaims(c *gin.Context) (*jwt.Claims, error) {
	ticket := notificationTicketFromRequest(c)
	claims, err := jwt.ParseWebSocketTicket(ticket)
	if err != nil {
		return nil, err
	}
	if err := consumeNotificationTicketContext(c.Request.Context(), claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func notificationTicketFromRequest(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if ticket := strings.TrimSpace(c.Query("ticket")); ticket != "" {
		return ticket
	}
	return ""
}

func consumeNotificationTicketContext(ctx context.Context, claims *jwt.Claims) error {
	if claims == nil || claims.ExpiresAt == nil {
		return jwt.ErrInvalidToken
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return jwt.ErrExpiredToken
	}
	consumed, err := jwt.ConsumeTokenID(ctx, claims.ID, ttl)
	if err != nil {
		return err
	}
	if !consumed {
		return jwt.ErrRevokedToken
	}
	return nil
}

func usernameString(value any) string {
	if username, ok := value.(string); ok {
		return username
	}
	return ""
}

func StartNotificationRedisBridge(ctx context.Context, broadcaster *systemsvc.NotificationBroadcaster) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if broadcaster == nil {
		broadcaster = systemsvc.DefaultNotificationBroadcaster()
	}

	notificationBridgeMu.Lock()
	defer notificationBridgeMu.Unlock()

	if notificationBridgeRunning {
		return notificationBridgeErr
	}

	bridgeCtx, cancel := context.WithCancel(ctx)
	subscriber, err := notificationBridgeStart(bridgeCtx, broadcaster)
	if err != nil {
		cancel()
		notificationBridgeErr = err
		if logger.Logger != nil {
			logger.Warn("notification redis bridge disabled", logger.Err(err))
		}
		return err
	}

	notificationBridgeCancel = cancel
	notificationBridgeSubscriber = subscriber
	notificationBridgeRunning = true
	notificationBridgeErr = nil
	go func() {
		<-bridgeCtx.Done()
		_ = StopNotificationRedisBridge()
	}()
	return nil
}

func StopNotificationRedisBridge() error {
	notificationBridgeMu.Lock()
	cancel := notificationBridgeCancel
	subscriber := notificationBridgeSubscriber
	notificationBridgeCancel = nil
	notificationBridgeSubscriber = nil
	notificationBridgeRunning = false
	notificationBridgeErr = nil
	notificationBridgeMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if subscriber != nil {
		return subscriber.Close()
	}
	return nil
}

var notificationUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	CheckOrigin: func(r *http.Request) bool {
		return notificationOriginAllowed(r)
	},
}

func notificationOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}
	if strings.EqualFold(originURL.Host, r.Host) {
		return true
	}

	normalizedOrigin := originURL.Scheme + "://" + originURL.Host
	for _, allowed := range config.Cfg.CORS.AllowOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" || allowed == "*" {
			continue
		}
		if strings.EqualFold(allowed, origin) || strings.EqualFold(allowed, normalizedOrigin) {
			return true
		}
		allowedURL, err := url.Parse(allowed)
		if err == nil && strings.EqualFold(allowedURL.Scheme, originURL.Scheme) && strings.EqualFold(allowedURL.Host, originURL.Host) {
			return true
		}
	}
	return false
}
