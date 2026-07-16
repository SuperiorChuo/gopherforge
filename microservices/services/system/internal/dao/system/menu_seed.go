package system

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

// MenuSeedDAO owns default menu bootstrap persistence.
type MenuSeedDAO struct {
	db *gorm.DB
}

func NewMenuSeedDAO(db *gorm.DB) *MenuSeedDAO {
	return &MenuSeedDAO{db: db}
}

func (d *MenuSeedDAO) BootstrapDefaultMenusContext(ctx context.Context, seed []model.Menu, now time.Time) (int, error) {
	db := d.baseDB()
	if db == nil {
		return 0, errors.New("database is not initialized")
	}

	created := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 按 ID 补插缺失的种子菜单：存量库升级时也能拿到新增菜单，
		// 同时不覆盖管理员对已有菜单的修改。
		var existingIDs []uint
		if err := tx.Model(&model.Menu{}).Pluck("id", &existingIDs).Error; err != nil {
			return err
		}
		existing := make(map[uint]bool, len(existingIDs))
		for _, id := range existingIDs {
			existing[id] = true
		}

		for _, item := range seed {
			if existing[item.ID] {
				continue
			}
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
	return d.db
}
