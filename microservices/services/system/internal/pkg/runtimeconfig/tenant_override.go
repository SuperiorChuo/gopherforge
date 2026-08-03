package runtimeconfig

import (
	"context"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/database"
	"github.com/go-admin-kit/services/system/internal/pkg/tenant"
)

// TenantSettingStore 读取租户级覆盖行（tenant_settings）。
type TenantSettingStore interface {
	GetByKeyContext(ctx context.Context, tenantID uint, key string) (*model.TenantSetting, error)
}

// tenantOverride 返回显式租户上下文下某键的覆盖行；后台/无租户上下文
// （tenant.FromContext==0）返回 nil，消费方维持平台默认。
func tenantOverride(ctx context.Context, store TenantSettingStore, key string) *model.TenantSetting {
	if store == nil {
		return nil
	}
	tid := tenant.FromContext(ctx)
	if tid == 0 {
		return nil
	}
	setting, err := store.GetByKeyContext(ctx, tid, key)
	if err != nil || setting == nil {
		return nil
	}
	return setting
}

type defaultTenantSettingStore struct{}

func (defaultTenantSettingStore) GetByKeyContext(ctx context.Context, tenantID uint, key string) (*model.TenantSetting, error) {
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	return systemdao.NewTenantSettingDAO(database.DB).GetByKeyContext(ctx, tenantID, key)
}
