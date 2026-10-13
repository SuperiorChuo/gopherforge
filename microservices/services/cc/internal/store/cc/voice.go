package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CCVoiceNotify TTS 语音通知任务。
// 状态：pending → running → queued → answered → completed|noanswer；denied/failed 为入队/媒体失败终态。
type CCVoiceNotify struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	TenantID    uint64     `gorm:"not null;index" json:"tenant_id"`
	To          string     `gorm:"size:32;not null" json:"to"`
	Text        string     `gorm:"size:512;not null" json:"text"`
	Gateway     string     `gorm:"size:64" json:"gateway,omitempty"`
	PromptFile  string     `gorm:"size:128" json:"prompt_file,omitempty"`
	Status      string     `gorm:"size:16;not null;index" json:"status"`
	Error       string     `gorm:"size:512" json:"error,omitempty"`
	Reply       string     `gorm:"size:256" json:"reply,omitempty"`
	CallID      string     `gorm:"size:64;index" json:"call_id,omitempty"`
	HangupClass string     `gorm:"size:16" json:"hangup_class,omitempty"`
	OperatorID  uint64     `json:"operator_id,omitempty"`
	RefType     string     `gorm:"size:32;index" json:"ref_type,omitempty"`
	RefID       string     `gorm:"size:64;index" json:"ref_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func (CCVoiceNotify) TableName() string { return "cc_voice_notifies" }

func (s *Store) CreateVoiceNotify(ctx context.Context, j *CCVoiceNotify) error {
	if j == nil {
		return errors.New("job required")
	}
	j.TenantID = EnsureTenant(j.TenantID)
	j.To = strings.TrimSpace(j.To)
	j.Text = strings.TrimSpace(j.Text)
	j.Status = "pending"
	return s.db.WithContext(ctx).Create(j).Error
}

func (s *Store) GetVoiceNotify(ctx context.Context, tenantID, id uint64) (*CCVoiceNotify, error) {
	var j CCVoiceNotify
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", EnsureTenant(tenantID), id).First(&j).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) ListVoiceNotifies(ctx context.Context, tenantID uint64, page, pageSize int) ([]CCVoiceNotify, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	db := s.db.WithContext(ctx).Model(&CCVoiceNotify{}).Where("tenant_id = ?", EnsureTenant(tenantID))
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []CCVoiceNotify
	err := db.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (s *Store) ClaimNextVoiceNotify(ctx context.Context) (*CCVoiceNotify, error) {
	var j CCVoiceNotify
	err := s.db.WithContext(ctx).Where("status = ?", "pending").Order("id ASC").First(&j).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&CCVoiceNotify{}).
		Where("id = ? AND status = ?", j.ID, "pending").
		Updates(map[string]any{"status": "running", "started_at": now, "updated_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	j.Status = "running"
	j.StartedAt = &now
	return &j, nil
}

func (s *Store) FinishVoiceNotify(ctx context.Context, id uint64, status, prompt, reply, errMsg string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&CCVoiceNotify{}).Where("id = ?", id).Updates(map[string]any{
		"status":      status,
		"prompt_file": prompt,
		"reply":       reply,
		"error":       errMsg,
		"finished_at": now,
		"updated_at":  now,
	}).Error
}

var voiceTerminal = map[string]bool{
	"denied": true, "failed": true, "completed": true, "noanswer": true,
}

// ApplyVoiceHangup 把挂机结果写回任务。已终态（含 denied/failed）不回退。
// answered=true 或 durationSec>0 视为接通完成，否则未接通。
func (s *Store) ApplyVoiceHangup(ctx context.Context, id uint64, callID, cause string, durationSec int, answered bool) (bool, error) {
	var j CCVoiceNotify
	if err := s.db.WithContext(ctx).First(&j, id).Error; err != nil {
		return false, err
	}
	if voiceTerminal[j.Status] {
		return false, nil
	}
	status := "noanswer"
	if answered || durationSec > 0 {
		status = "completed"
	}
	now := time.Now()
	updates := map[string]any{
		"status":      status,
		"error":       strings.TrimSpace(cause),
		"finished_at": now,
		"updated_at":  now,
	}
	if j.HangupClass == "" {
		updates["hangup_class"] = ClassifyHangup(cause)
	}
	if callID != "" {
		updates["call_id"] = callID
	}
	res := s.db.WithContext(ctx).Model(&CCVoiceNotify{}).Where("id = ? AND status = ?", id, j.Status).Updates(updates)
	return res.RowsAffected > 0, res.Error
}

func (s *Store) SetVoiceNotifyHangupClass(ctx context.Context, id uint64, class string) error {
	if id == 0 || class == "" {
		return nil
	}
	return s.db.WithContext(ctx).Model(&CCVoiceNotify{}).
		Where("id = ?", id).Update("hangup_class", class).Error
}

func (s *Store) GetVoiceNotifyByCallID(ctx context.Context, callID string) (*CCVoiceNotify, error) {
	if strings.TrimSpace(callID) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var j CCVoiceNotify
	err := s.db.WithContext(ctx).Where("call_id = ?", callID).First(&j).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) ApplyVoiceAnswered(ctx context.Context, id uint64, callID string) (bool, error) {
	var j CCVoiceNotify
	if err := s.db.WithContext(ctx).First(&j, id).Error; err != nil {
		return false, err
	}
	if voiceTerminal[j.Status] || j.Status == "answered" {
		return false, nil
	}
	now := time.Now()
	updates := map[string]any{"status": "answered", "updated_at": now}
	if callID != "" {
		updates["call_id"] = callID
	}
	res := s.db.WithContext(ctx).Model(&CCVoiceNotify{}).Where("id = ? AND status = ?", id, j.Status).Updates(updates)
	return res.RowsAffected > 0, res.Error
}

func (s *Store) RequeueStaleVoiceNotifies(ctx context.Context, staleAfter time.Duration) (int64, error) {
	cut := time.Now().Add(-staleAfter)
	res := s.db.WithContext(ctx).Model(&CCVoiceNotify{}).
		Where("status = ? AND started_at < ?", "running", cut).
		Updates(map[string]any{"status": "pending", "updated_at": time.Now()})
	return res.RowsAffected, res.Error
}
