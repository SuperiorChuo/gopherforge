package sms

import (
	"context"

	"github.com/go-admin-kit/services/shared/pkg/logger"
)

// DebugSender 开发联调用：不真正外发，只写日志并直接成功。
type DebugSender struct{}

// NewDebugSender 构造 debug 发送器。
func NewDebugSender() *DebugSender { return &DebugSender{} }

func (s *DebugSender) Provider() string { return ProviderDebug }

// Send 记录一条日志即视为成功；MessageID 留空表示无云厂商回执。
func (s *DebugSender) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	if logger.Logger != nil {
		logger.Info("debug sms sent",
			logger.String("mobile", req.Mobile),
			logger.String("content", req.Content),
		)
	}
	return &SendResult{}, nil
}
