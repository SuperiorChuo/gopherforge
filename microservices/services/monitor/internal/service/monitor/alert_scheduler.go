package monitor

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
)

const DefaultAlertEvaluationInterval = 30 * time.Second

type AlertEvaluator struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func StartAlertEvaluator(parent context.Context, service *AlertService, interval time.Duration) *AlertEvaluator {
	if parent == nil {
		parent = context.Background()
	}
	if interval <= 0 {
		interval = DefaultAlertEvaluationInterval
	}
	ctx, cancel := context.WithCancel(parent)
	evaluator := &AlertEvaluator{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(evaluator.done)
		evaluateAlertRules(ctx, service)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				evaluateAlertRules(ctx, service)
			}
		}
	}()
	return evaluator
}

func (e *AlertEvaluator) Shutdown(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if e.cancel != nil {
		e.cancel()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func evaluateAlertRules(ctx context.Context, service *AlertService) {
	if service == nil {
		return
	}
	_, failed, err := service.EvaluateAllContext(ctx)
	if err != nil && !errors.Is(err, context.Canceled) && logger.Logger != nil {
		logger.Warn("alert rule evaluation completed with failures", logger.Int("failed", failed), logger.Err(err))
	}
}
