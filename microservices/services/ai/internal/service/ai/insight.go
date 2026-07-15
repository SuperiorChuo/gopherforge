package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
)

// defaultInsightDays is the report window when the caller omits days.
const defaultInsightDays = 7

// InsightService turns aggregated audit data into AI-generated reports and
// drafts operator-facing content.
type InsightService struct {
	insights  *aidao.InsightDAO
	providers aiclient.Providers
}

// NewInsightServiceWithDB builds an InsightService backed by an injected
// database handle and provider set.
func NewInsightServiceWithDB(db *gorm.DB, providers aiclient.Providers) InsightService {
	return InsightService{
		insights:  aidao.NewInsightDAO(db),
		providers: providers,
	}
}

// GenerateLogInsight aggregates login and operation logs over the last days
// days and asks the model for a Chinese markdown security report.
func (s *InsightService) GenerateLogInsight(ctx context.Context, days int) (string, error) {
	if days <= 0 {
		days = defaultInsightDays
	}
	since := time.Now().AddDate(0, 0, -days)

	loginStats, err := s.insights.LoginStatsSinceContext(ctx, since)
	if err != nil {
		return "", fmt.Errorf("aggregate login logs: %w", err)
	}
	operationStats, err := s.insights.OperationStatsSinceContext(ctx, since)
	if err != nil {
		return "", fmt.Errorf("aggregate operation logs: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"window_days":    days,
		"login_stats":    loginStats,
		"operation_logs": operationStats,
	})
	if err != nil {
		return "", fmt.Errorf("encode insight payload: %w", err)
	}

	msgs := []aiclient.ChatMessage{
		{
			Role: aiclient.RoleSystem,
			Content: "你是企业内部管理系统的安全分析师。根据用户提供的登录日志与操作日志聚合数据,输出一份中文 Markdown 安全分析报告。" +
				"报告需包含:整体概览、登录安全分析(成功率、失败原因、异常 IP 迹象)、操作行为分析(高频模块与操作人)、风险提示与改进建议。" +
				"只依据给出的数据,不要编造数字。",
		},
		{
			Role:    aiclient.RoleUser,
			Content: fmt.Sprintf("请分析最近 %d 天的日志聚合数据并生成报告:\n%s", days, string(payload)),
		},
	}

	return s.complete(ctx, msgs)
}

// ComposeRequest describes a content-drafting request.
type ComposeRequest struct {
	Kind   string
	Prompt string
	Draft  string
}

// Compose drafts operator-facing content. The system prompt is customized
// per kind; unknown kinds fall back to a generic writing assistant.
func (s *InsightService) Compose(ctx context.Context, req ComposeRequest) (string, error) {
	var system string
	switch strings.ToLower(strings.TrimSpace(req.Kind)) {
	case "notice":
		system = "你是企业内部管理系统的公告撰写助手。请根据用户的要求撰写一份正式、简洁、面向企业内部员工的通知公告。" +
			"直接输出公告正文,不要添加解释或额外说明。"
	default:
		system = "你是企业内部管理系统的写作助手。请根据用户的要求撰写内容,直接输出正文,不要添加解释。"
	}

	userPrompt := req.Prompt
	if strings.TrimSpace(req.Draft) != "" {
		userPrompt = fmt.Sprintf("%s\n\n以下是现有草稿,请在其基础上修改完善:\n%s", req.Prompt, req.Draft)
	}

	msgs := []aiclient.ChatMessage{
		{Role: aiclient.RoleSystem, Content: system},
		{Role: aiclient.RoleUser, Content: userPrompt},
	}
	return s.complete(ctx, msgs)
}

// complete runs a streaming chat call and collects the full reply.
func (s *InsightService) complete(ctx context.Context, msgs []aiclient.ChatMessage) (string, error) {
	var reply strings.Builder
	err := s.providers.Chat.Chat(ctx, msgs, func(delta aiclient.ChatDelta) error {
		if !delta.Done {
			reply.WriteString(delta.Content)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return reply.String(), nil
}
