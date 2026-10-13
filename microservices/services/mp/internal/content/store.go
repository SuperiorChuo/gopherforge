package content

import (
	"errors"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// Store 内容中台持久层。复用主 store 的 *gorm.DB（同库），迁移由 Migrate 追加。
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// Migrate 建内容中台的表（posts / post_publications / channel_accounts）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Post{}, &PostPublication{}, &ChannelAccount{})
}

func (s *Store) DB() *gorm.DB { return s.db }

func tenantQ(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

func ensureTenant(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

var ErrNotFound = errors.New("not found")

// ---------- Post ----------

type PostFilter struct {
	Platform string
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

func clamp(page, pageSize int) (offset, limit int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return (page - 1) * pageSize, pageSize
}

func (s *Store) ListPosts(tenantID uint64, f PostFilter) ([]Post, int64, error) {
	q := tenantQ(s.db.Model(&Post{}), tenantID)
	if f.Platform != "" {
		q = q.Where("platform = ?", f.Platform)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR summary LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := clamp(f.Page, f.PageSize)
	var list []Post
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (s *Store) GetPost(id, tenantID uint64) (*Post, error) {
	var p Post
	err := tenantQ(s.db, tenantID).First(&p, id).Error
	return &p, err
}

func (s *Store) CreatePost(p *Post) error {
	p.TenantID = ensureTenant(p.TenantID)
	if p.Status == "" {
		p.Status = "draft"
	}
	return s.db.Create(p).Error
}

func (s *Store) UpdatePost(p *Post) error { return s.db.Save(p).Error }

func (s *Store) DeletePost(id, tenantID uint64) error {
	res := tenantQ(s.db, tenantID).Delete(&Post{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- PostPublication ----------

func (s *Store) ListPublications(tenantID, postID uint64) ([]PostPublication, error) {
	return fullread.All[PostPublication](tenantQ(s.db, tenantID).Where("post_id = ?", postID).Order("id DESC"))
}

func (s *Store) CreatePublication(p *PostPublication) error {
	p.TenantID = ensureTenant(p.TenantID)
	if p.Status == "" {
		p.Status = "pending"
	}
	return s.db.Create(p).Error
}

func (s *Store) UpdatePublication(p *PostPublication) error { return s.db.Save(p).Error }

// UpdatePublicationStatusByExternalID settles publications only through a
// channel account owned by the verified tenant and linked platform account.
func (s *Store) UpdatePublicationStatusByExternalID(
	tenantID uint64,
	channel, externalID string,
	accountRefID uint64,
	status string,
) (bool, error) {
	accountIDs := s.db.Model(&ChannelAccount{}).
		Select("id").
		Where("tenant_id = ? AND channel = ? AND ref_id = ?", tenantID, channel, accountRefID)
	res := tenantQ(s.db.Model(&PostPublication{}), tenantID).
		Where("channel = ? AND external_id = ?", channel, externalID).
		Where("account_id IN (?)", accountIDs).
		Update("status", status)
	return res.RowsAffected > 0, res.Error
}

// ---------- ChannelAccount ----------

func (s *Store) ListChannelAccounts(tenantID uint64, channel string) ([]ChannelAccount, error) {
	q := tenantQ(s.db.Model(&ChannelAccount{}), tenantID)
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	return fullread.All[ChannelAccount](q.Order("id DESC"))
}

func (s *Store) GetChannelAccount(id, tenantID uint64) (*ChannelAccount, error) {
	var a ChannelAccount
	err := tenantQ(s.db, tenantID).First(&a, id).Error
	return &a, err
}

func (s *Store) UpsertChannelAccount(a *ChannelAccount) error {
	a.TenantID = ensureTenant(a.TenantID)
	if a.Status == "" {
		a.Status = "active"
	}
	if a.ID == 0 {
		return s.db.Create(a).Error
	}
	a.UpdatedAt = time.Now()
	return s.db.Save(a).Error
}
