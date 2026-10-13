package system

import (
	"context"
	"errors"
	"fmt"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// MenuSeedDAO 负责默认菜单播种的持久化。
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
		// Serialize bootstrap replicas and concurrent menu writes, without blocking reads.
		if err := tx.Exec("LOCK TABLE menus IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		var existing []model.Menu
		const maxMenus = 10000
		if err := tx.Select("id", "name", "path").Limit(maxMenus + 1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > maxMenus {
			return errors.New("menu bootstrap inventory exceeds limit")
		}

		// Seed IDs describe relationships, not persisted identities. Public releases
		// and older installations can have the same menu under different IDs.
		byID := make(map[uint]model.Menu, len(seed))
		resolved := make(map[uint]uint, len(seed))
		claimed := make(map[uint]model.Menu, len(seed))
		for _, item := range seed {
			if _, duplicate := byID[item.ID]; item.ID == 0 || duplicate {
				return fmt.Errorf("invalid or duplicate menu seed ID %d", item.ID)
			}
			byID[item.ID] = item
			id, err := existingMenuIdentity(item, existing)
			if err != nil {
				return err
			}
			if id != 0 {
				// Per-seed uniqueness is insufficient: an administrator rename can
				// make one row match another seed's name and this seed's path.
				// Reject this before inserts or nontransactional sequence changes.
				if previous, duplicate := claimed[id]; duplicate {
					return fmt.Errorf("ambiguous menu identity: seeds %d (%q, %q) and %d (%q, %q) share persisted ID %d; reconcile menus before bootstrap",
						previous.ID, previous.Name, previous.Path, item.ID, item.Name, item.Path, id)
				}
				claimed[id] = item
				resolved[item.ID] = id
			}
		}

		visiting := make(map[uint]bool, len(seed))
		sequenceReady := false
		var insert func(uint) (uint, error)
		insert = func(seedID uint) (uint, error) {
			if id, ok := resolved[seedID]; ok {
				return id, nil // Do not rewrite administrator edits or existing associations.
			}
			item, ok := byID[seedID]
			if !ok {
				return 0, fmt.Errorf("missing parent menu seed %d", seedID)
			}
			if visiting[seedID] {
				return 0, fmt.Errorf("cyclic menu seed %d", seedID)
			}
			visiting[seedID] = true
			if item.ParentID != 0 {
				parentID, err := insert(item.ParentID)
				if err != nil {
					return 0, err
				}
				item.ParentID = parentID
			}
			if !sequenceReady {
				// Old seeders used explicit IDs. Advance past live rows, but never
				// rewind consumed/deleted IDs or change an unused empty sequence.
				if err := tx.Exec(`SELECT setval(pg_get_serial_sequence('menus', 'id'),
					GREATEST(MAX(id), pg_sequence_last_value(pg_get_serial_sequence('menus', 'id')::regclass)))
					FROM menus HAVING MAX(id) IS NOT NULL`).Error; err != nil {
					return 0, err
				}
				sequenceReady = true
			}
			item.ID = 0 // Allocate through the database; never steal a custom menu ID.
			item.CreatedAt = now
			item.UpdatedAt = now
			if err := tx.Create(&item).Error; err != nil {
				return 0, err
			}
			resolved[seedID] = item.ID
			created++
			return item.ID, nil
		}
		for _, item := range seed {
			if _, err := insert(item.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

func existingMenuIdentity(item model.Menu, existing []model.Menu) (uint, error) {
	match := func(exact bool) (uint, error) {
		var matches []uint
		for _, current := range existing {
			name := item.Name != "" && item.Name == current.Name
			path := item.Path != "" && item.Path == current.Path
			if (exact && !(name && path)) || (!exact && !(name || path)) {
				continue
			}
			matches = append(matches, current.ID)
		}
		if len(matches) == 0 {
			return 0, nil
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		// Public migration 000005 inserted setting=22, while the released Go
		// seed inserted setting=23. Recognize ONLY that exact historical pair,
		// retain both rows/associations, and use the public Go identity for any
		// new children. A third row or any different ID still fails closed.
		if exact && item.Name == "setting" && item.Path == "/system/setting" && len(matches) == 2 &&
			((matches[0] == 22 && matches[1] == 23) || (matches[0] == 23 && matches[1] == 22)) {
			return 23, nil
		}
		return 0, fmt.Errorf("ambiguous menu identity %q (%d and %d); reconcile menus before bootstrap", item.Name, matches[0], matches[1])
	}
	// Historical SQL and the old ID-only seeder can leave Tenant and tenant at
	// the same path. A unique full identity is authoritative; retain all other
	// rows and associations, rather than deleting legacy data or picking an ID.
	if id, err := match(true); id != 0 || err != nil {
		return id, err
	}
	// Allow administrator renames/path edits only when the remaining identity
	// resolves uniquely. Unknown split identities/duplicate pairs fail closed.
	return match(false)
}

func (d *MenuSeedDAO) baseDB() *gorm.DB {
	return d.db
}
