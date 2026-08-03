package system

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

type SettingAPI struct {
	settingService systemsvc.SettingService
}

func NewSettingAPI() *SettingAPI {
	return &SettingAPI{settingService: systemsvc.SettingService{}}
}

// NewSettingAPIWithService creates a SettingAPI instance from an injected service.
func NewSettingAPIWithService(settingService systemsvc.SettingService) *SettingAPI {
	return &SettingAPI{settingService: settingService}
}

func (a *SettingAPI) GetSettings(c *gin.Context) {
	settings, err := a.settingService.ListSettingsContext(c.Request.Context(), c.Query("group"))
	if err != nil {
		internalServerError(c, "failed to get system settings", err)
		return
	}
	response.Success(c, settings)
}

func (a *SettingAPI) GetSetting(c *gin.Context) {
	setting, err := a.settingService.GetSettingContext(c.Request.Context(), c.Param("key"))
	if err != nil {
		writeSystemSettingServiceError(c, "failed to get system setting", err)
		return
	}
	response.Success(c, setting)
}

func (a *SettingAPI) UpsertSetting(c *gin.Context) {
	var req systemsvc.UpsertSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	req.SettingKey = c.Param("key")

	setting, err := a.settingService.UpsertSettingContext(c.Request.Context(), req)
	if err != nil {
		writeSystemSettingServiceError(c, "failed to update system setting", err)
		return
	}
	response.Success(c, setting)
}

func (a *SettingAPI) BatchUpsertSettings(c *gin.Context) {
	var req systemsvc.BatchUpsertSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	settings, err := a.settingService.BatchUpsertSettingsContext(c.Request.Context(), req)
	if err != nil {
		writeSystemSettingServiceError(c, "failed to update system settings", err)
		return
	}
	response.Success(c, settings)
}

func (a *SettingAPI) DeleteSetting(c *gin.Context) {
	if err := a.settingService.DeleteSettingContext(c.Request.Context(), c.Param("key")); err != nil {
		writeSystemSettingServiceError(c, "failed to delete system setting", err)
		return
	}
	response.SuccessWithMessage(c, "system setting deleted successfully", nil)
}

// ---------- 租户级配置覆盖（tenant_settings） ----------

func (a *SettingAPI) GetTenantSettings(c *gin.Context) {
	settings, err := a.settingService.ListTenantSettingsContext(c.Request.Context())
	if err != nil {
		writeTenantSettingServiceError(c, "failed to get tenant settings", err)
		return
	}
	response.Success(c, settings)
}

func (a *SettingAPI) UpsertTenantSetting(c *gin.Context) {
	var req systemsvc.UpsertSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	setting, err := a.settingService.UpsertTenantSettingContext(c.Request.Context(), c.Param("key"), req.ValueJSON)
	if err != nil {
		writeTenantSettingServiceError(c, "failed to update tenant setting", err)
		return
	}
	response.Success(c, setting)
}

func (a *SettingAPI) DeleteTenantSetting(c *gin.Context) {
	if err := a.settingService.DeleteTenantSettingContext(c.Request.Context(), c.Param("key")); err != nil {
		writeTenantSettingServiceError(c, "failed to delete tenant setting", err)
		return
	}
	response.SuccessWithMessage(c, "tenant setting deleted successfully", nil)
}

func writeTenantSettingServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrTenantSettingNotConfigurable):
		response.Forbidden(c, systemsvc.ErrTenantSettingNotConfigurable.Error())
	case errors.Is(err, systemsvc.ErrTenantSettingNotFound):
		response.NotFound(c, systemsvc.ErrTenantSettingNotFound.Error())
	case errors.Is(err, systemsvc.ErrInvalidSystemSettingKey):
		response.BadRequest(c, systemsvc.ErrInvalidSystemSettingKey.Error())
	case errors.Is(err, systemsvc.ErrTenantContextRequired):
		response.BadRequest(c, systemsvc.ErrTenantContextRequired.Error())
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemSettingServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrInvalidSystemSettingKey):
		response.BadRequest(c, systemsvc.ErrInvalidSystemSettingKey.Error())
	case errors.Is(err, systemsvc.ErrProtectedSystemSettingKey):
		response.Forbidden(c, systemsvc.ErrProtectedSystemSettingKey.Error())
	case errors.Is(err, systemsvc.ErrSystemSettingNotFound):
		response.NotFound(c, systemsvc.ErrSystemSettingNotFound.Error())
	default:
		internalServerError(c, operation, err)
	}
}
