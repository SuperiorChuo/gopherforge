package store

import (
	"encoding/json"
	"errors"
	"fmt"
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
	return s.db.AutoMigrate(&model.Site{}, &model.Visitor{}, &model.Conversation{}, &model.Message{})
}

func (s *Store) seed() error {
	var n int64
	if err := s.db.Model(&model.Site{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	origins, _ := json.Marshal([]string{"http://localhost:8000", "http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:8088"})
	site := model.Site{
		AppKey:         "demo",
		AppSecret:      "demo-secret-change-me",
		Name:           "演示站点",
		AllowedOrigins: string(origins),
		WelcomeText:    "您好，我是在线客服，请问有什么可以帮您？",
		Status:         1,
	}
	return s.db.Create(&site).Error
}

func (s *Store) GetSiteByAppKey(appKey string) (*model.Site, error) {
	var site model.Site
	err := s.db.Where("app_key = ? AND status = 1", appKey).First(&site).Error
	if err != nil {
		return nil, err
	}
	return &site, nil
}

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

func (s *Store) CreateConversation(siteID, visitorID uint64, channel, contextJSON string) (*model.Conversation, error) {
	now := time.Now()
	c := model.Conversation{
		PublicID:  uuid.New(),
		SiteID:    siteID,
		Channel:   channel,
		VisitorID: visitorID,
		Status:    "queued",
		Context:   contextJSON,
		QueuedAt:  &now,
	}
	if c.Channel == "" {
		c.Channel = "h5"
	}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
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

func (s *Store) ListAgentConversations(limit int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []model.Conversation
	err := s.db.Where("status IN ?", []string{"queued", "assigned"}).
		Order("last_message_at DESC NULLS LAST, created_at DESC").
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

func (s *Store) CloseConversation(c *model.Conversation) error {
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	return s.db.Save(c).Error
}

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
		preview := previewFromContent(m.Content)
		now := time.Now()
		return tx.Model(&model.Conversation{}).Where("id = ?", m.ConversationID).Updates(map[string]any{
			"last_message_at":      now,
			"last_message_preview": preview,
		}).Error
	})
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

func (s *Store) EnsureOpenConversation(siteID, visitorID uint64, channel, contextJSON string) (*model.Conversation, error) {
	var c model.Conversation
	err := s.db.Where("visitor_id = ? AND status IN ?", visitorID, []string{"queued", "assigned", "created", "bot_serving"}).
		Order("id DESC").First(&c).Error
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return s.CreateConversation(siteID, visitorID, channel, contextJSON)
}

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
		return false
	}
	for _, o := range list {
		if strings.EqualFold(strings.TrimSpace(o), strings.TrimSpace(origin)) {
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
