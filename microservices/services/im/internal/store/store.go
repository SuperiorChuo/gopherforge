package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) migrate() error {
	return s.db.AutoMigrate(
		&model.Site{},
		&model.Visitor{},
		&model.SkillGroup{},
		&model.AgentSkill{},
		&model.AgentPresence{},
		&model.Conversation{},
		&model.Message{},
	)
}

func (s *Store) seed() error {
	// default skill group
	var sgCount int64
	if err := s.db.Model(&model.SkillGroup{}).Count(&sgCount).Error; err != nil {
		return err
	}
	var defaultSG *model.SkillGroup
	if sgCount == 0 {
		sg := model.SkillGroup{
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
		if err := s.db.Where("code = ?", "default").First(&sg).Error; err == nil {
			defaultSG = &sg
		}
	}

	var n int64
	if err := s.db.Model(&model.Site{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		// ensure demo site has default skill group if empty
		if defaultSG != nil {
			_ = s.db.Model(&model.Site{}).
				Where("app_key = ? AND default_skill_group_id IS NULL", "demo").
				Update("default_skill_group_id", defaultSG.ID).Error
		}
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
		"null", // file:// demo pages
	})
	site := model.Site{
		AppKey:         "demo",
		AppSecret:      "demo-secret-change-me",
		Name:           "演示站点",
		AllowedOrigins: string(origins),
		WelcomeText:    "您好，我是在线客服，请问有什么可以帮您？",
		Status:         1,
	}
	if defaultSG != nil {
		site.DefaultSkillGroupID = &defaultSG.ID
	}
	return s.db.Create(&site).Error
}

// ---------- Sites ----------

func (s *Store) GetSiteByAppKey(appKey string) (*model.Site, error) {
	var site model.Site
	err := s.db.Where("app_key = ? AND status = 1", appKey).First(&site).Error
	if err != nil {
		return nil, err
	}
	return &site, nil
}

func (s *Store) ListSites() ([]model.Site, error) {
	var list []model.Site
	err := s.db.Order("id ASC").Find(&list).Error
	return list, err
}

func (s *Store) GetSite(id uint64) (*model.Site, error) {
	var site model.Site
	if err := s.db.First(&site, id).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

type SiteUpdate struct {
	Name                *string
	WelcomeText         *string
	AllowedOrigins      *string
	Status              *int16
	DefaultSkillGroupID *uint64
}

func (s *Store) UpdateSite(id uint64, u SiteUpdate) (*model.Site, error) {
	site, err := s.GetSite(id)
	if err != nil {
		return nil, err
	}
	if u.Name != nil {
		site.Name = *u.Name
	}
	if u.WelcomeText != nil {
		site.WelcomeText = *u.WelcomeText
	}
	if u.AllowedOrigins != nil {
		site.AllowedOrigins = *u.AllowedOrigins
	}
	if u.Status != nil {
		site.Status = *u.Status
	}
	if u.DefaultSkillGroupID != nil {
		if *u.DefaultSkillGroupID == 0 {
			site.DefaultSkillGroupID = nil
		} else {
			site.DefaultSkillGroupID = u.DefaultSkillGroupID
		}
	}
	if err := s.db.Save(site).Error; err != nil {
		return nil, err
	}
	return site, nil
}

// ---------- Visitors ----------

func (s *Store) UpsertVisitor(siteID uint64, guestKey, displayName string) (*model.Visitor, error) {
	var v model.Visitor
	err := s.db.Where("site_id = ? AND guest_key = ?", siteID, guestKey).First(&v).Error
	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		v = model.Visitor{
			SiteID:      siteID,
			GuestKey:    guestKey,
			DisplayName: displayName,
			LastSeenAt:  now,
		}
		if v.DisplayName == "" {
			v.DisplayName = "访客"
		}
		if err := s.db.Create(&v).Error; err != nil {
			return nil, err
		}
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	v.LastSeenAt = now
	if displayName != "" {
		v.DisplayName = displayName
	}
	if err := s.db.Save(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) GetVisitor(id uint64) (*model.Visitor, error) {
	var v model.Visitor
	if err := s.db.First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// ---------- Skill groups ----------

func (s *Store) ListSkillGroups() ([]model.SkillGroup, error) {
	var list []model.SkillGroup
	err := s.db.Order("id ASC").Find(&list).Error
	return list, err
}

func (s *Store) GetSkillGroup(id uint64) (*model.SkillGroup, error) {
	var g model.SkillGroup
	if err := s.db.First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) GetSkillGroupByCode(code string) (*model.SkillGroup, error) {
	var g model.SkillGroup
	if err := s.db.Where("code = ? AND status = 1", code).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

type SkillGroupInput struct {
	Name     string
	Code     string
	Strategy string
	Status   int16
}

func (s *Store) CreateSkillGroup(in SkillGroupInput) (*model.SkillGroup, error) {
	if in.Name == "" || in.Code == "" {
		return nil, errors.New("name and code required")
	}
	if in.Strategy == "" {
		in.Strategy = "round_robin"
	}
	if in.Status == 0 {
		in.Status = 1
	}
	g := model.SkillGroup{
		Name:     in.Name,
		Code:     in.Code,
		Strategy: in.Strategy,
		Status:   in.Status,
	}
	if err := s.db.Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) UpdateSkillGroup(id uint64, in SkillGroupInput) (*model.SkillGroup, error) {
	g, err := s.GetSkillGroup(id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		g.Name = in.Name
	}
	if in.Code != "" {
		g.Code = in.Code
	}
	if in.Strategy != "" {
		g.Strategy = in.Strategy
	}
	if in.Status != 0 {
		g.Status = in.Status
	}
	if err := s.db.Save(g).Error; err != nil {
		return nil, err
	}
	return g, nil
}

func (s *Store) ListAgentSkills(skillGroupID, agentUserID uint64) ([]model.AgentSkill, error) {
	q := s.db.Model(&model.AgentSkill{})
	if skillGroupID > 0 {
		q = q.Where("skill_group_id = ?", skillGroupID)
	}
	if agentUserID > 0 {
		q = q.Where("agent_user_id = ?", agentUserID)
	}
	var list []model.AgentSkill
	err := q.Order("id ASC").Find(&list).Error
	return list, err
}

func (s *Store) UpsertAgentSkill(agentUserID, skillGroupID uint64, maxConcurrent int, status int16) (*model.AgentSkill, error) {
	if agentUserID == 0 || skillGroupID == 0 {
		return nil, errors.New("agent_user_id and skill_group_id required")
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	if status == 0 {
		status = 1
	}
	var row model.AgentSkill
	err := s.db.Where("agent_user_id = ? AND skill_group_id = ?", agentUserID, skillGroupID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.AgentSkill{
			AgentUserID:   agentUserID,
			SkillGroupID:  skillGroupID,
			MaxConcurrent: maxConcurrent,
			Status:        status,
		}
		if err := s.db.Create(&row).Error; err != nil {
			return nil, err
		}
		return &row, nil
	}
	if err != nil {
		return nil, err
	}
	row.MaxConcurrent = maxConcurrent
	row.Status = status
	if err := s.db.Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) DeleteAgentSkill(id uint64) error {
	return s.db.Delete(&model.AgentSkill{}, id).Error
}

// ---------- Presence ----------

func (s *Store) UpsertPresence(agentUserID uint64, status, displayName string) (*model.AgentPresence, error) {
	if agentUserID == 0 {
		return nil, errors.New("agent_user_id required")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "online", "busy", "offline":
	default:
		return nil, errors.New("status must be online|busy|offline")
	}
	now := time.Now()
	var p model.AgentPresence
	err := s.db.First(&p, agentUserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		p = model.AgentPresence{
			AgentUserID: agentUserID,
			Status:      status,
			DisplayName: displayName,
			LastSeenAt:  now,
		}
		if err := s.db.Create(&p).Error; err != nil {
			return nil, err
		}
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	p.Status = status
	p.LastSeenAt = now
	if displayName != "" {
		p.DisplayName = displayName
	}
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPresence(agentUserID uint64) (*model.AgentPresence, error) {
	var p model.AgentPresence
	if err := s.db.First(&p, agentUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.AgentPresence{
				AgentUserID: agentUserID,
				Status:      "offline",
			}, nil
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListPresence(statuses ...string) ([]model.AgentPresence, error) {
	q := s.db.Model(&model.AgentPresence{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	var list []model.AgentPresence
	err := q.Order("agent_user_id ASC").Find(&list).Error
	return list, err
}

func (s *Store) CountAssignedForAgent(agentUserID uint64) (int64, error) {
	var n int64
	err := s.db.Model(&model.Conversation{}).
		Where("agent_user_id = ? AND status = ?", agentUserID, "assigned").
		Count(&n).Error
	return n, err
}

// ---------- Conversations ----------

func (s *Store) CreateConversation(siteID, visitorID uint64, channel, contextJSON string, skillGroupID *uint64) (*model.Conversation, error) {
	now := time.Now()
	c := model.Conversation{
		PublicID:     uuid.New(),
		SiteID:       siteID,
		Channel:      channel,
		VisitorID:    visitorID,
		SkillGroupID: skillGroupID,
		Status:       "queued",
		Context:      contextJSON,
		QueuedAt:     &now,
	}
	if c.Channel == "" {
		c.Channel = "h5"
	}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	// best-effort auto assign
	_ = s.TryAutoAssign(&c)
	return &c, nil
}

func (s *Store) GetConversationByPublicID(publicID string) (*model.Conversation, error) {
	id, err := uuid.Parse(publicID)
	if err != nil {
		return nil, err
	}
	var c model.Conversation
	if err := s.db.Where("public_id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListAgentConversations scope: all | mine | queue
func (s *Store) ListAgentConversations(agentUserID uint64, scope string, skillGroupID uint64, limit int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	q := s.db.Model(&model.Conversation{})
	switch scope {
	case "mine":
		q = q.Where("status = ? AND agent_user_id = ?", "assigned", agentUserID)
	case "queue":
		q = q.Where("status = ?", "queued")
		if skillGroupID > 0 {
			q = q.Where("skill_group_id = ?", skillGroupID)
		}
	default: // all open
		q = q.Where("status IN ?", []string{"queued", "assigned"})
		if skillGroupID > 0 {
			q = q.Where("(skill_group_id = ? OR skill_group_id IS NULL)", skillGroupID)
		}
	}
	var list []model.Conversation
	err := q.Order("last_message_at DESC NULLS LAST, created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (s *Store) AssignConversation(c *model.Conversation, agentUserID uint64) error {
	now := time.Now()
	c.AgentUserID = &agentUserID
	c.Status = "assigned"
	c.AssignedAt = &now
	return s.db.Save(c).Error
}

type CloseOpts struct {
	Reason string
}

func (s *Store) CloseConversation(c *model.Conversation, opts CloseOpts) error {
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if opts.Reason != "" {
		c.CloseReason = opts.Reason
	} else if c.CloseReason == "" {
		c.CloseReason = "agent"
	}
	return s.db.Save(c).Error
}

// TransferConversation moves session to another agent, or re-queues.
// targetAgentID == 0 → re-queue (optionally to new skill group).
func (s *Store) TransferConversation(c *model.Conversation, fromAgentID, targetAgentID uint64, skillGroupID *uint64, note string) error {
	if c.Status == "closed" {
		return errors.New("conversation already closed")
	}
	now := time.Now()
	if skillGroupID != nil {
		if *skillGroupID == 0 {
			c.SkillGroupID = nil
		} else {
			c.SkillGroupID = skillGroupID
		}
	}
	if targetAgentID == 0 {
		c.AgentUserID = nil
		c.Status = "queued"
		c.QueuedAt = &now
		c.AssignedAt = nil
		if err := s.db.Save(c).Error; err != nil {
			return err
		}
		_ = s.AppendSystemEvent(c.ID, "transferred_queue", map[string]any{
			"from_agent_user_id": fromAgentID,
			"skill_group_id":     c.SkillGroupID,
			"note":               note,
		})
		// try auto assign after re-queue
		_ = s.TryAutoAssign(c)
		return nil
	}
	c.AgentUserID = &targetAgentID
	c.Status = "assigned"
	c.AssignedAt = &now
	if err := s.db.Save(c).Error; err != nil {
		return err
	}
	_ = s.AppendSystemEvent(c.ID, "transferred", map[string]any{
		"from_agent_user_id": fromAgentID,
		"to_agent_user_id":   targetAgentID,
		"skill_group_id":     c.SkillGroupID,
		"note":               note,
	})
	return nil
}

// TryAutoAssign picks an online agent in the skill group by strategy.
func (s *Store) TryAutoAssign(c *model.Conversation) error {
	if c == nil || c.Status != "queued" {
		return nil
	}
	sgID := uint64(0)
	if c.SkillGroupID != nil {
		sgID = *c.SkillGroupID
	}
	if sgID == 0 {
		// no skill group → skip auto assign (manual accept only)
		return nil
	}
	sg, err := s.GetSkillGroup(sgID)
	if err != nil || sg.Status != 1 {
		return err
	}
	if sg.Strategy == "manual" {
		return nil
	}

	var skills []model.AgentSkill
	if err := s.db.Where("skill_group_id = ? AND status = 1", sgID).Find(&skills).Error; err != nil {
		return err
	}
	if len(skills) == 0 {
		return nil
	}

	// filter online/busy agents under max concurrent
	type candidate struct {
		UserID   uint64
		Load     int64
		Max      int
		Status   string
	}
	var cands []candidate
	for _, sk := range skills {
		p, err := s.GetPresence(sk.AgentUserID)
		if err != nil {
			continue
		}
		if p.Status != "online" && p.Status != "busy" {
			continue
		}
		// busy only accepts if strategy needs and still under load? skip busy for auto
		if p.Status == "busy" {
			continue
		}
		load, err := s.CountAssignedForAgent(sk.AgentUserID)
		if err != nil {
			continue
		}
		if int(load) >= sk.MaxConcurrent {
			continue
		}
		cands = append(cands, candidate{UserID: sk.AgentUserID, Load: load, Max: sk.MaxConcurrent, Status: p.Status})
	}
	if len(cands) == 0 {
		return nil
	}

	var pick uint64
	switch sg.Strategy {
	case "least_load":
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].Load == cands[j].Load {
				return cands[i].UserID < cands[j].UserID
			}
			return cands[i].Load < cands[j].Load
		})
		pick = cands[0].UserID
	default: // round_robin
		sort.Slice(cands, func(i, j int) bool { return cands[i].UserID < cands[j].UserID })
		// pick next after RRCursor
		pick = cands[0].UserID
		for i, ca := range cands {
			if ca.UserID > sg.RRCursor {
				pick = ca.UserID
				// rotate from this index
				_ = i
				break
			}
		}
		// if all <= cursor, wrap to first
		if pick == cands[0].UserID && cands[len(cands)-1].UserID <= sg.RRCursor {
			pick = cands[0].UserID
		}
		// update cursor
		_ = s.db.Model(sg).Update("rr_cursor", pick).Error
	}

	if err := s.AssignConversation(c, pick); err != nil {
		return err
	}
	_ = s.AppendSystemEvent(c.ID, "assigned", map[string]any{
		"agent_user_id":  pick,
		"skill_group_id": sgID,
		"strategy":       sg.Strategy,
		"auto":           true,
	})
	return nil
}

func (s *Store) AppendSystemEvent(conversationID uint64, event string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["event"] = event
	content := JSONText(payload)
	msg := &model.Message{
		ConversationID: conversationID,
		SenderType:     "system",
		MsgType:        "event",
		Content:        content,
	}
	return s.CreateMessage(msg)
}

func (s *Store) EnsureOpenConversation(siteID, visitorID uint64, channel, contextJSON string, skillGroupID *uint64) (*model.Conversation, error) {
	var c model.Conversation
	err := s.db.Where("visitor_id = ? AND status IN ?", visitorID, []string{"queued", "assigned", "created", "bot_serving"}).
		Order("id DESC").First(&c).Error
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return s.CreateConversation(siteID, visitorID, channel, contextJSON, skillGroupID)
}

// ---------- Messages ----------

func (s *Store) NextSeq(conversationID uint64) (int64, error) {
	var maxSeq *int64
	err := s.db.Model(&model.Message{}).
		Select("MAX(seq)").
		Where("conversation_id = ?", conversationID).
		Scan(&maxSeq).Error
	if err != nil {
		return 0, err
	}
	if maxSeq == nil {
		return 1, nil
	}
	return *maxSeq + 1, nil
}

func (s *Store) CreateMessage(m *model.Message) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		seq, err := nextSeqTx(tx, m.ConversationID)
		if err != nil {
			return err
		}
		m.Seq = seq
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		// system events still update last preview lightly
		preview := previewFromContent(m.Content)
		if m.MsgType == "event" {
			preview = "[系统] " + eventName(m.Content)
		}
		now := time.Now()
		return tx.Model(&model.Conversation{}).Where("id = ?", m.ConversationID).Updates(map[string]any{
			"last_message_at":      now,
			"last_message_preview": preview,
		}).Error
	})
}

func eventName(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return "事件"
	}
	if e, ok := m["event"].(string); ok {
		return e
	}
	return "事件"
}

func nextSeqTx(tx *gorm.DB, conversationID uint64) (int64, error) {
	// lock conversation row to serialize seq
	var c model.Conversation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, conversationID).Error; err != nil {
		return 0, err
	}
	var maxSeq *int64
	if err := tx.Model(&model.Message{}).Select("MAX(seq)").Where("conversation_id = ?", conversationID).Scan(&maxSeq).Error; err != nil {
		return 0, err
	}
	if maxSeq == nil {
		return 1, nil
	}
	return *maxSeq + 1, nil
}

func previewFromContent(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return truncate(content, 200)
	}
	if t, ok := m["text"].(string); ok {
		return truncate(t, 200)
	}
	return "[消息]"
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func (s *Store) ListMessages(conversationID uint64, afterSeq int64, limit int) ([]model.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.Where("conversation_id = ?", conversationID)
	if afterSeq > 0 {
		q = q.Where("seq > ?", afterSeq)
	}
	var list []model.Message
	err := q.Order("seq ASC").Limit(limit).Find(&list).Error
	return list, err
}

// ---------- helpers ----------

func JSONText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func OriginAllowed(allowedJSON, origin string) bool {
	if origin == "" {
		return true // non-browser clients
	}
	var list []string
	if err := json.Unmarshal([]byte(allowedJSON), &list); err != nil {
		// tolerate misconfigured legacy rows: deny
		return false
	}
	if len(list) == 0 {
		return true
	}
	for _, o := range list {
		o = strings.TrimSpace(o)
		if o == "*" {
			return true
		}
		if strings.EqualFold(o, strings.TrimSpace(origin)) {
			return true
		}
	}
	return false
}

func FmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
