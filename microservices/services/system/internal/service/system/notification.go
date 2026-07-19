package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/system/internal/model"
	redisstore "github.com/go-admin-kit/services/system/internal/pkg/redis"
	"github.com/google/uuid"
)

const NotificationRedisChannel = "go_admin_kit:notifications"

// NotificationMessage is the realtime payload sent to web console clients.
type NotificationMessage struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	UserID    uint      `json:"user_id,omitempty"`
	Link      string    `json:"link,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	SourceID  string    `json:"source_id,omitempty"`
}

type NotificationBroadcaster struct {
	mu       sync.RWMutex
	clients  map[uint]map[chan NotificationMessage]struct{}
	sourceID string
}

var defaultNotificationBroadcaster = NewNotificationBroadcaster()

func DefaultNotificationBroadcaster() *NotificationBroadcaster {
	return defaultNotificationBroadcaster
}

func NewNotificationBroadcaster() *NotificationBroadcaster {
	return &NotificationBroadcaster{
		clients:  make(map[uint]map[chan NotificationMessage]struct{}),
		sourceID: uuid.NewString(),
	}
}

func NotificationMessageFromNotice(notice *model.Notice) NotificationMessage {
	if notice == nil {
		return NotificationMessage{}
	}
	messageType := "notice"
	if notice.Type == 2 {
		messageType = "announcement"
	}
	createdAt := notice.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	return NotificationMessage{
		ID:        fmt.Sprintf("notice:%d", notice.ID),
		Type:      messageType,
		Title:     notice.Title,
		Content:   notice.Content,
		UserID:    0,
		Link:      "/system/notice",
		CreatedAt: createdAt,
	}
}

func (b *NotificationBroadcaster) Subscribe(userID uint) (<-chan NotificationMessage, func()) {
	if b == nil {
		b = DefaultNotificationBroadcaster()
	}
	ch := make(chan NotificationMessage, 16)
	b.mu.Lock()
	if b.clients[userID] == nil {
		b.clients[userID] = make(map[chan NotificationMessage]struct{})
	}
	b.clients[userID][ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if clients := b.clients[userID]; clients != nil {
			delete(clients, ch)
			if len(clients) == 0 {
				delete(b.clients, userID)
			}
		}
		b.mu.Unlock()
	}
	return ch, unsubscribe
}

func (b *NotificationBroadcaster) PublishLocal(message NotificationMessage) {
	if b == nil {
		b = DefaultNotificationBroadcaster()
	}
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	b.mu.RLock()
	targets := make([]chan NotificationMessage, 0)
	if message.UserID == 0 {
		for _, clients := range b.clients {
			for ch := range clients {
				targets = append(targets, ch)
			}
		}
	} else if clients := b.clients[message.UserID]; clients != nil {
		for ch := range clients {
			targets = append(targets, ch)
		}
	}
	b.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- message:
		default:
			if logger.Logger != nil {
				logger.Warn("notification client channel is full", logger.String("message_id", message.ID))
			}
		}
	}
}

func (b *NotificationBroadcaster) PublishContext(ctx context.Context, message NotificationMessage) error {
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	b.PublishLocal(message)

	message.SourceID = b.sourceID
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if err := redisstore.PublishString(ctx, NotificationRedisChannel, string(payload)); isRedisUnavailable(err) {
		return nil
	} else {
		return err
	}
}

func (b *NotificationBroadcaster) StartRedisBridge(ctx context.Context) (*redisstore.StringSubscriber, error) {
	subscriber, err := redisstore.StartSubscriber(ctx, NotificationRedisChannel, func(_ context.Context, payload string) {
		var message NotificationMessage
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			if logger.Logger != nil {
				logger.Warn("invalid notification pubsub payload", logger.Err(err))
			}
			return
		}
		if message.SourceID == b.sourceID {
			return
		}
		b.PublishLocal(message)
	})
	if isRedisUnavailable(err) {
		return nil, nil
	}
	return subscriber, err
}

func isRedisUnavailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "redis client is nil")
}
