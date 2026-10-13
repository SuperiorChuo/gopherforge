package store

import (
	"context"
	"errors"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
)

func (s *Store) GetSiteByAppKey(appKey string) (*model.Site, error) {
	var site model.Site
	err := s.db.Where("app_key = ? AND status = 1", appKey).First(&site).Error
	if err != nil {
		return nil, err
	}
	return &site, nil
}

func (s *Store) ListSites(tenantID uint64) ([]model.Site, error) {
	var list []model.Site
	q := s.db.Order("id ASC")
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Limit(adminListLimit).Find(&list).Error
	return list, err
}

func (s *Store) GetSite(id uint64) (*model.Site, error) {
	var site model.Site
	if err := s.db.First(&site, id).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

// GetSiteForTenant 仅在站点属于 tenantID 时返回。
func (s *Store) GetSiteForTenant(id, tenantID uint64) (*model.Site, error) {
	var site model.Site
	if err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&site).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

type SiteCreate struct {
	Name                string
	WelcomeText         string
	AllowedOrigins      string // JSON 数组字符串
	DefaultSkillGroupID *uint64
	BotEnabled          *bool // nil = 默认开启
	BotKBEnabled        *bool // nil = 默认开启知识库
	BotSystemPrompt     string
}

// CreateSite 用生成的 app_key/app_secret 创建租户站点。
func (s *Store) CreateSite(ctx context.Context, tenantID uint64, in SiteCreate) (*model.Site, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if in.DefaultSkillGroupID != nil && *in.DefaultSkillGroupID > 0 {
		if _, err := s.GetSkillGroupForTenant(*in.DefaultSkillGroupID, tenantID); err != nil {
			return nil, errors.New("default skill group not found in tenant")
		}
	} else {
		in.DefaultSkillGroupID = nil
	}
	botEnabled := true
	if in.BotEnabled != nil {
		botEnabled = *in.BotEnabled
	}
	botKB := true
	if in.BotKBEnabled != nil {
		botKB = *in.BotKBEnabled
	}
	site := model.Site{
		TenantID: tenantID,
		// uuid 去连字符：app_key 进埋码脚本 URL，短一点；secret 仅服务端可见
		AppKey:              strings.ReplaceAll(uuid.NewString(), "-", ""),
		AppSecret:           strings.ReplaceAll(uuid.NewString(), "-", "") + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Name:                name,
		WelcomeText:         in.WelcomeText,
		AllowedOrigins:      in.AllowedOrigins,
		Status:              1,
		BotEnabled:          botEnabled,
		BotKBEnabled:        botKB,
		BotSystemPrompt:     in.BotSystemPrompt,
		DefaultSkillGroupID: in.DefaultSkillGroupID,
	}
	if err := s.db.WithContext(ctx).Create(&site).Error; err != nil {
		return nil, err
	}
	// gorm 把 false 当零值跳过、落成列默认 true，插入后补一笔显式更新
	if !botEnabled {
		if err := s.db.WithContext(ctx).Model(&site).Update("bot_enabled", false).Error; err != nil {
			return nil, err
		}
	}
	if !botKB {
		if err := s.db.WithContext(ctx).Model(&site).Update("bot_kb_enabled", false).Error; err != nil {
			return nil, err
		}
	}
	return &site, nil
}

type SiteUpdate struct {
	Name                *string
	WelcomeText         *string
	AllowedOrigins      *string
	Status              *int16
	DefaultSkillGroupID *uint64
	BotEnabled          *bool
	BotKBEnabled        *bool
	BotSystemPrompt     *string
	BusinessHours       *string
}

func (s *Store) UpdateSite(ctx context.Context, id, tenantID uint64, u SiteUpdate) (*model.Site, error) {
	site, err := s.GetSiteForTenant(id, tenantID)
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
			// 技能组必须属于同一租户
			if _, err := s.GetSkillGroupForTenant(*u.DefaultSkillGroupID, tenantID); err != nil {
				return nil, errors.New("default skill group not found in tenant")
			}
			site.DefaultSkillGroupID = u.DefaultSkillGroupID
		}
	}
	if u.BotEnabled != nil {
		site.BotEnabled = *u.BotEnabled
	}
	if u.BotKBEnabled != nil {
		site.BotKBEnabled = *u.BotKBEnabled
	}
	if u.BotSystemPrompt != nil {
		site.BotSystemPrompt = *u.BotSystemPrompt
	}
	if u.BusinessHours != nil {
		site.BusinessHours = *u.BusinessHours
	}
	if err := s.db.WithContext(ctx).Save(site).Error; err != nil {
		return nil, err
	}
	// false 零值 Save 可能被跳过，显式补写
	if u.BotKBEnabled != nil && !*u.BotKBEnabled {
		_ = s.db.WithContext(ctx).Model(site).Update("bot_kb_enabled", false).Error
	}
	if u.BotEnabled != nil && !*u.BotEnabled {
		_ = s.db.WithContext(ctx).Model(site).Update("bot_enabled", false).Error
	}
	return site, nil
}

func (s *Store) ListQuickReplies(tenantID uint64) ([]model.QuickReply, error) {
	var list []model.QuickReply
	err := s.db.Where("tenant_id = ?", tenantID).Order("sort ASC, id ASC").Limit(adminListLimit).Find(&list).Error
	return list, err
}

func (s *Store) CreateQuickReply(ctx context.Context, tenantID, createdBy uint64, title, content string) (*model.QuickReply, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return nil, errors.New("title and content are required")
	}
	qr := model.QuickReply{TenantID: tenantID, Title: title, Content: content, CreatedBy: createdBy}
	if err := s.db.WithContext(ctx).Create(&qr).Error; err != nil {
		return nil, err
	}
	return &qr, nil
}

func (s *Store) UpdateQuickReply(ctx context.Context, id, tenantID uint64, title, content *string, sort *int) (*model.QuickReply, error) {
	var qr model.QuickReply
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&qr).Error; err != nil {
		return nil, err
	}
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return nil, errors.New("title is required")
		}
		qr.Title = strings.TrimSpace(*title)
	}
	if content != nil {
		if strings.TrimSpace(*content) == "" {
			return nil, errors.New("content is required")
		}
		qr.Content = strings.TrimSpace(*content)
	}
	if sort != nil {
		qr.Sort = *sort
	}
	if err := s.db.WithContext(ctx).Save(&qr).Error; err != nil {
		return nil, err
	}
	return &qr, nil
}

func (s *Store) DeleteQuickReply(ctx context.Context, id, tenantID uint64) error {
	res := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&model.QuickReply{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
