package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

var (
	ErrOfficeInvalidFloorSize = errors.New("floor size must be positive")
	ErrOfficeSeatOutOfBounds  = errors.New("seat coordinate outside floor bounds")
	ErrOfficeDuplicateUser    = errors.New("one user can only occupy one seat")
	ErrOfficeTooManySeats     = errors.New("too many seats in one submission")
)

// 单次整层提交的工位上限。拖拽编辑器一次发全层，给个上界防止误发超大载荷。
const maxSeatsPerFloor = 2000

type HermesOfficeService struct {
	dao *dao.HermesOfficeDAO
}

func NewHermesOfficeService(d *dao.HermesOfficeDAO) *HermesOfficeService {
	return &HermesOfficeService{dao: d}
}

// ---------------------------------------------------------------------------
// 楼层
// ---------------------------------------------------------------------------

type FloorInput struct {
	Name  string  `json:"name"`
	Width float64 `json:"width"`
	Depth float64 `json:"depth"`
	Sort  int     `json:"sort"`
}

func (s *HermesOfficeService) ListFloors(ctx context.Context) ([]localmodel.HermesOfficeFloor, error) {
	return s.dao.ListFloors(ctx)
}

func (s *HermesOfficeService) CreateFloor(ctx context.Context, in FloorInput) (*localmodel.HermesOfficeFloor, error) {
	if err := validateFloor(in); err != nil {
		return nil, err
	}
	floor := &localmodel.HermesOfficeFloor{
		Name:  strings.TrimSpace(in.Name),
		Width: in.Width,
		Depth: in.Depth,
		Sort:  in.Sort,
	}
	if err := s.dao.CreateFloor(ctx, floor); err != nil {
		return nil, err
	}
	return floor, nil
}

func (s *HermesOfficeService) UpdateFloor(ctx context.Context, id uint64, in FloorInput) error {
	if err := validateFloor(in); err != nil {
		return err
	}
	return s.dao.UpdateFloor(ctx, id, map[string]any{
		"name":  strings.TrimSpace(in.Name),
		"width": in.Width,
		"depth": in.Depth,
		"sort":  in.Sort,
	})
}

func (s *HermesOfficeService) DeleteFloor(ctx context.Context, id uint64) error {
	return s.dao.DeleteFloor(ctx, id)
}

func validateFloor(in FloorInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("floor name is required")
	}
	if in.Width <= 0 || in.Depth <= 0 {
		return ErrOfficeInvalidFloorSize
	}
	// 上限防止手滑填出一个渲染不动的巨大平面
	if in.Width > 500 || in.Depth > 500 {
		return ErrOfficeInvalidFloorSize
	}
	return nil
}

// ---------------------------------------------------------------------------
// 工位
// ---------------------------------------------------------------------------

// OfficeSeatView 是给前端的扁平结构，与 web/src/api/ai/hermes-office.ts 对齐。
type OfficeSeatView struct {
	Seat localmodel.HermesOfficeSeat `json:"seat"`
	User *OfficeSeatUser             `json:"user"`
	Dev  *OfficeSeatDevice           `json:"device"`
}

type OfficeSeatUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

type OfficeSeatDevice struct {
	DeviceID      string `json:"device_id"`
	ActualState   string `json:"actual_state"`
	HealthStatus  string `json:"health_status"`
	HermesVersion string `json:"hermes_version"`
	LastSeenAt    string `json:"last_seen_at,omitempty"`
}

func (s *HermesOfficeService) ListSeats(ctx context.Context, floorID uint64) ([]OfficeSeatView, error) {
	rows, err := s.dao.ListSeatViews(ctx, floorID)
	if err != nil {
		return nil, err
	}
	out := make([]OfficeSeatView, 0, len(rows))
	for _, r := range rows {
		view := OfficeSeatView{Seat: r.HermesOfficeSeat}
		if r.HermesOfficeSeat.UserID > 0 && r.Username != "" {
			view.User = &OfficeSeatUser{ID: r.HermesOfficeSeat.UserID, Username: r.Username, Nickname: r.Nickname}
		}
		if r.DeviceID != "" {
			dev := &OfficeSeatDevice{
				DeviceID:      r.DeviceID,
				ActualState:   r.ActualState,
				HealthStatus:  r.HealthStatus,
				HermesVersion: r.HermesVersion,
			}
			if r.DeviceSeenAt != nil {
				dev.LastSeenAt = r.DeviceSeenAt.Format("2006-01-02T15:04:05Z07:00")
			}
			view.Dev = dev
		}
		out = append(out, view)
	}
	return out, nil
}

type SeatInput struct {
	ID       uint64  `json:"id"`
	Label    string  `json:"label"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Rotation float64 `json:"rotation"`
	UserID   uint    `json:"user_id"`
}

// ReplaceSeats 校验并整层替换工位布局。
func (s *HermesOfficeService) ReplaceSeats(ctx context.Context, floorID uint64, seats []SeatInput) error {
	if len(seats) > maxSeatsPerFloor {
		return ErrOfficeTooManySeats
	}
	floor, err := s.dao.GetFloor(ctx, floorID)
	if err != nil {
		return err
	}

	seen := make(map[uint]struct{}, len(seats))
	payload := make([]dao.SeatUpsert, 0, len(seats))
	for _, in := range seats {
		// 坐标必须落在楼层平面内，否则工位会被拖到地板外面
		if in.X < 0 || in.Y < 0 || in.X > floor.Width || in.Y > floor.Depth {
			return ErrOfficeSeatOutOfBounds
		}
		// 一人一工位：DB 有唯一索引兜底，这里先给出可读错误
		if in.UserID > 0 {
			if _, dup := seen[in.UserID]; dup {
				return ErrOfficeDuplicateUser
			}
			seen[in.UserID] = struct{}{}
		}
		payload = append(payload, dao.SeatUpsert{
			ID:       in.ID,
			Label:    strings.TrimSpace(in.Label),
			X:        in.X,
			Y:        in.Y,
			Rotation: normalizeRotation(in.Rotation),
			UserID:   in.UserID,
		})
	}
	return s.dao.ReplaceSeats(ctx, floorID, payload)
}

func (s *HermesOfficeService) DeleteSeat(ctx context.Context, seatID uint64) error {
	return s.dao.DeleteSeat(ctx, seatID)
}

// normalizeRotation 把任意角度折进 [0,360)，对齐 DB 的 CHECK 约束。
func normalizeRotation(deg float64) float64 {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return 0
	}
	r := math.Mod(deg, 360)
	if r < 0 {
		r += 360
	}
	return r
}
