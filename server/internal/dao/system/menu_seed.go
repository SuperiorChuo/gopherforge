package system

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

// MenuSeedDAO owns default menu bootstrap persistence.
type MenuSeedDAO struct {
	db *gorm.DB
}

func NewMenuSeedDAO(db *gorm.DB) *MenuSeedDAO {
	return &MenuSeedDAO{db: db}
}

// Deprecated: use BootstrapDefaultMenusContext instead.
func (d *MenuSeedDAO) BootstrapDefaultMenus(seed []model.Menu, now time.Time) (int, error) {
	return d.BootstrapDefaultMenusContext(context.Background(), seed, now)
}

func (d *MenuSeedDAO) BootstrapDefaultMenusContext(ctx context.Context, seed []model.Menu, now time.Time) (int, error) {
	db := d.baseDB()
	if db == nil {
		return 0, errors.New("database is not initialized")
	}

	created := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Menu{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}

		for _, item := range seed {
			item.CreatedAt = now
			item.UpdatedAt = now
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			created++
		}
		return nil
	})
	return created, err
}

func (d *MenuSeedDAO) baseDB() *gorm.DB {
	if d != nil && d.db != nil {
		return d.db
	}
	return database.DB
}
