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
	// Platform 目标社媒平台（wechat/xiaohongshu/zhihu/weibo/douyin），仅当
	// Kind=social_post 时生效，决定文案风格。空=通用图文。
	Platform string
}

// platformSystemPrompt 按目标社媒平台返回定制的 system prompt。所有平台都要求
// 输出统一 JSON {title, digest, content}，便于内容中台解析；content 的体裁按平台
// 调整（长图文 / 种草笔记 / 专业长文 / 短博文 / 口播脚本）。
func platformSystemPrompt(platform string) string {
	base := `请严格以 JSON 对象输出,格式为 {"title":"标题","digest":"摘要","content":"正文"}。` +
		"直接输出 JSON,不要添加解释、Markdown 代码块或任何额外说明。"
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "xiaohongshu", "redbook":
		return "你是小红书爆款笔记写手。写一篇口语化、有种草感的小红书笔记:标题带 emoji 和吸引点," +
			"正文分点、多用 emoji、结尾带 3-6 个 #话题# 标签。content 为纯文本(不要 HTML),换行用 \\n。" + base
	case "zhihu":
		return "你是知乎优质答主。写一篇专业、理性、有信息增量的知乎风格文章:逻辑清晰、有小标题、论据充分。" +
			"content 为正文 HTML,小标题用 <h3>,段落用 <p>。" + base
	case "weibo":
		return "你是微博运营。写一条简短有传播力的微博:140 字左右、口语化、结尾带 1-2 个 #话题#。" +
			"content 为纯文本(不要 HTML)。" + base
	case "douyin", "tiktok":
		return "你是短视频编导。为口播短视频写脚本:开头 3 秒钩子、中间要点、结尾引导互动。" +
			"content 为口播脚本纯文本,分镜/段落用 \\n 分隔。" + base
	default: // wechat 或未知 → 微信公众号图文
		return "你是微信公众号内容创作助手。撰写一篇适配公众号的图文:正式、有条理、适合长图文阅读。" +
			"content 为正文 HTML,段落用 <p> 标签。" + base
	}
}

// Compose drafts operator-facing content. The system prompt is customized
// per kind; unknown kinds fall back to a generic writing assistant.
func (s *InsightService) Compose(ctx context.Context, req ComposeRequest) (string, error) {
	var system string
	switch strings.ToLower(strings.TrimSpace(req.Kind)) {
	case "notice":
		system = "你是企业内部管理系统的公告撰写助手。请根据用户的要求撰写一份正式、简洁、面向企业内部员工的通知公告。" +
			"直接输出公告正文,不要添加解释或额外说明。"
	case "wechat_article":
		// 存量微信图文 kind,保持不变(向后兼容 mp M5 调用)。
		system = "你是微信公众号的内容创作助手。请根据用户的要求撰写一篇适配微信公众号的图文文章," +
			"并严格以 JSON 对象输出,格式为:" +
			`{"title":"标题","digest":"摘要不超过120字","content":"正文HTML,段落用<p>标签,适配微信公众号"}。` +
			"content 字段为正文的 HTML 片段,段落使用 <p> 标签包裹,适配微信公众号排版。" +
			"直接输出 JSON,不要添加解释、Markdown 代码块或任何额外说明。"
	case "social_post":
		// 社媒多平台 kind:按 Platform 切风格。
		system = platformSystemPrompt(req.Platform)
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

// GenerateImage produces AI images for prompt via the configured chat
// provider's image endpoint (OpenAI-compatible /v1/images/generations).
func (s *InsightService) GenerateImage(ctx context.Context, prompt string, opts aiclient.ImageOptions) (aiclient.ImageResult, error) {
	return s.providers.Chat.GenerateImage(ctx, prompt, opts)
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
