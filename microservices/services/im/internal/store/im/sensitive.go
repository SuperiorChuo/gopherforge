package store

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// NormalizeSensitiveWord 对单个词做修剪与校验；空结果表示跳过。
func NormalizeSensitiveWord(w string) (string, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return "", nil
	}
	if utf8.RuneCountInString(w) < 2 {
		return "", errors.New("敏感词至少 2 个字符: " + w)
	}
	if len(w) > 128 {
		return "", errors.New("敏感词过长(≤128字节): " + w)
	}
	return w, nil
}

type SensitiveWordFilter struct {
	Keyword string
	Action  string
	Status  *int16
	Tag     string
	Offset  int
	Limit   int
}

func (s *Store) ListSensitiveWords(tenantID uint64, f SensitiveWordFilter) ([]model.SensitiveWord, int64, error) {
	q := s.db.Model(&model.SensitiveWord{}).Where("tenant_id = ?", tenantID)
	if f.Keyword != "" {
		q = q.Where("word LIKE ?", "%"+f.Keyword+"%")
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Tag != "" {
		q = q.Where("tag = ?", f.Tag)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var list []model.SensitiveWord
	err := q.Order("id DESC").Offset(f.Offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// CreateSensitiveWords 批量插入敏感词（去重去空）；已存在的词跳过。
func (s *Store) CreateSensitiveWords(ctx context.Context, tenantID, createdBy uint64, words []string, action, tag string) (created int, skipped int, err error) {
	if action != "block" {
		action = "mask"
	}
	seen := map[string]bool{}
	rows := make([]model.SensitiveWord, 0, len(words))
	for _, w := range words {
		w, nerr := NormalizeSensitiveWord(w)
		if nerr != nil {
			return 0, 0, nerr
		}
		if w == "" || seen[strings.ToLower(w)] {
			continue
		}
		seen[strings.ToLower(w)] = true
		rows = append(rows, model.SensitiveWord{
			TenantID: tenantID, Word: w, Action: action, Tag: strings.TrimSpace(tag),
			Status: 1, CreatedBy: createdBy,
		})
	}
	if len(rows) == 0 {
		return 0, 0, errors.New("没有有效的敏感词")
	}
	// audittrail 插件拒绝 ON CONFLICT 批量形状（MaxRows=1），改逐行 Create；
	// 唯一约束 (tenant_id, word) 冲突即视为跳过，语义与批量 DoNothing 一致。
	createdN := 0
	for i := range rows {
		if err := s.db.WithContext(ctx).Create(&rows[i]).Error; err != nil {
			if !isUniqueViolation(err) {
				return createdN, len(rows) - createdN, err
			}
			continue // 已存在 → skipped
		}
		createdN++
	}
	return createdN, len(rows) - createdN, nil
}

type SensitiveWordUpdate struct {
	Word   *string
	Action *string
	Tag    *string
	Status *int16
}

func (s *Store) UpdateSensitiveWord(ctx context.Context, tenantID, id uint64, u SensitiveWordUpdate) (*model.SensitiveWord, error) {
	var row model.SensitiveWord
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if u.Word != nil {
		w, err := NormalizeSensitiveWord(*u.Word)
		if err != nil {
			return nil, err
		}
		if w == "" {
			return nil, errors.New("敏感词不能为空")
		}
		updates["word"] = w
	}
	if u.Action != nil {
		if *u.Action != "mask" && *u.Action != "block" {
			return nil, errors.New("action 只能是 mask 或 block")
		}
		updates["action"] = *u.Action
	}
	if u.Tag != nil {
		updates["tag"] = strings.TrimSpace(*u.Tag)
	}
	if u.Status != nil {
		updates["status"] = *u.Status
	}
	if len(updates) == 0 {
		return &row, nil
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return nil, err
	}
	// 重查拿更新后的 updated_at（gorm Updates 不回写时间戳到模型）。
	var updated model.SensitiveWord
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&updated).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Store) DeleteSensitiveWord(ctx context.Context, tenantID, id uint64) error {
	res := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.SensitiveWord{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("敏感词不存在")
	}
	return nil
}

// ListEnabledSensitiveWords 为匹配器缓存（sensitive.WordSource）提供数据。
func (s *Store) ListEnabledSensitiveWords(tenantID uint64) ([]model.SensitiveWord, error) {
	// Never truncate the matcher dictionary or turn a large valid dictionary into an error.
	return fullread.All[model.SensitiveWord](s.db.Where("tenant_id = ? AND status = 1", tenantID))
}

// isUniqueViolation 在驱动未把错误翻译成 gorm.ErrDuplicatedKey 时退化为文本匹配
// （同 cc/ai 服务先例）：Postgres 报 "duplicate key value"，sqlite 报
// "UNIQUE constraint failed"，两者都认。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}
