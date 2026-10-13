package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
)

// Alertmanager webhook 接收端（运维告警闭环）：Prometheus 规则触发/恢复 →
// Alertmanager 分组去抖 → 本端点 → 站内信通知运维管理员。
// 鉴权双轨：X-Internal-Token（内部服务惯例）或 Authorization: Bearer
// （Alertmanager http_config.authorization 只会带这个头）。
// 去重：ref = fingerprint+status+startsAt，Alertmanager repeat_interval 的
// 重复推送不产生重复站内信；恢复（resolved）作为新 ref 单独通知一次。

// amWebhook 是 Alertmanager webhook payload 的业务相关子集。
type amWebhook struct {
	Status string    `json:"status"`
	Alerts []amAlert `json:"alerts"`
}

type amAlert struct {
	Status      string            `json:"status"` // firing / resolved
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// requireInternalOrBearer 同 requireInternal，另放行 Bearer 形态（Alertmanager）。
func (s *Server) requireInternalOrBearer(c *gin.Context) bool {
	if s.InternalToken == "" {
		fail(c, http.StatusServiceUnavailable, "internal notify not configured")
		return false
	}
	if c.GetHeader("X-Internal-Token") == s.InternalToken {
		return true
	}
	if strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ") == s.InternalToken {
		return true
	}
	fail(c, http.StatusUnauthorized, "invalid internal token")
	return false
}

// ReceiveAlerts POST /api/v1/notify/internal/alerts（Alertmanager 直连也走此端点）。
// AlertUserID 为收件人（平台运维管理员，NOTIFY_ALERT_USER 配置，默认 1）。
func (s *Server) ReceiveAlerts(c *gin.Context) {
	if !s.requireInternalOrBearer(c) {
		return
	}
	var payload amWebhook
	if err := c.ShouldBindJSON(&payload); err != nil {
		fail(c, http.StatusBadRequest, "invalid alertmanager payload")
		return
	}
	created, skipped, err := s.processAlerts(c.Request.Context(), payload, s.AlertUserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "persist alert failed")
		return
	}
	ok(c, gin.H{"created": created, "skipped": skipped})
}

// processAlerts 把一条 webhook 里的所有 alert 转站内信，返回 created/skipped。
// 幂等：同 ref（fingerprint-status-startsAt）重复不重复投递（store 层 duplicate 判定）。
// 供 HTTP ReceiveAlerts 与 gRPC InternalAlert 共用。err 非 nil 时上层应 500。
func (s *Server) processAlerts(ctx context.Context, payload amWebhook, userID uint64) (created, skipped int, err error) {
	if userID == 0 {
		userID = 1
	}
	for _, a := range payload.Alerts {
		name := a.Labels["alertname"]
		if name == "" {
			continue
		}
		title, content := renderAlert(a)
		vars := map[string]string{
			"alertname":   name,
			"status":      a.Status,
			"severity":    a.Labels["severity"],
			"summary":     a.Annotations["summary"],
			"description": a.Annotations["description"],
		}
		result, err := s.Store.Send(ctx, store.SendInput{
			TenantID:     1,
			UserID:       userID,
			TemplateCode: "ops_alert",
			Vars:         vars,
			Type:         "ops_alert",
			RefType:      "prom_alert",
			RefID:        fmt.Sprintf("%s-%s-%s", a.Fingerprint, a.Status, a.StartsAt),
			Title:        title,
			Content:      content,
			Link:         "/monitor/grafana",
		})
		if err != nil {
			if errors.Is(err, store.ErrUserNotInTenant) {
				// AlertUserID 配错到别的租户：跳过并记日志，不产生跨租户投递也不 500。
				log.Printf("notify alerts: alert user %d not in tenant 1, skipped", userID)
				skipped++
				continue
			}
			if strings.Contains(err.Error(), "duplicate") {
				skipped++
				continue
			}
			return created, skipped, err
		}
		if s.Outbox != nil && (channel.Has(result.Channels, "email") || channel.Has(result.Channels, "sms")) {
			messageID := uint64(0)
			if result.Message != nil {
				messageID = uint64(result.Message.ID)
			}
			s.Outbox.Enqueue(outboxJob{
				tenantID: 1, userID: userID, messageID: messageID,
				code: result.Code, refType: "prom_alert",
				refID:    fmt.Sprintf("%s-%s-%s", a.Fingerprint, a.Status, a.StartsAt),
				channels: result.Channels, contact: s.Store.GetUserContact(ctx, 1, userID),
				payload: channel.Payload{
					TenantID: 1, TemplateCode: result.Code, Params: vars,
					Title: result.Title, Content: result.Content, Link: result.Link,
				},
			})
		}
		created++
	}
	return created, skipped, nil
}

// renderAlert 拼站内信标题与正文：标题带触发/恢复语义，正文含摘要与关键标签。
func renderAlert(a amAlert) (title, content string) {
	name := a.Labels["alertname"]
	if a.Status == "resolved" {
		title = fmt.Sprintf("【恢复】%s", name)
	} else {
		sev := a.Labels["severity"]
		if sev == "" {
			sev = "warning"
		}
		title = fmt.Sprintf("【告警·%s】%s", sev, name)
	}
	var b strings.Builder
	if v := a.Annotations["summary"]; v != "" {
		b.WriteString(v)
	}
	if v := a.Annotations["description"]; v != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(v)
	}
	// 关键标签排序输出（稳定）；跳过冗余项。
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		if k == "alertname" || k == "severity" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+a.Labels[k])
		}
		b.WriteString(strings.Join(parts, " "))
	}
	if a.StartsAt != "" {
		b.WriteString("\n开始于 " + a.StartsAt)
	}
	return title, b.String()
}
