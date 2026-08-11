package system

import (
	"context"

	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// PostDAO persists job position records.
type PostDAO struct {
	db *gorm.DB
}

func NewPostDAO(db *gorm.DB) *PostDAO {
	return &PostDAO{db: db}
}

func (d *PostDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *PostDAO) GetByIDContext(ctx context.Context, id uint) (*localmodel.Post, error) {
	var post localmodel.Post
	result := d.dbWithContext(ctx).First(&post, id)
	return &post, result.Error
}

func (d *PostDAO) GetByCodeContext(ctx context.Context, code string) (*localmodel.Post, error) {
	var post localmodel.Post
	q := d.dbWithContext(ctx).Where("code = ?", code)
	if tid := tenant.FromContext(ctx); tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	result := q.First(&post)
	return &post, result.Error
}

func (d *PostDAO) GetListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]localmodel.Post, int64, error) {
	var posts []localmodel.Post
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.Post{})
	if tid := tenant.FromContext(ctx); tid > 0 {
		query = query.Where("sys_posts.tenant_id = ?", tid)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("sort ASC, created_at ASC").
		Find(&posts)

	return posts, total, result.Error
}

func (d *PostDAO) GetAllContext(ctx context.Context, status *int8) ([]localmodel.Post, error) {
	var posts []localmodel.Post
	query := d.dbWithContext(ctx).Model(&localmodel.Post{})
	if tid := tenant.FromContext(ctx); tid > 0 {
		query = query.Where("sys_posts.tenant_id = ?", tid)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("sort ASC, created_at ASC").Find(&posts)
	return posts, result.Error
}

func (d *PostDAO) CreateContext(ctx context.Context, post *localmodel.Post) error {
	return d.dbWithContext(ctx).Create(post).Error
}

func (d *PostDAO) UpdateContext(ctx context.Context, post *localmodel.Post) error {
	return d.dbWithContext(ctx).Save(post).Error
}

// DeleteContext removes a post unless users are still assigned to it.
func (d *PostDAO) DeleteContext(ctx context.Context, id uint) error {
	db := d.dbWithContext(ctx)

	var count int64
	if err := db.Model(&localmodel.UserPost{}).Where("post_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPostHasUsers
	}

	return db.Delete(&localmodel.Post{}, id).Error
}

type postError string

func (e postError) Error() string { return string(e) }

const (
	ErrPostHasUsers postError = "post still has assigned users"
)
