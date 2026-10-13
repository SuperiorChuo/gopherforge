package store

import (
	"encoding/json"
	"fmt"
	"github.com/go-admin-kit/services/im/internal/model"
)

// adminListLimit 兜住管理端「小表全量列表」的单次读取规模（站点、技能组、
// 坐席绑定、快捷回复）。这些表按租户天然是几十到几百行，加上限只是防止某个
// 租户异常膨胀时把整表拉进内存。
const adminListLimit = 1000

func (s *Store) migrate() error {
	if err := s.db.AutoMigrate(
		&model.Site{},
		&model.Visitor{},
		&model.SkillGroup{},
		&model.AgentSkill{},
		&model.AgentPresence{},
		&model.Conversation{},
		&model.Message{},
		&model.QuickReply{},
		&model.KBMiss{},
		&model.KBDailyStat{},
		&model.SensitiveWord{},
	); err != nil {
		return err
	}
	if err := s.ensureIndexes(); err != nil {
		return err
	}
	return s.backfillConversationLastSeq()
}

// backfillConversationLastSeq 把新增的 im_conversations.last_seq 回填为该会话
// 现有 MAX(im_messages.seq)。AutoMigrate 给新列填的是 0，若不回填，存量会话的
// 下一条消息会从 seq=1 重发，撞 ux_im_messages_conv_seq 唯一索引并让客户端拉取
// 出现重号。幂等：只修 last_seq 落后于实际 MAX(seq) 的行，重复执行是空操作，
// 因此每次启动都跑也安全（新库直接 0 行命中）。
func (s *Store) backfillConversationLastSeq() error {
	var stmt string
	switch s.db.Dialector.Name() {
	case "postgres":
		stmt = `UPDATE im_conversations c SET last_seq = m.max_seq
			FROM (SELECT conversation_id, MAX(seq) AS max_seq FROM im_messages GROUP BY conversation_id) m
			WHERE m.conversation_id = c.id AND c.last_seq < m.max_seq`
	default: // sqlite (tests)
		stmt = `UPDATE im_conversations SET last_seq = (
				SELECT MAX(seq) FROM im_messages WHERE conversation_id = im_conversations.id
			) WHERE last_seq < (
				SELECT COALESCE(MAX(seq), 0) FROM im_messages WHERE conversation_id = im_conversations.id
			)`
	}
	if err := s.db.Exec(stmt).Error; err != nil {
		return fmt.Errorf("backfill im_conversations.last_seq: %w", err)
	}
	return nil
}

// ensureIndexes 维护 gorm tag 表达不了的索引（partial、DESC NULLS LAST、跨列唯一），
// 并清理被复合索引取代的旧单列索引；全部幂等。
// (conversation_id, seq) 唯一索引两种方言都建：它是 seq 不重号的最终保证，
// 测试（sqlite）要能真的踩到这条约束，否则并发/自愈行为无法验证。
// 其余 partial / DESC NULLS LAST 索引是 PG 专属语法，仅 PG 执行。
func (s *Store) ensureIndexes() error {
	// 消息按会话 + seq 唯一：seq 由会话计数器原子分配，唯一索引兜底防重，
	// 同时服务拉取历史（seq > ? ORDER BY seq）。
	if err := s.db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_im_messages_conv_seq ON im_messages (conversation_id, seq)`,
	).Error; err != nil {
		return fmt.Errorf("ensure ux_im_messages_conv_seq: %w", err)
	}
	if s.db.Dialector.Name() != "postgres" {
		return nil
	}
	stmts := []string{
		// widget 断线重发的 client_msg_id 幂等查找
		`CREATE INDEX IF NOT EXISTS idx_im_messages_conv_client ON im_messages (conversation_id, client_msg_id) WHERE client_msg_id IS NOT NULL`,
		// 工作台会话列表：tenant+status 过滤 + last_message_at DESC NULLS LAST 排序
		`CREATE INDEX IF NOT EXISTS idx_im_conversations_wb_list ON im_conversations (tenant_id, status, last_message_at DESC NULLS LAST)`,
		// 坐席在接会话数统计 / 我的会话列表
		`CREATE INDEX IF NOT EXISTS idx_im_conversations_agent_status ON im_conversations (agent_user_id, status)`,
		// 过期会话归档清理（status='closed' AND closed_at < cutoff）
		`CREATE INDEX IF NOT EXISTS idx_im_conversations_closed_cleanup ON im_conversations (closed_at) WHERE status = 'closed'`,
		// 被上面复合索引取代的旧索引
		`DROP INDEX IF EXISTS idx_conv_seq`,
		`DROP INDEX IF EXISTS idx_im_conversations_tenant_id`,
		`DROP INDEX IF EXISTS idx_im_conversations_status`,
		`DROP INDEX IF EXISTS idx_im_conversations_agent_user_id`,
		`DROP INDEX IF EXISTS idx_im_conversations_tenant_status`,
	}
	for _, stmt := range stmts {
		if err := s.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure index (%s): %w", stmt, err)
		}
	}
	return nil
}

func (s *Store) seed() error {
	// 仅给 tenant 1 提供默认技能组（全局种子数据）
	const seedTenant uint64 = 1
	// 唯一性检查前回填缺失的 tenant_id（旧行可能仍为 0）
	_ = s.db.Model(&model.Site{}).Where("tenant_id = ?", 0).Update("tenant_id", seedTenant).Error
	_ = s.db.Model(&model.SkillGroup{}).Where("tenant_id = ?", 0).Update("tenant_id", seedTenant).Error
	_ = s.db.Model(&model.Conversation{}).Where("tenant_id = ?", 0).Update("tenant_id", seedTenant).Error

	var sgCount int64
	if err := s.db.Model(&model.SkillGroup{}).Where("tenant_id = ?", seedTenant).Count(&sgCount).Error; err != nil {
		return err
	}
	var defaultSG *model.SkillGroup
	if sgCount == 0 {
		sg := model.SkillGroup{
			TenantID: seedTenant,
			Name:     "默认客服组",
			Code:     "default",
			Strategy: "round_robin",
			Status:   1,
		}
		if err := s.db.Create(&sg).Error; err != nil {
			return err
		}
		defaultSG = &sg
	} else {
		var sg model.SkillGroup
		if err := s.db.Where("tenant_id = ? AND code = ?", seedTenant, "default").First(&sg).Error; err == nil {
			defaultSG = &sg
		}
	}

	var n int64
	if err := s.db.Model(&model.Site{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		// 确保演示站点在没有默认技能组时补上
		if defaultSG != nil {
			_ = s.db.Model(&model.Site{}).
				Where("app_key = ? AND default_skill_group_id IS NULL", "demo").
				Update("default_skill_group_id", defaultSG.ID).Error
		}
		// M4: 列刚新增时一次性为 demo 开启 bot（false + 空 prompt）
		_ = s.db.Exec(`
			UPDATE im_sites SET bot_enabled = true
			WHERE app_key = 'demo' AND bot_enabled = false
			  AND (bot_system_prompt IS NULL OR bot_system_prompt = '')
			  AND welcome_text LIKE '%在线客服%'
		`).Error
		return nil
	}
	origins, _ := json.Marshal([]string{
		"http://localhost:8000",
		"http://127.0.0.1:8000",
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://localhost:8088",
		"http://127.0.0.1:8088",
		"http://localhost:5174",
		"http://127.0.0.1:5174",
		"null", // file:// 演示页面
	})
	site := model.Site{
		TenantID:       seedTenant,
		AppKey:         "demo",
		AppSecret:      "demo-secret-change-me",
		Name:           "演示站点",
		AllowedOrigins: string(origins),
		WelcomeText:    "您好，我是智能客服助手。可直接提问，或回复「转人工」接入坐席。",
		Status:         1,
		BotEnabled:     true,
	}
	if defaultSG != nil {
		site.DefaultSkillGroupID = &defaultSG.ID
	}
	return s.db.Create(&site).Error
}
