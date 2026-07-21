package system

import (
	"context"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
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

func (d *PostDAO) GetByIDContext(ctx context.Context, id uint) (*model.Post, error) {
	var post model.Post
	result := d.dbWithContext(ctx).First(&post, id)
	return &post, result.Error
}

func (d *PostDAO) GetByCodeContext(ctx context.Context, code string) (*model.Post, error) {
	var post model.Post
	q := d.dbWithContext(ctx).Where("code = ?", code)
	if tid, ok := ctx.Value("tenant_id").(uint); ok && tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	result := q.First(&post)
	return &post, result.Error
}

func (d *PostDAO) GetListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Post, int64, error) {
	var posts []model.Post
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Post{})
	if tid, ok := ctx.Value("tenant_id").(uint); ok && tid > 0 {
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

func (d *PostDAO) GetAllContext(ctx context.Context, status *int8) ([]model.Post, error) {
	var posts []model.Post
	query := d.dbWithContext(ctx).Model(&model.Post{})
	if tid, ok := ctx.Value("tenant_id").(uint); ok && tid > 0 {
		query = query.Where("sys_posts.tenant_id = ?", tid)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("sort ASC, created_at ASC").Find(&posts)
	return posts, result.Error
}

func (d *PostDAO) CreateContext(ctx context.Context, post *model.Post) error {
	return d.dbWithContext(ctx).Create(post).Error
}

func (d *PostDAO) UpdateContext(ctx context.Context, post *model.Post) error {
	return d.dbWithContext(ctx).Save(post).Error
}

// DeleteContext removes a post unless users are still assigned to it.
func (d *PostDAO) DeleteContext(ctx context.Context, id uint) error {
	db := d.dbWithContext(ctx)

	var count int64
	if err := db.Model(&model.UserPost{}).Where("post_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPostHasUsers
	}

	return db.Delete(&model.Post{}, id).Error
}

type postError string

func (e postError) Error() string { return string(e) }

const (
	ErrPostHasUsers postError = "post still has assigned users"
)
