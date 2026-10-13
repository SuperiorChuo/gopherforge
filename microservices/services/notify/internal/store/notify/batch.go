package store

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/model"
)

// createBatchSize 单条 INSERT 携带的行数上限（PG 参数上限 65535，Message 12 列，
// 200 行远在安全区内）。
const createBatchSize = 200

// BatchSendInput 是 fan-out 的批量入参：一次渲染、一次租户校验、一次去重、
// 一条批量 INSERT，替代原来 per-user 的 5 条查询。
type BatchSendInput struct {
	TenantID     uint64
	UserIDs      []uint64
	TemplateCode string
	Type         string
	RefType      string
	RefID        string
	Vars         map[string]string
	Title        string
	Content      string
	Link         string
	Channels     []string
}

// BatchRecipient 是一个已受理的收件人：站内信已落库（MessageID>0 时），
// 外部渠道投递所需的联系方式随查询一并取回，无需二次 SELECT。
type BatchRecipient struct {
	UserID    uint64
	MessageID uint64
	Contact   channel.Contact
}

// BatchSendResult 计数语义与逐条 Send 完全一致：
// Created=受理人数（站内信落库，或纯外部渠道的受理），Skipped=24h 内重复被抑制，
// Rejected=收件人不属于该租户（跨租户投递被拒，不落库）。
type BatchSendResult struct {
	Rendered        Rendered
	Created         int
	Skipped         int
	Rejected        int
	RejectedUserIDs []uint64
	Recipients      []BatchRecipient
}

// SendBatch 批量落站内信并返回外发所需的收件人列表（外部渠道由调用方异步投递）。
//
// 与单条 Send 的语义对齐：
//   - 模板只解析一次（含 fallback 到 tenant 1 平台模板），不再 per-user 重复 SELECT；
//   - 收件人租户归属用一条 `id IN (?) AND tenant_id = ?` 批量校验，
//     users 表不可读时沿用软读惯例放行（与 userInTenant 一致）；
//   - ref 去重用一条查询取回"已存在"的 user_id 集合；
//   - 同一批里重复出现的同一 user 只受理一次，多出的计入 Skipped。
func (s *Store) SendBatch(ctx context.Context, in BatchSendInput) (*BatchSendResult, error) {
	if in.TenantID == 0 {
		in.TenantID = 1
	}
	ids := make([]uint64, 0, len(in.UserIDs))
	seen := make(map[uint64]bool, len(in.UserIDs))
	dupes := 0
	for _, id := range in.UserIDs {
		if id == 0 {
			continue
		}
		if seen[id] {
			dupes++
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("user_id required")
	}

	// 模板解析一次（循环外）：渲染结果与渠道对整批收件人相同。
	r := s.resolveTemplate(SendInput{
		TenantID:     in.TenantID,
		TemplateCode: in.TemplateCode,
		Type:         in.Type,
		Vars:         in.Vars,
		Title:        in.Title,
		Content:      in.Content,
		Link:         in.Link,
		Channels:     in.Channels,
	})
	if strings.TrimSpace(r.Title) == "" {
		return nil, errors.New("title required")
	}

	out := &BatchSendResult{Rendered: r, Skipped: dupes}

	allowed, contacts := s.filterTenantUsers(ctx, in.TenantID, ids)
	var duplicated map[uint64]bool
	if in.RefID != "" && in.RefType != "" {
		duplicated = s.existingRefUsers(in.TenantID, ids, in.RefType, in.RefID, r.Code)
	}

	targets := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !allowed[id] {
			out.Rejected++
			out.RejectedUserIDs = append(out.RejectedUserIDs, id)
			continue
		}
		if duplicated[id] {
			out.Skipped++
			continue
		}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return out, nil
	}

	inbox := channel.Has(r.Channels, "inbox")
	msgs := make([]model.Message, 0, len(targets))
	if inbox {
		for _, id := range targets {
			msgs = append(msgs, model.Message{
				TenantID:     in.TenantID,
				UserID:       id,
				TemplateCode: r.Code,
				Title:        r.Title,
				Content:      r.Content,
				Link:         r.Link,
				Type:         r.Type,
				RefType:      in.RefType,
				RefID:        in.RefID,
			})
		}
		if err := s.db.CreateInBatches(&msgs, createBatchSize).Error; err != nil {
			return nil, err
		}
	}

	out.Recipients = make([]BatchRecipient, 0, len(targets))
	for i, id := range targets {
		rec := BatchRecipient{UserID: id, Contact: contacts[id]}
		rec.Contact.UserID = id
		if inbox {
			rec.MessageID = msgs[i].ID
		}
		out.Recipients = append(out.Recipients, rec)
		out.Created++
	}
	return out, nil
}

// filterTenantUsers 批量取"租户归属校验 + 联系方式"，Phase 2C 改走 identity owner API
// （userContacts），不再直查共享 users 表。identity 不可达沿用软读降级。
func (s *Store) filterTenantUsers(ctx context.Context, tenantID uint64, ids []uint64) (map[uint64]bool, map[uint64]channel.Contact) {
	return s.userContacts(ctx, tenantID, ids)
}

// existingRefUsers 一条查询取回 24h 内已收到同 (ref_type, ref_id[, template_code])
// 消息的 user_id 集合，替代 per-user 的 COUNT。查询失败时返回空集（不抑制），
// 与原 `_ = q.Count(&n)` 忽略错误的口径一致。
func (s *Store) existingRefUsers(tenantID uint64, ids []uint64, refType, refID, code string) map[uint64]bool {
	out := make(map[uint64]bool, len(ids))
	if len(ids) == 0 {
		return out
	}
	since := time.Now().Add(-24 * time.Hour)
	q := s.db.Model(&model.Message{}).
		Where("tenant_id = ? AND user_id IN ? AND ref_type = ? AND ref_id = ? AND created_at > ?",
			tenantID, ids, refType, refID, since)
	if code != "" {
		q = q.Where("template_code = ?", code)
	}
	var found []uint64
	if err := q.Pluck("user_id", &found).Error; err != nil {
		log.Printf("notify: batch dedupe lookup failed (tenant=%d ref=%s/%s): %v", tenantID, refType, refID, err)
		return out
	}
	for _, id := range found {
		out[id] = true
	}
	return out
}

// SaveDeliveryLogsBatch 一条 INSERT 写多行投递日志（原实现逐条 Create）。
func (s *Store) SaveDeliveryLogsBatch(logs []model.DeliveryLog) {
	if len(logs) == 0 {
		return
	}
	if err := s.db.CreateInBatches(&logs, createBatchSize).Error; err != nil {
		log.Printf("notify: save delivery logs failed (n=%d): %v", len(logs), err)
	}
}

// BuildDeliveryLogs 把一次外发结果转成日志行（供批量写入）。
func BuildDeliveryLogs(tenantID, userID, messageID uint64, code, refType, refID string, results []channel.Result) []model.DeliveryLog {
	if len(results) == 0 {
		return nil
	}
	logs := make([]model.DeliveryLog, 0, len(results))
	for _, r := range results {
		logs = append(logs, model.DeliveryLog{
			TenantID:     tenantID,
			UserID:       userID,
			MessageID:    messageID,
			TemplateCode: code,
			Channel:      r.Channel,
			Target:       r.Target,
			Status:       r.Status,
			Detail:       truncate(r.Detail, 500),
			RefType:      refType,
			RefID:        refID,
		})
	}
	return logs
}
