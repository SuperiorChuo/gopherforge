package service

import (
	"context"
	"errors"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
	"strings"
)

// HermesInstanceRequest 是实例的创建/更新入参。DeviceID 为空表示不放置到节点，
// PolicyID 为 0 表示不覆盖策略（回落权益/最新）。
type HermesInstanceRequest struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	OwnerUserID uint   `json:"owner_user_id"`
	DeviceID    string `json:"device_id"`
	PolicyID    uint64 `json:"policy_id"`
	// ClusterID 为 0 表示未归组；>0 表示实例属于某个 company_cluster（多 Agent 协作）
	ClusterID uint64 `json:"cluster_id"`
	Enabled   bool   `json:"enabled"`
}

func (s *HermesService) ListInstances(ctx context.Context, page, pageSize int, deviceID string) ([]localmodel.HermesInstance, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListInstances(ctx, page, pageSize, strings.TrimSpace(deviceID))
}

// validateInstance 归一化并校验入参，同时执行单实例守卫。
// excludeID>0 表示更新场景，把实例自己排除在守卫计数之外。
func (s *HermesService) validateInstance(ctx context.Context, request *HermesInstanceRequest, excludeID uint64) error {
	request.Name = strings.TrimSpace(request.Name)
	request.Role = strings.TrimSpace(request.Role)
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	if request.Role == "" {
		request.Role = "default"
	}
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return fmt.Errorf("%w: invalid instance name", ErrHermesInvalidInput)
	}
	if len(request.Role) > 40 {
		return fmt.Errorf("%w: invalid instance role", ErrHermesInvalidInput)
	}
	if request.DeviceID != "" {
		// 绑定的节点必须存在且未吊销，否则实例会指向一台永远收不到期望状态的机器
		if _, err := s.dao.GetDevice(ctx, request.DeviceID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: device not found", ErrHermesInvalidInput)
			}
			return err
		}
	}
	if request.PolicyID > 0 {
		policy, err := s.dao.GetPolicy(ctx, request.PolicyID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: policy not found", ErrHermesInvalidInput)
			}
			return err
		}
		// company_cluster 策略不可绑定实例：它会随 buildDesiredState 进入签名
		// 期望状态，而 manager 本期未放行其工具集（见 errHermesClusterPolicyNotDeployable）。
		if policy.Profile == localmodel.HermesProfileCompanyCluster {
			return fmt.Errorf("%w: %v", ErrHermesInvalidInput, errHermesClusterPolicyNotDeployable)
		}
	}
	// 单实例守卫：只在「启用 且 已绑节点」时生效——未绑节点的实例不占用任何
	// 运行位，可以随意多建。
	if request.Enabled && request.DeviceID != "" {
		count, err := s.dao.CountEnabledInstancesOnDevice(ctx, request.DeviceID, excludeID)
		if err != nil {
			return err
		}
		if count > 0 {
			return ErrHermesInstanceLimit
		}
	}
	return nil
}

func hermesInstanceStatus(request HermesInstanceRequest) string {
	if request.DeviceID == "" {
		return localmodel.HermesInstanceStatusUnassigned
	}
	return localmodel.HermesInstanceStatusPending
}

func (s *HermesService) CreateInstance(ctx context.Context, actorID uint, request HermesInstanceRequest) (*localmodel.HermesInstance, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	if err := s.validateInstance(ctx, &request, 0); err != nil {
		return nil, err
	}
	instance := &localmodel.HermesInstance{
		Name:        request.Name,
		Role:        request.Role,
		OwnerUserID: request.OwnerUserID,
		DeviceID:    request.DeviceID,
		PolicyID:    request.PolicyID,
		ClusterID:   request.ClusterID,
		Enabled:     request.Enabled,
		Status:      hermesInstanceStatus(request),
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
	}
	if err := s.dao.CreateInstance(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *HermesService) UpdateInstance(ctx context.Context, actorID uint, id uint64, request HermesInstanceRequest) error {
	if actorID == 0 || id == 0 {
		return fmt.Errorf("%w: invalid instance", ErrHermesInvalidInput)
	}
	if _, err := s.dao.GetInstance(ctx, id); err != nil {
		return err
	}
	if err := s.validateInstance(ctx, &request, id); err != nil {
		return err
	}
	return s.dao.UpdateInstance(ctx, id, map[string]any{
		"name":          request.Name,
		"role":          request.Role,
		"owner_user_id": request.OwnerUserID,
		"device_id":     request.DeviceID,
		"policy_id":     request.PolicyID,
		"cluster_id":    request.ClusterID,
		"enabled":       request.Enabled,
		"status":        hermesInstanceStatus(request),
		"updated_by":    actorID,
		"updated_at":    s.now(),
	})
}

func (s *HermesService) DeleteInstance(ctx context.Context, id uint64) error {
	if id == 0 {
		return fmt.Errorf("%w: invalid instance", ErrHermesInvalidInput)
	}
	return s.dao.DeleteInstance(ctx, id)
}
