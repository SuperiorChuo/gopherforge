package dao

import (
	"context"
	"errors"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrOfficeFloorNotFound = errors.New("hermes office floor not found")
	ErrOfficeSeatUserTaken = errors.New("hermes office seat user already assigned")
)

// HermesOfficeSeatView 是工位在地图上的完整视图：布局 + 绑定员工 + 该员工的
// Hermes 设备状态。一次 JOIN 拿全，避免前端按工位逐个查设备。
type HermesOfficeSeatView struct {
	localmodel.HermesOfficeSeat

	Username string `gorm:"column:username" json:"-"`
	Nickname string `gorm:"column:nickname" json:"-"`

	DeviceID      string     `gorm:"column:device_id" json:"-"`
	ActualState   string     `gorm:"column:actual_state" json:"-"`
	HealthStatus  string     `gorm:"column:health_status" json:"-"`
	HermesVersion string     `gorm:"column:hermes_version" json:"-"`
	DeviceSeenAt  *time.Time `gorm:"column:device_seen_at" json:"-"`
}

type HermesOfficeDAO struct {
	db       *gorm.DB
	idClient *identityclient.Client
}

func NewHermesOfficeDAO(db *gorm.DB) *HermesOfficeDAO {
	return &HermesOfficeDAO{db: db}
}

// SetIdentity 注入 identity 客户端（工位显示名，禁止 JOIN users）。
func (d *HermesOfficeDAO) SetIdentity(c *identityclient.Client) {
	d.idClient = c
}

// ---------------------------------------------------------------------------
// 楼层
// ---------------------------------------------------------------------------

func (d *HermesOfficeDAO) ListFloors(ctx context.Context) ([]localmodel.HermesOfficeFloor, error) {
	return fullread.All[localmodel.HermesOfficeFloor](d.db.WithContext(ctx).
		Model(&localmodel.HermesOfficeFloor{}).Order("sort ASC, id ASC"))
}

func (d *HermesOfficeDAO) GetFloor(ctx context.Context, id uint64) (*localmodel.HermesOfficeFloor, error) {
	var floor localmodel.HermesOfficeFloor
	err := d.db.WithContext(ctx).Model(&localmodel.HermesOfficeFloor{}).
		Where("id = ?", id).First(&floor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOfficeFloorNotFound
	}
	if err != nil {
		return nil, err
	}
	return &floor, nil
}

func (d *HermesOfficeDAO) CreateFloor(ctx context.Context, floor *localmodel.HermesOfficeFloor) error {
	return d.db.WithContext(ctx).Create(floor).Error
}

func (d *HermesOfficeDAO) UpdateFloor(ctx context.Context, id uint64, fields map[string]any) error {
	res := d.db.WithContext(ctx).Model(&localmodel.HermesOfficeFloor{}).
		Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrOfficeFloorNotFound
	}
	return nil
}

// DeleteFloor 连同该层工位一起删除，避免留下孤儿工位。
func (d *HermesOfficeDAO) DeleteFloor(ctx context.Context, id uint64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("floor_id = ?", id).Delete(&localmodel.HermesOfficeSeat{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&localmodel.HermesOfficeFloor{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrOfficeFloorNotFound
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// 工位
// ---------------------------------------------------------------------------

// ListSeatViews 返回某层的全部工位及其绑定员工的设备状态。
//
// 租户隔离注意：tenant 插件只会给主表（hermes_office_seats）自动加 tenant_id，
// JOIN 进来的 hermes_devices 不在其管辖内，因此显式把 tenant_id 写进设备子查询。
// 用户显示名不 JOIN public.users，经 identity owner API 批量补全（B2）。
func (d *HermesOfficeDAO) ListSeatViews(ctx context.Context, floorID uint64) ([]HermesOfficeSeatView, error) {
	tenantID := tenant.FromContextOrDefault(ctx)

	// 每个用户可能有多台设备，取最近心跳的那台未吊销设备作为代表。
	latestDevice := d.db.
		Select("DISTINCT ON (user_id) user_id, device_id, actual_state, health_status, hermes_version, last_seen_at").
		Table("hermes_devices").
		Where("tenant_id = ? AND revoked_at IS NULL", tenantID).
		Order("user_id, last_seen_at DESC NULLS LAST, id DESC")

	views, err := fullread.All[HermesOfficeSeatView](d.db.WithContext(ctx).Model(&localmodel.HermesOfficeSeat{}).
		Select(`hermes_office_seats.*,
			dev.device_id AS device_id,
			dev.actual_state AS actual_state,
			dev.health_status AS health_status,
			dev.hermes_version AS hermes_version,
			dev.last_seen_at AS device_seen_at`).
		Joins("LEFT JOIN (?) AS dev ON dev.user_id = hermes_office_seats.user_id", latestDevice).
		Where("hermes_office_seats.floor_id = ?", floorID).
		Order("hermes_office_seats.id ASC"))
	if err != nil {
		return nil, err
	}
	d.fillSeatNames(ctx, views)
	return views, nil
}

func (d *HermesOfficeDAO) fillSeatNames(ctx context.Context, views []HermesOfficeSeatView) {
	if d.idClient == nil || len(views) == 0 {
		return
	}
	ids := make([]uint64, 0, len(views))
	seen := map[uint64]struct{}{}
	for i := range views {
		uid := uint64(views[i].UserID)
		if uid == 0 {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		ids = append(ids, uid)
	}
	if len(ids) == 0 {
		return
	}
	names, err := d.idClient.BatchUserDisplayNames(ctx, ids)
	if err != nil || len(names) == 0 {
		return
	}
	for i := range views {
		uid := uint64(views[i].UserID)
		if n, ok := names[uid]; ok && n != "" {
			views[i].Username = n
			views[i].Nickname = n
		}
	}
}

type SeatUpsert struct {
	ID       uint64
	Label    string
	X        float64
	Y        float64
	Rotation float64
	UserID   uint
}

// ReplaceSeats 整层替换：拖拽编辑器一次提交全层布局。
// 传入列表里没有的既有工位会被删除，有 ID 的更新，无 ID 的新建。
func (d *HermesOfficeDAO) ReplaceSeats(ctx context.Context, floorID uint64, seats []SeatUpsert) error {
	tenantID := tenant.FromContextOrDefault(ctx)

	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepIDs := make([]uint64, 0, len(seats))
		for _, s := range seats {
			if s.ID > 0 {
				keepIDs = append(keepIDs, s.ID)
			}
		}

		del := tx.Where("floor_id = ?", floorID)
		if len(keepIDs) > 0 {
			del = del.Where("id NOT IN ?", keepIDs)
		}
		if err := del.Delete(&localmodel.HermesOfficeSeat{}).Error; err != nil {
			return err
		}

		for _, s := range seats {
			if s.ID > 0 {
				res := tx.Model(&localmodel.HermesOfficeSeat{}).
					Where("id = ? AND floor_id = ?", s.ID, floorID).
					Updates(map[string]any{
						"label":      s.Label,
						"x":          s.X,
						"y":          s.Y,
						"rotation":   s.Rotation,
						"user_id":    s.UserID,
						"updated_at": time.Now(),
					})
				if res.Error != nil {
					return res.Error
				}
				continue
			}
			row := localmodel.HermesOfficeSeat{
				TenantID: tenantID,
				FloorID:  floorID,
				Label:    s.Label,
				X:        s.X,
				Y:        s.Y,
				Rotation: s.Rotation,
				UserID:   s.UserID,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *HermesOfficeDAO) DeleteSeat(ctx context.Context, seatID uint64) error {
	return d.db.WithContext(ctx).Where("id = ?", seatID).
		Delete(&localmodel.HermesOfficeSeat{}).Error
}
