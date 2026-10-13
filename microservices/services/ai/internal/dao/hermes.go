package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrHermesAuthorizationPending = errors.New("hermes authorization pending")
	ErrHermesAuthorizationDenied  = errors.New("hermes authorization denied")
	ErrHermesAuthorizationExpired = errors.New("hermes authorization expired")
	ErrHermesEntitlementDisabled  = errors.New("hermes entitlement disabled")
	ErrHermesDeviceLimitReached   = errors.New("hermes device limit reached")
	ErrHermesDeviceRevoked        = errors.New("hermes device revoked")
)

type HermesEntitlementView struct {
	localmodel.HermesEntitlement
	Username          string     `json:"username"`
	Nickname          string     `json:"nickname"`
	ActiveDeviceCount int64      `json:"active_device_count"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
}

type HermesDAO struct {
	db       *gorm.DB
	idClient *identityclient.Client // 可选：显示名/存在性走 identity owner API
}

func NewHermesDAO(db *gorm.DB) *HermesDAO {
	return &HermesDAO{db: db}
}

// SetIdentity 注入 identity 客户端（Phase 2C/B2：禁止直查 public.users）。
func (d *HermesDAO) SetIdentity(c *identityclient.Client) {
	d.idClient = c
}

// DB 暴露底层句柄，供装配根派生同库的其他 DAO 与测试建表使用。
func (d *HermesDAO) DB() *gorm.DB { return d.db }

func (d *HermesDAO) CreateDeviceCode(ctx context.Context, code *localmodel.HermesDeviceCode) error {
	return d.db.WithContext(ctx).Create(code).Error
}

func (d *HermesDAO) ApproveDeviceCode(ctx context.Context, userCodeHash string, tenantID, userID uint, now time.Time) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var code localmodel.HermesDeviceCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_code_hash = ?", userCodeHash).
			First(&code).Error; err != nil {
			return err
		}
		if !now.Before(code.ExpiresAt) {
			return ErrHermesAuthorizationExpired
		}
		if code.Status != localmodel.HermesDeviceCodePending {
			return ErrHermesAuthorizationDenied
		}

		tenantCtx := tenant.WithContext(ctx, tenantID)
		var entitlement localmodel.HermesEntitlement
		if err := tx.WithContext(tenantCtx).
			Where("user_id = ? AND enabled = ?", userID, true).
			First(&entitlement).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrHermesEntitlementDisabled
			}
			return err
		}

		updates := map[string]any{
			"status":             localmodel.HermesDeviceCodeApproved,
			"approved_tenant_id": tenantID,
			"approved_user_id":   userID,
			"approved_at":        now,
		}
		result := tx.Model(&localmodel.HermesDeviceCode{}).
			Where("id = ? AND status = ?", code.ID, localmodel.HermesDeviceCodePending).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrHermesAuthorizationDenied
		}
		return nil
	})
}

// ConsumeDeviceCode atomically turns an approved registration into a device.
// The device poll has no tenant context, so the approved identity is recovered
// from the locked code row before tenant-scoped entitlement checks and writes.
func (d *HermesDAO) ConsumeDeviceCode(ctx context.Context, deviceCodeHash string, now time.Time, device *localmodel.HermesDevice) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var code localmodel.HermesDeviceCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("device_code_hash = ?", deviceCodeHash).
			First(&code).Error; err != nil {
			return err
		}
		if !now.Before(code.ExpiresAt) {
			return ErrHermesAuthorizationExpired
		}
		switch code.Status {
		case localmodel.HermesDeviceCodePending:
			return ErrHermesAuthorizationPending
		case localmodel.HermesDeviceCodeApproved:
			// Continue below.
		default:
			return ErrHermesAuthorizationDenied
		}
		if code.ApprovedTenantID == nil || code.ApprovedUserID == nil {
			return ErrHermesAuthorizationDenied
		}

		tenantCtx := tenant.WithContext(ctx, *code.ApprovedTenantID)
		var entitlement localmodel.HermesEntitlement
		if err := tx.WithContext(tenantCtx).
			Where("user_id = ? AND enabled = ?", *code.ApprovedUserID, true).
			First(&entitlement).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrHermesEntitlementDisabled
			}
			return err
		}

		var active int64
		if err := tx.WithContext(tenantCtx).Model(&localmodel.HermesDevice{}).
			Where("user_id = ? AND revoked_at IS NULL", *code.ApprovedUserID).
			Count(&active).Error; err != nil {
			return err
		}
		if active >= int64(entitlement.DeviceLimit) {
			return ErrHermesDeviceLimitReached
		}

		device.TenantID = *code.ApprovedTenantID
		device.UserID = *code.ApprovedUserID
		device.PublicKey = code.PublicKey
		device.KeyFingerprint = code.KeyFingerprint
		device.DeviceName = code.DeviceName
		device.Platform = code.Platform
		device.Arch = code.Arch
		if err := tx.WithContext(tenantCtx).Create(device).Error; err != nil {
			return err
		}

		result := tx.Model(&localmodel.HermesDeviceCode{}).
			Where("id = ? AND status = ?", code.ID, localmodel.HermesDeviceCodeApproved).
			Updates(map[string]any{
				"status":      localmodel.HermesDeviceCodeConsumed,
				"consumed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrHermesAuthorizationDenied
		}

		return tx.WithContext(tenantCtx).Create(&localmodel.HermesDeviceEvent{
			TenantID:   device.TenantID,
			DeviceID:   device.DeviceID,
			EventType:  "registered",
			Result:     "succeeded",
			OccurredAt: now,
		}).Error
	})
}

func (d *HermesDAO) GetDeviceIdentity(ctx context.Context, deviceID string) (*localmodel.HermesDevice, error) {
	var device localmodel.HermesDevice
	queryCtx := tenant.DisableScope(ctx)
	if err := d.db.WithContext(queryCtx).Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		return &device, err
	}
	// 策略级 revoked 可恢复，也使用 actual_state=revoked。永久吊销确认必须是
	// 服务端 revoked_at 之后到达的 revoked 心跳；更早的策略心跳不能关闭回执
	// 通道，事件偶发投递失败也不会让最终确认永久卡住。
	device.RevocationConfirmed = device.RevokedAt != nil &&
		device.ActualState == localmodel.HermesDesiredStateRevoked &&
		device.LastSeenAt != nil && device.LastSeenAt.After(*device.RevokedAt)
	return &device, nil
}

func (d *HermesDAO) GetDevice(ctx context.Context, deviceID string) (*localmodel.HermesDevice, error) {
	var device localmodel.HermesDevice
	err := d.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error
	return &device, err
}

func (d *HermesDAO) GetUserDevice(ctx context.Context, userID uint, deviceID string) (*localmodel.HermesDevice, error) {
	var device localmodel.HermesDevice
	err := d.db.WithContext(ctx).
		Where("device_id = ? AND user_id = ?", deviceID, userID).
		First(&device).Error
	return &device, err
}

func (d *HermesDAO) ListDevices(ctx context.Context, page, pageSize int, userID *uint) ([]localmodel.HermesDevice, int64, error) {
	devices := make([]localmodel.HermesDevice, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesDevice{})
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("last_seen_at DESC NULLS LAST, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&devices).Error
	return devices, total, err
}

func (d *HermesDAO) ListUserDevices(ctx context.Context, userID uint) ([]localmodel.HermesDevice, error) {
	devices := make([]localmodel.HermesDevice, 0)
	err := d.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Find(&devices).Error
	return devices, err
}

func (d *HermesDAO) UpdateHeartbeat(ctx context.Context, deviceID string, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesDevice{}).
		Where("device_id = ? AND revoked_at IS NULL", deviceID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrHermesDeviceRevoked
	}
	return nil
}

// ConfirmRevokedHeartbeat records a terminal manager heartbeat after the
// server has revoked a device. It accepts both successful local shutdown and
// an error state so the control plane never hides a failed stop attempt.
func (d *HermesDAO) ConfirmRevokedHeartbeat(ctx context.Context, deviceID string, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesDevice{}).
		Where("device_id = ? AND revoked_at IS NOT NULL", deviceID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrHermesDeviceRevoked
	}
	return nil
}

func (d *HermesDAO) RevokeDevice(ctx context.Context, deviceID string, userID *uint, now time.Time) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&localmodel.HermesDevice{}).
			Where("device_id = ? AND revoked_at IS NULL", deviceID)
		if userID != nil {
			query = query.Where("user_id = ?", *userID)
		}
		result := query.Updates(map[string]any{
			"revoked_at": now,
			"updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&localmodel.HermesDeviceEvent{
			TenantID:   tenant.FromContextOrDefault(ctx),
			DeviceID:   deviceID,
			EventType:  "revoke",
			Result:     "started",
			OccurredAt: now,
		}).Error
	})
}

func (d *HermesDAO) CreateDeviceEvent(ctx context.Context, event *localmodel.HermesDeviceEvent) error {
	return d.db.WithContext(ctx).Create(event).Error
}

func (d *HermesDAO) ListDeviceEvents(ctx context.Context, deviceID string, limit int) ([]localmodel.HermesDeviceEvent, error) {
	events := make([]localmodel.HermesDeviceEvent, 0)
	err := d.db.WithContext(ctx).Where("device_id = ?", deviceID).
		Order("occurred_at DESC, id DESC").Limit(limit).Find(&events).Error
	return events, err
}

func (d *HermesDAO) GetEntitlement(ctx context.Context, userID uint) (*localmodel.HermesEntitlement, error) {
	var entitlement localmodel.HermesEntitlement
	err := d.db.WithContext(ctx).Where("user_id = ?", userID).First(&entitlement).Error
	return &entitlement, err
}

func (d *HermesDAO) UpsertEntitlement(ctx context.Context, entitlement *localmodel.HermesEntitlement) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"enabled":      entitlement.Enabled,
			"policy_id":    entitlement.PolicyID,
			"device_limit": entitlement.DeviceLimit,
			"updated_by":   entitlement.UpdatedBy,
			"updated_at":   entitlement.UpdatedAt,
		}),
	}).Create(entitlement).Error
}

func (d *HermesDAO) ListEntitlements(ctx context.Context, page, pageSize int) ([]HermesEntitlementView, int64, error) {
	views := make([]HermesEntitlementView, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesEntitlement{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 不 JOIN users：显示名经 identity owner API 批量补全（schema 拆分后 users 不在 ai_svc）。
	err := query.
		Select(`hermes_entitlements.*,
			(SELECT COUNT(*) FROM hermes_devices d
			 WHERE d.tenant_id = hermes_entitlements.tenant_id
			   AND d.user_id = hermes_entitlements.user_id
			   AND d.revoked_at IS NULL) AS active_device_count,
			(SELECT MAX(d.last_seen_at) FROM hermes_devices d
			 WHERE d.tenant_id = hermes_entitlements.tenant_id
			   AND d.user_id = hermes_entitlements.user_id) AS last_seen_at`).
		Order("hermes_entitlements.updated_at DESC, hermes_entitlements.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&views).Error
	if err != nil {
		return nil, 0, err
	}
	d.fillEntitlementNames(ctx, views)
	return views, total, nil
}

func (d *HermesDAO) fillEntitlementNames(ctx context.Context, views []HermesEntitlementView) {
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
			// identity 返回 COALESCE(nickname, username)；双字段同填供前端兼容。
			views[i].Username = n
			views[i].Nickname = n
		}
	}
}

func (d *HermesDAO) UserExists(ctx context.Context, userID uint) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	// Phase 2C/B2：走 identity 显示名批量接口（命中即存在）。
	// 未注入客户端时（单测/本地未配 IDENTITY_API_BASE）放行，避免误拦管理写路径。
	if d.idClient == nil {
		return true, nil
	}
	names, err := d.idClient.BatchUserDisplayNames(ctx, []uint64{uint64(userID)})
	if err != nil {
		return false, err
	}
	_, ok := names[uint64(userID)]
	return ok, nil
}

func (d *HermesDAO) CreatePolicy(ctx context.Context, policy *localmodel.HermesPolicy) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxRevision uint64
		if err := tx.WithContext(ctx).Model(&localmodel.HermesPolicy{}).
			Select("COALESCE(MAX(revision), 0)").Scan(&maxRevision).Error; err != nil {
			return err
		}
		policy.Revision = maxRevision + 1
		return tx.WithContext(ctx).Create(policy).Error
	})
}

func (d *HermesDAO) ListPolicies(ctx context.Context, page, pageSize int) ([]localmodel.HermesPolicy, int64, error) {
	policies := make([]localmodel.HermesPolicy, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesPolicy{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("revision DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&policies).Error
	return policies, total, err
}

func (d *HermesDAO) GetPolicy(ctx context.Context, policyID uint64) (*localmodel.HermesPolicy, error) {
	var policy localmodel.HermesPolicy
	err := d.db.WithContext(ctx).First(&policy, policyID).Error
	return &policy, err
}

func (d *HermesDAO) UpdatePolicy(ctx context.Context, policy *localmodel.HermesPolicy) error {
	return d.db.WithContext(ctx).Save(policy).Error
}

func (d *HermesDAO) GetLatestPolicy(ctx context.Context) (*localmodel.HermesPolicy, error) {
	var policy localmodel.HermesPolicy
	err := d.db.WithContext(ctx).Order("revision DESC").First(&policy).Error
	return &policy, err
}

func (d *HermesDAO) GetArtifact(ctx context.Context, artifactID uint64) (*localmodel.HermesArtifact, error) {
	var artifact localmodel.HermesArtifact
	err := d.db.WithContext(ctx).First(&artifact, artifactID).Error
	return &artifact, err
}

func (d *HermesDAO) ListArtifacts(ctx context.Context, page, pageSize int, approvedOnly bool) ([]localmodel.HermesArtifact, int64, error) {
	artifacts := make([]localmodel.HermesArtifact, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesArtifact{})
	if approvedOnly {
		query = query.Where("status = ?", localmodel.HermesArtifactApproved)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&artifacts).Error
	return artifacts, total, err
}

// SetPendingAction 置位设备的运维动作。只对未吊销设备生效——对已吊销设备
// 排一个动作没有意义，manager 也不会再拉到期望状态。
func (d *HermesDAO) SetPendingAction(ctx context.Context, deviceID, action string, now time.Time) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesDevice{}).
		Where("device_id = ? AND revoked_at IS NULL", deviceID).
		Updates(map[string]any{
			"pending_action":    action,
			"pending_action_at": now,
			"updated_at":        now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set Hermes pending action: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// ClearPendingAction 在 manager 回执对应事件后清空动作。带 action 条件是为了
// 避免清掉执行期间管理员新排的另一个动作。
func (d *HermesDAO) ClearPendingAction(ctx context.Context, deviceID, action string, now time.Time) error {
	return d.db.WithContext(ctx).Model(&localmodel.HermesDevice{}).
		Where("device_id = ? AND pending_action = ?", deviceID, action).
		Updates(map[string]any{
			"pending_action":    "",
			"pending_action_at": nil,
			"updated_at":        now,
		}).Error
}

// ErrHermesArtifactDuplicate 对应 uk_hermes_artifacts_release 唯一索引：
// 同一 (kind, platform, arch, version) 只能录入一次，避免同版本号指向不同摘要。
var ErrHermesArtifactDuplicate = errors.New("hermes artifact already exists")

func (d *HermesDAO) CreateArtifact(ctx context.Context, artifact *localmodel.HermesArtifact) error {
	err := d.db.WithContext(ctx).Create(artifact).Error
	if err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err)) {
		return ErrHermesArtifactDuplicate
	}
	return err
}

// isUniqueViolation 在驱动未把错误翻译成 gorm.ErrDuplicatedKey 时退化为文本匹配。
// 各驱动措辞不同：Postgres 报索引名与 "duplicate key value"，sqlite 报
// "UNIQUE constraint failed"，因此三者都要认。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_hermes_artifacts_release") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

// SetArtifactStatus 做带来源状态守卫的状态流转，避免并发下把已吊销的制品改回批准。
func (d *HermesDAO) SetArtifactStatus(ctx context.Context, artifactID uint64, from, to string, actor uint, now time.Time) error {
	fields := map[string]any{"status": to, "updated_at": now}
	if to == localmodel.HermesArtifactApproved {
		fields["approved_by"] = actor
		fields["approved_at"] = now
	}
	result := d.db.WithContext(ctx).Model(&localmodel.HermesArtifact{}).
		Where("id = ? AND status = ?", artifactID, from).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set Hermes artifact status: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) ApproveArtifact(ctx context.Context, artifactID uint64, approver uint, now time.Time) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesArtifact{}).
		Where("id = ? AND status = ?", artifactID, localmodel.HermesArtifactPending).
		Updates(map[string]any{
			"status":      localmodel.HermesArtifactApproved,
			"approved_by": approver,
			"approved_at": now,
			"updated_at":  now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("approve Hermes artifact: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) CreateInstance(ctx context.Context, instance *localmodel.HermesInstance) error {
	instance.TenantID = tenant.FromContextOrDefault(ctx)
	return d.db.WithContext(ctx).Create(instance).Error
}

func (d *HermesDAO) GetInstance(ctx context.Context, id uint64) (*localmodel.HermesInstance, error) {
	var instance localmodel.HermesInstance
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

// GetInstanceByDevice 取绑定在该节点上的实例，**含已停用的**。
//
// 刻意不只查启用实例：调用方要能区分「该节点没有实例」（回落到权益策略，存量
// 设备走这条）与「该节点的实例被管理员停用」（应当停跑）。只查启用实例的话
// 两种情况都返回 not found，停用实例会悄悄退回权益策略继续跑，与管理员意图相反。
//
// 排序 enabled DESC, id ASC：优先返回启用实例。服务层保证同一节点至多一个启用
// 实例，排序只是让守卫万一被绕过时行为可预测，而不是随机取一条。
func (d *HermesDAO) GetInstanceByDevice(ctx context.Context, deviceID string) (*localmodel.HermesInstance, error) {
	var instance localmodel.HermesInstance
	err := d.db.WithContext(ctx).
		Where("device_id = ?", deviceID).
		Order("enabled DESC, id ASC").First(&instance).Error
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

// CountEnabledInstancesOnDevice 支撑单实例守卫。excludeID 用于更新场景：
// 把实例自己排除掉，否则改自己的名字都会被判成「第二个实例」。
func (d *HermesDAO) CountEnabledInstancesOnDevice(ctx context.Context, deviceID string, excludeID uint64) (int64, error) {
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesInstance{}).
		Where("device_id = ? AND enabled = ?", deviceID, true)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Count(&total).Error
	return total, err
}

func (d *HermesDAO) ListInstances(ctx context.Context, page, pageSize int, deviceID string) ([]localmodel.HermesInstance, int64, error) {
	instances := make([]localmodel.HermesInstance, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesInstance{})
	if deviceID != "" {
		query = query.Where("device_id = ?", deviceID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&instances).Error
	return instances, total, err
}

func (d *HermesDAO) UpdateInstance(ctx context.Context, id uint64, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesInstance{}).
		Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes instance: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) DeleteInstance(ctx context.Context, id uint64) error {
	result := d.db.WithContext(ctx).Where("id = ?", id).Delete(&localmodel.HermesInstance{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delete Hermes instance: %w", gorm.ErrRecordNotFound)
	}
	return nil
}
