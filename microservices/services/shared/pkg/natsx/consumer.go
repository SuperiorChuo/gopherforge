// Package natsx 封装 NATS 订阅/发布与消费者生命周期。

package natsx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	DefaultMaxDeliver = 5
	DefaultAckWait    = 30 * time.Second
	DLQSubjectPrefix  = "dlq."
)

type ConsumerConfig struct {
	StreamName    string
	Subject       string
	ConsumerName  string
	Durable       bool
	MaxDeliver    int
	AckWait       time.Duration
	DLQ           bool
	DLQSubject    string
	BatchSize     int
	ProcessFunc   func(ctx context.Context, data []byte, meta *MsgMeta) error
}

type MsgMeta struct {
	Subject    string
	Sequence   uint64
	Delivered  int
	Timestamp  time.Time
}

func StartConsumer(ctx context.Context, natsURL string, cfg ConsumerConfig) (func(), error) {
	if cfg.MaxDeliver <= 0 {
		cfg.MaxDeliver = DefaultMaxDeliver
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = DefaultAckWait
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	if cfg.DLQ && cfg.DLQSubject == "" {
		cfg.DLQSubject = DLQSubjectPrefix + cfg.Subject
	}
	nc, err := nats.Connect(natsURL,
		nats.Name(cfg.ConsumerName),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("natsx: connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("natsx: jetstream: %w", err)
	}
	if err := ensureStream(ctx, js, cfg); err != nil {
		nc.Close()
		return nil, err
	}
	if cfg.DLQ {
		if err := ensureDLQStream(ctx, js, cfg); err != nil {
			nc.Close()
			return nil, err
		}
	}
	consCfg := &nats.ConsumerConfig{
		Durable:       cfg.ConsumerName,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       cfg.AckWait,
		MaxDeliver:    cfg.MaxDeliver,
		MaxAckPending: cfg.BatchSize * 10,
	}
	if cfg.DLQ {
		consCfg.DeliverSubject = ""
		consCfg.DeliverPolicy = nats.DeliverAllPolicy
	}
	_, err = js.AddConsumer(cfg.StreamName, consCfg)
	if err != nil {
		var apiErr *nats.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode == nats.JSErrCodeConsumerNameExists {
			err = nil
		}
	}
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("natsx: add consumer %s: %w", cfg.ConsumerName, err)
	}
	sub, err := js.PullSubscribe("", cfg.ConsumerName, nats.BindStream(cfg.StreamName))
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("natsx: pull subscribe: %w", err)
	}
	logger.Info("natsx consumer started",
		zap.String("stream", cfg.StreamName),
		zap.String("consumer", cfg.ConsumerName),
		zap.Int("max_deliver", cfg.MaxDeliver),
		zap.Bool("dlq", cfg.DLQ),
	)
	go runConsumerLoop(ctx, sub, cfg, js)
	return func() {
		_ = sub.Unsubscribe()
		nc.Close()
	}, nil
}

func runConsumerLoop(ctx context.Context, sub *nats.Subscription, cfg ConsumerConfig, js nats.JetStreamContext) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msgs, err := sub.Fetch(cfg.BatchSize, nats.MaxWait(2*time.Second))
		if err != nil {
			if err != nats.ErrTimeout {
				logger.Warn("natsx: fetch error", logger.Err(err))
			}
			continue
		}
		for _, msg := range msgs {
			processMsg(ctx, msg, cfg, js)
		}
	}
}

func processMsg(ctx context.Context, msg *nats.Msg, cfg ConsumerConfig, js nats.JetStreamContext) {
	meta, err := msg.Metadata()
	if err != nil {
		logger.Warn("natsx: metadata error", logger.Err(err))
		_ = msg.Term()
		return
	}
	m := &MsgMeta{
		Subject:   msg.Subject,
		Sequence:  meta.Sequence.Stream,
		Delivered: int(meta.NumDelivered),
		Timestamp: meta.Timestamp,
	}
	processCtx, cancel := context.WithTimeout(ctx, cfg.AckWait-2*time.Second)
	defer cancel()
	err = cfg.ProcessFunc(processCtx, msg.Data, m)
	if err == nil {
		if ackErr := msg.Ack(); ackErr != nil {
			logger.Warn("natsx: ack failed", logger.Err(ackErr))
		}
		return
	}
	if meta.NumDelivered > uint64(cfg.MaxDeliver) {
		logger.Error("natsx: max deliver exceeded, sending to DLQ",
			zap.String("subject", msg.Subject),
			zap.Uint64("sequence", meta.Sequence.Stream),
			zap.Uint64("delivered", meta.NumDelivered),
			logger.Err(err),
		)
		if cfg.DLQ {
			forwardToDLQ(js, msg, cfg)
		}
		_ = msg.Term()
		return
	}
	logger.Warn("natsx: processing failed, will retry",
		zap.String("subject", msg.Subject),
		zap.Uint64("delivered", meta.NumDelivered),
		zap.Int("max_deliver", cfg.MaxDeliver),
		logger.Err(err),
	)
	_ = msg.Nak()
}

func forwardToDLQ(js nats.JetStreamContext, msg *nats.Msg, cfg ConsumerConfig) {
	_, err := js.Publish(cfg.DLQSubject, msg.Data)
	if err != nil {
		logger.Error("natsx: DLQ forward failed",
			zap.String("dlq_subject", cfg.DLQSubject),
			logger.Err(err),
		)
	}
}

func ensureStream(ctx context.Context, js nats.JetStreamContext, cfg ConsumerConfig) error {
	_, err := js.StreamInfo(cfg.StreamName)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      cfg.StreamName,
		Subjects:  []string{cfg.Subject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("natsx: create stream %s: %w", cfg.StreamName, err)
	}
	return nil
}

func ensureDLQStream(ctx context.Context, js nats.JetStreamContext, cfg ConsumerConfig) error {
	dlqStream := cfg.StreamName + "_dlq"
	_, err := js.StreamInfo(dlqStream)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      dlqStream,
		Subjects:  []string{DLQSubjectPrefix + ">"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    30 * 24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("natsx: create DLQ stream %s: %w", dlqStream, err)
	}
	return nil
}
