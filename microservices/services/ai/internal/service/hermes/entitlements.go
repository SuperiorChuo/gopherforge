package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type UpdateHermesEntitlementRequest struct {
	Enabled     bool   `json:"enabled"`
	PolicyID    uint64 `json:"policy_id"`
	DeviceLimit int    `json:"device_limit"`
}

func (s *HermesService) UpdateEntitlement(ctx context.Context, actorID, userID uint, request UpdateHermesEntitlementRequest) error {
	if userID == 0 || actorID == 0 || request.DeviceLimit < 1 || request.DeviceLimit > 10 {
		return fmt.Errorf("%w: invalid entitlement", ErrHermesInvalidInput)
	}
	exists, err := s.dao.UserExists(ctx, userID)
	if err != nil {
		return err
	}
	if !exists {
		return gorm.ErrRecordNotFound
	}
	if request.PolicyID > 0 {
		policy, err := s.dao.GetPolicy(ctx, request.PolicyID)
		if err != nil {
			return err
		}
		if policy.Profile == localmodel.HermesProfileCompanyCluster {
			return fmt.Errorf("%w: %v", ErrHermesInvalidInput, errHermesClusterPolicyNotDeployable)
		}
	}
	now := s.now().UTC()
	if err := s.dao.UpsertEntitlement(ctx, &localmodel.HermesEntitlement{
		TenantID:    tenant.FromContextOrDefault(ctx),
		UserID:      userID,
		Enabled:     request.Enabled,
		PolicyID:    request.PolicyID,
		DeviceLimit: request.DeviceLimit,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}
	// 停用即刻切断模型访问，不等 token 自然过期
	if !request.Enabled && s.tokenRevoker != nil {
		if err := s.tokenRevoker.RevokeUserTokens(ctx, userID); err != nil {
			logger.Warn("revoke Hermes model tokens failed", logger.Err(err), logger.Uint("user_id", userID))
		}
	}
	return nil
}

func (s *HermesService) ListEntitlements(ctx context.Context, page, pageSize int) ([]dao.HermesEntitlementView, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListEntitlements(ctx, page, pageSize)
}

type HermesMe struct {
	Entitlement *localmodel.HermesEntitlement `json:"entitlement"`
	Devices     []HermesDeviceView            `json:"devices"`
	Downloads   []localmodel.HermesArtifact   `json:"downloads"`
}

func (s *HermesService) GetMe(ctx context.Context, userID uint) (*HermesMe, error) {
	entitlement, err := s.dao.GetEntitlement(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		entitlement = nil
	}
	devices, err := s.dao.ListUserDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	views, err := s.deviceViews(ctx, devices)
	if err != nil {
		return nil, err
	}
	downloads, _, err := s.dao.ListArtifacts(ctx, 1, maxHermesPageSize, true)
	if err != nil {
		return nil, err
	}
	return &HermesMe{Entitlement: entitlement, Devices: views, Downloads: downloads}, nil
}
