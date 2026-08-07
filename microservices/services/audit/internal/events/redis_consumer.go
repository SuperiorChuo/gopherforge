package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/redis/go-redis/v9"
)

const redisLoginEventsPattern = "auth.login.*"

// StartRedisLoginConsumer subscribes to auth login events via Redis pub/sub
// and persists them as login_logs. A nil client disables consumption (returns nil, nil).
// Uses PSubscribe for pattern-based subscription matching the auth publisher's
// per-event channels (auth.login.success, auth.login.failed).
func StartRedisLoginConsumer(ctx context.Context, client redis.UniversalClient, recorder *systemsvc.LoginLogService) (*RedisConsumer, error) {
	if client == nil || recorder == nil {
		return nil, nil
	}
	pubsub := client.PSubscribe(ctx, redisLoginEventsPattern)
	c := &RedisConsumer{pubsub: pubsub, recorder: recorder}
	go c.run(ctx)
	return c, nil
}

type RedisConsumer struct {
	pubsub   *redis.PubSub
	recorder *systemsvc.LoginLogService
}

func (c *RedisConsumer) Close() {
	if c != nil && c.pubsub != nil {
		c.pubsub.Close()
	}
}

func (c *RedisConsumer) run(ctx context.Context) {
	ch := c.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			c.handle(ctx, msg.Channel, msg.Payload)
		}
	}
}

func (c *RedisConsumer) handle(ctx context.Context, channel, payload string) {
	var event loginEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		warn("malformed redis login event", fmt.Errorf("unmarshal: %w", err))
		return
	}
	tenantID := event.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	info := &systemsvc.LoginInfo{
		UserID:     event.UserID,
		TenantID:   tenantID,
		Username:   event.Username,
		LoginType:  loginTypeCode(event.LoginType),
		IP:         event.IP,
		UserAgent:  event.UserAgent,
		DeviceID:   event.DeviceID,
		OccurredAt: parseEventTime(event.Timestamp),
	}
	switch {
	case strings.HasSuffix(channel, "success"):
		info.Status = loginStatusSuccess
	case strings.HasSuffix(channel, "failed"):
		info.Status = loginStatusFailed
		info.Message = truncate(event.Reason, messageMaxLen)
	default:
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, recordTimeout)
	defer cancel()
	if err := c.recorder.RecordContext(writeCtx, info); err != nil {
		warn("login log write failed", err)
	}
}
