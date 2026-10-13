package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"strings"
	"time"
)

type HermesHeartbeatRequest struct {
	ManagerVersion        string `json:"manager_version"`
	HermesVersion         string `json:"hermes_version"`
	ActualState           string `json:"actual_state"`
	HealthStatus          string `json:"health_status"`
	AppliedPolicyRevision uint64 `json:"applied_policy_revision"`
	ConfigDigest          string `json:"config_digest"`
	LastErrorCode         string `json:"last_error_code"`
	// APIServerFingerprint 是设备端自持 Gateway API server 密钥的 sha256 前
	// 16 字节指纹（32 hex）。控制面只存指纹不持原始 key；空串表示未启用。
	APIServerFingerprint string `json:"api_server_fingerprint"`
}

func (s *HermesService) RecordHeartbeat(ctx context.Context, deviceID string, request HermesHeartbeatRequest) error {
	request.ManagerVersion = strings.TrimSpace(request.ManagerVersion)
	request.HermesVersion = strings.TrimSpace(request.HermesVersion)
	request.ActualState = strings.ToLower(strings.TrimSpace(request.ActualState))
	request.HealthStatus = strings.ToLower(strings.TrimSpace(request.HealthStatus))
	request.ConfigDigest = strings.ToLower(strings.TrimSpace(request.ConfigDigest))
	request.LastErrorCode = strings.ToUpper(strings.TrimSpace(request.LastErrorCode))
	if request.ManagerVersion != "" && !hermesVersionPattern.MatchString(request.ManagerVersion) {
		return fmt.Errorf("%w: invalid manager_version", ErrHermesInvalidInput)
	}
	if request.HermesVersion != "" && !hermesVersionPattern.MatchString(request.HermesVersion) {
		return fmt.Errorf("%w: invalid hermes_version", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{"registered", "installing", "active", "stopped", "updating", "revoked", "error"}, request.ActualState) {
		return fmt.Errorf("%w: invalid actual_state", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{"unknown", "healthy", "degraded", "unhealthy", "policy_drift"}, request.HealthStatus) {
		return fmt.Errorf("%w: invalid health_status", ErrHermesInvalidInput)
	}
	if request.ConfigDigest != "" && !hermesDigestPattern.MatchString(request.ConfigDigest) {
		return fmt.Errorf("%w: invalid config_digest", ErrHermesInvalidInput)
	}
	if request.LastErrorCode != "" && !hermesErrorCodePattern.MatchString(request.LastErrorCode) {
		return fmt.Errorf("%w: invalid last_error_code", ErrHermesInvalidInput)
	}
	request.APIServerFingerprint = strings.ToLower(strings.TrimSpace(request.APIServerFingerprint))
	if request.APIServerFingerprint != "" && !hermesHex32Pattern.MatchString(request.APIServerFingerprint) {
		return fmt.Errorf("%w: invalid api_server_fingerprint", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	updates := map[string]any{
		"manager_version":         request.ManagerVersion,
		"hermes_version":          request.HermesVersion,
		"actual_state":            request.ActualState,
		"health_status":           request.HealthStatus,
		"applied_policy_revision": request.AppliedPolicyRevision,
		"config_digest":           request.ConfigDigest,
		"last_error_code":         request.LastErrorCode,
		"api_server_fingerprint":  request.APIServerFingerprint,
		"last_seen_at":            now,
		"updated_at":              now,
	}
	if err := s.dao.UpdateHeartbeat(ctx, deviceID, updates); err != nil {
		if !errors.Is(err, dao.ErrHermesDeviceRevoked) {
			return err
		}
		// A permanently revoked device may only report the outcome of stopping
		// locally. Success converges to revoked; a stop failure converges to error
		// so administrators see the failure instead of a stale active state.
		if request.ActualState != localmodel.HermesDesiredStateRevoked && request.ActualState != "error" {
			return dao.ErrHermesDeviceRevoked
		}
		return s.dao.ConfirmRevokedHeartbeat(ctx, deviceID, updates)
	}
	return nil
}

type HermesEventRequest struct {
	EventType             string    `json:"event_type"`
	Result                string    `json:"result"`
	ErrorCode             string    `json:"error_code"`
	DesiredPolicyRevision uint64    `json:"desired_policy_revision"`
	AppliedPolicyRevision uint64    `json:"applied_policy_revision"`
	OccurredAt            time.Time `json:"occurred_at"`
}

func (s *HermesService) RecordEvent(ctx context.Context, deviceID string, request HermesEventRequest) error {
	request.EventType = strings.ToLower(strings.TrimSpace(request.EventType))
	request.Result = strings.ToLower(strings.TrimSpace(request.Result))
	request.ErrorCode = strings.ToUpper(strings.TrimSpace(request.ErrorCode))
	if !containsHermesValue([]string{"install", "start", "stop", "update", "rollback", "apply_policy", "policy_drift", "revoke"}, request.EventType) {
		return fmt.Errorf("%w: invalid event_type", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{"started", "succeeded", "failed"}, request.Result) {
		return fmt.Errorf("%w: invalid result", ErrHermesInvalidInput)
	}
	if request.ErrorCode != "" && !hermesErrorCodePattern.MatchString(request.ErrorCode) {
		return fmt.Errorf("%w: invalid error_code", ErrHermesInvalidInput)
	}
	device, err := s.dao.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if device.RevokedAt != nil &&
		(request.Result == "started" || !containsHermesValue([]string{"stop", "revoke"}, request.EventType)) {
		return fmt.Errorf("%w: revoked device may only report a terminal stop or revoke event", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	if request.OccurredAt.IsZero() {
		request.OccurredAt = now
	}
	if request.OccurredAt.Before(now.Add(-24*time.Hour)) || request.OccurredAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("%w: occurred_at is outside the accepted window", ErrHermesInvalidInput)
	}
	// 动作回执：只在终态清除待办动作。started 不清，否则动作会在执行途中丢失，
	// 设备中途崩溃就再也不会重试。失败也清——失败已被记成事件，重试由管理员再排。
	if request.Result == "succeeded" || request.Result == "failed" {
		for action, eventTypes := range hermesActionEventTypes {
			if containsHermesValue(eventTypes, request.EventType) {
				if err := s.dao.ClearPendingAction(ctx, deviceID, action, now); err != nil {
					return err
				}
			}
		}
	}

	return s.dao.CreateDeviceEvent(ctx, &localmodel.HermesDeviceEvent{
		TenantID:              tenant.FromContextOrDefault(ctx),
		DeviceID:              deviceID,
		EventType:             request.EventType,
		Result:                request.Result,
		ErrorCode:             request.ErrorCode,
		DesiredPolicyRevision: request.DesiredPolicyRevision,
		AppliedPolicyRevision: request.AppliedPolicyRevision,
		OccurredAt:            request.OccurredAt.UTC(),
		CreatedAt:             now,
	})
}
