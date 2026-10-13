package service

import (
	"context"
	"errors"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/google/uuid"
	"time"
)

const hermesDeviceOnlineWindow = 3 * time.Minute

// HermesCapabilitySkill 是能力预览里的技能条目（M6）。
type HermesCapabilitySkill struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HermesDeviceCapabilities 是员工视角的「这台 Hermes 会什么」（M6 能力预览）。
// MCP 只暴露名字——URL/命令/env 模板不进员工视图。
type HermesDeviceCapabilities struct {
	Skills     []HermesCapabilitySkill `json:"skills"`
	McpServers []string                `json:"mcp_servers"`
	Toolsets   []string                `json:"toolsets"`
}

type HermesDeviceView struct {
	localmodel.HermesDevice
	DesiredState     string                    `json:"desired_state"`
	ConnectionStatus string                    `json:"connection_status"`
	Capabilities     *HermesDeviceCapabilities `json:"capabilities,omitempty"`
}

func (s *HermesService) deviceView(ctx context.Context, device localmodel.HermesDevice) (*HermesDeviceView, error) {
	desired, err := s.buildDesiredState(ctx, &device)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil && !errors.Is(err, ErrHermesPolicyNotConfigured) && logger.Logger != nil {
		logger.Warn("resolve Hermes device desired state failed",
			logger.Err(err),
			logger.String("device_id", device.DeviceID))
	}
	desiredState := "unknown"
	var capabilities *HermesDeviceCapabilities
	if err == nil && desired != nil {
		desiredState = desired.DesiredState
		// M6 能力预览：buildDesiredState 已解析出技能/MCP/工具集，直接折进视图。
		// MCP 只取名字，URL/命令/env 模板不进员工视图。
		skills := make([]HermesCapabilitySkill, 0, len(desired.Skills))
		for _, skill := range desired.Skills {
			skills = append(skills, HermesCapabilitySkill{Name: skill.Name, Version: skill.Version})
		}
		mcpNames := make([]string, 0, len(desired.McpServers))
		for _, server := range desired.McpServers {
			mcpNames = append(mcpNames, server.Name)
		}
		capabilities = &HermesDeviceCapabilities{
			Skills:     skills,
			McpServers: mcpNames,
			Toolsets:   desired.Toolsets,
		}
	}
	connectionStatus := "never"
	if device.LastSeenAt != nil {
		connectionStatus = "online"
		if device.LastSeenAt.Before(s.now().UTC().Add(-hermesDeviceOnlineWindow)) {
			connectionStatus = "offline"
		}
	}
	return &HermesDeviceView{
		HermesDevice:     device,
		DesiredState:     desiredState,
		ConnectionStatus: connectionStatus,
		Capabilities:     capabilities,
	}, nil
}

func (s *HermesService) deviceViews(ctx context.Context, devices []localmodel.HermesDevice) ([]HermesDeviceView, error) {
	views := make([]HermesDeviceView, 0, len(devices))
	for _, device := range devices {
		view, err := s.deviceView(ctx, device)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func (s *HermesService) ListDevices(ctx context.Context, page, pageSize int, userID *uint) ([]HermesDeviceView, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	devices, total, err := s.dao.ListDevices(ctx, page, pageSize, userID)
	if err != nil {
		return nil, 0, err
	}
	views, err := s.deviceViews(ctx, devices)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

func (s *HermesService) GetDevice(ctx context.Context, deviceID string) (*HermesDeviceView, []localmodel.HermesDeviceEvent, error) {
	device, err := s.dao.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}
	view, err := s.deviceView(ctx, *device)
	if err != nil {
		return nil, nil, err
	}
	events, err := s.dao.ListDeviceEvents(ctx, deviceID, 100)
	return view, events, err
}

func (s *HermesService) RevokeDevice(ctx context.Context, deviceID string, ownerUserID *uint) error {
	if _, err := uuid.Parse(deviceID); err != nil {
		return fmt.Errorf("%w: invalid device id", ErrHermesInvalidInput)
	}
	if err := s.dao.RevokeDevice(ctx, deviceID, ownerUserID, s.now().UTC()); err != nil {
		return err
	}
	// 同步切断模型访问。吊销 token 失败不回滚设备吊销——设备吊销本身已生效，
	// 且代理每请求都会实时校验设备状态，残留 token 也调不动。
	if s.tokenRevoker != nil {
		if err := s.tokenRevoker.RevokeDeviceTokens(ctx, deviceID); err != nil {
			logger.Warn("revoke Hermes model tokens failed", logger.Err(err), logger.String("device_id", deviceID))
		}
	}
	return nil
}
