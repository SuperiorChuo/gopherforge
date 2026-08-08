package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/redis/go-redis/v9"
)

// Stream/group layout must match the auth publisher (auth:events, fields
// subject+payload). Consumer group gives at-least-once delivery: events
// published while audit is down (rolling update, crash) stay pending in the
// stream and are re-read on startup — pub/sub silently dropped them.
const (
	streamKey     = "auth:events"
	consumerGroup = "audit-loginlog"
	consumerName  = "audit"
	readBlock     = 5 * time.Second
	readCount     = 100
)

// StartRedisLoginConsumer consumes auth login events from the auth:events
// Redis Stream via a consumer group and persists them as login_logs.
// A nil client disables consumption (returns nil, nil).
func StartRedisLoginConsumer(ctx context.Context, client redis.UniversalClient, recorder *systemsvc.LoginLogService) (*RedisConsumer, error) {
	if client == nil || recorder == nil {
		return nil, nil
	}
	// Idempotent group creation; BUSYGROUP means it already exists.
	if err := client.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "$").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("create consumer group: %w", err)
	}
	c := &RedisConsumer{client: client, recorder: recorder}
	go c.run(ctx)
	return c, nil
}

type RedisConsumer struct {
	client   redis.UniversalClient
	recorder *systemsvc.LoginLogService
}

// Close is kept for lifecycle symmetry; the run loop exits with its context.
func (c *RedisConsumer) Close() {}

func (c *RedisConsumer) run(ctx context.Context) {
	// First drain this consumer's pending entries (delivered but unacked
	// before a previous crash/restart), then switch to new messages.
	cursor := "0"
	for {
		if ctx.Err() != nil {
			return
		}
		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{streamKey, cursor},
			Count:    readCount,
			Block:    readBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				if ctx.Err() != nil {
					return
				}
				cursor = ">" // no pending backlog left; wait for new entries
				continue
			}
			warn("redis stream read failed", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		drained := true
		for _, stream := range streams {
			if len(stream.Messages) > 0 {
				drained = false
			}
			for _, msg := range stream.Messages {
				c.handle(ctx, msg)
			}
		}
		// XREADGROUP with an explicit ID returns pending entries; an empty
		// batch means the backlog is drained — switch to ">" for new ones.
		if cursor != ">" && drained {
			cursor = ">"
		}
	}
}

// handle processes one stream entry and acks it. Malformed or non-login
// entries are acked and skipped (poison messages must not loop forever);
// persistence failures leave the entry pending for redelivery on restart.
func (c *RedisConsumer) handle(ctx context.Context, msg redis.XMessage) {
	subject, _ := msg.Values["subject"].(string)
	payload, _ := msg.Values["payload"].(string)

	if !strings.HasPrefix(subject, "auth.login.") {
		c.ack(ctx, msg.ID) // e.g. auth.logout: no login-log to write
		return
	}
	info, err := buildLoginInfo(subject, []byte(payload))
	if err != nil {
		warn("malformed redis login event", err)
		c.ack(ctx, msg.ID)
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, recordTimeout)
	defer cancel()
	if err := c.recorder.RecordContext(writeCtx, info); err != nil {
		warn("login log write failed", err)
		return // leave pending; redelivered after restart
	}
	c.ack(ctx, msg.ID)
}

func (c *RedisConsumer) ack(ctx context.Context, id string) {
	if err := c.client.XAck(ctx, streamKey, consumerGroup, id).Err(); err != nil {
		warn("stream ack failed", err)
	}
}

// buildLoginInfo maps a raw login event to a LoginInfo. subject is the auth
// publisher's per-event subject (auth.login.success / auth.login.failed).
// Unhandled subjects return an error.
func buildLoginInfo(channel string, payload []byte) (*systemsvc.LoginInfo, error) {
	var event loginEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
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
		return nil, fmt.Errorf("unhandled channel %q", channel)
	}
	return info, nil
}
