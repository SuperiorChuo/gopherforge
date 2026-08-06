package tenant

// 插件行为用例（激活与泛化后补齐）：schema 驱动的自动过滤/补值、
// DisableScope 逃生口、跨租户改删拦截、全局更新保护不被降级。

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scopedThing 带 tenant_id 列（任何此形状的模型都应被插件覆盖，不再白名单）。
type scopedThing struct {
	ID       uint   `gorm:"primaryKey"`
	TenantID uint   `gorm:"not null;default:1;index"`
	Name     string `gorm:"size:64"`
}

// platformThing 平台级表（无 tenant_id 列），应天然豁免。
type platformThing struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:64"`
}

var pluginDBSeq atomic.Int64

func openPluginTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:tenantplugin%d?mode=memory&cache=shared", pluginDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&scopedThing{}, &platformThing{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := Register(db); err != nil {
		t.Fatalf("register: %v", err)
	}
	// 种子：租户 1 两条、租户 2 一条（用 DisableScope 绕过 create 补值）
	seedCtx := DisableScope(context.Background())
	rows := []scopedThing{
		{TenantID: 1, Name: "t1-a"}, {TenantID: 1, Name: "t1-b"}, {TenantID: 2, Name: "t2-a"},
	}
	if err := db.WithContext(seedCtx).Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

// 查询：按 ctx 租户自动过滤；ctx 缺失不加过滤（与既有 DAO 语义一致）；
// DisableScope 显式跨租户；无 tenant_id 列的表不受影响。
func TestPluginQueryScoping(t *testing.T) {
	db := openPluginTest(t)

	var got []scopedThing
	if err := db.WithContext(WithContext(context.Background(), 1)).Find(&got).Error; err != nil {
		t.Fatalf("find t1: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("租户 1 应见 2 条, got %d", len(got))
	}
	got = nil
	if err := db.WithContext(WithContext(context.Background(), 2)).Find(&got).Error; err != nil {
		t.Fatalf("find t2: %v", err)
	}
	if len(got) != 1 || got[0].Name != "t2-a" {
		t.Fatalf("租户 2 应只见自己的 1 条, got %+v", got)
	}
	got = nil
	if err := db.WithContext(context.Background()).Find(&got).Error; err != nil {
		t.Fatalf("find no-ctx: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("无租户上下文应不加过滤（与既有 DAO 语义一致）, got %d", len(got))
	}
	got = nil
	if err := db.WithContext(DisableScope(WithContext(context.Background(), 2))).Find(&got).Error; err != nil {
		t.Fatalf("find disabled: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("DisableScope 应跨租户可见, got %d", len(got))
	}

	// 平台级表不受影响
	if err := db.WithContext(WithContext(context.Background(), 2)).
		Create(&platformThing{Name: "p1"}).Error; err != nil {
		t.Fatalf("platform create: %v", err)
	}
	var pl []platformThing
	if err := db.WithContext(WithContext(context.Background(), 1)).Find(&pl).Error; err != nil {
		t.Fatalf("platform find: %v", err)
	}
	if len(pl) != 1 {
		t.Fatalf("平台级表应豁免, got %d", len(pl))
	}
}

// 写入：ctx 租户为权威值自动补齐。
func TestPluginCreateFillsTenant(t *testing.T) {
	db := openPluginTest(t)
	row := scopedThing{Name: "t2-new"}
	if err := db.WithContext(WithContext(context.Background(), 2)).Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.TenantID != 2 {
		t.Fatalf("create 应补租户 2, got %d", row.TenantID)
	}
}

// 改/删：跨租户按 id 猜测应打空；全局更新保护不被插件降级。
func TestPluginMutateScoping(t *testing.T) {
	db := openPluginTest(t)
	var target scopedThing
	if err := db.WithContext(DisableScope(context.Background())).
		Where("name = ?", "t1-a").First(&target).Error; err != nil {
		t.Fatalf("load target: %v", err)
	}

	// 租户 2 上下文改租户 1 的行 → 0 行
	res := db.WithContext(WithContext(context.Background(), 2)).
		Model(&scopedThing{}).Where("id = ?", target.ID).Update("name", "hacked")
	if res.Error != nil || res.RowsAffected != 0 {
		t.Fatalf("跨租户更新应打空, err=%v rows=%d", res.Error, res.RowsAffected)
	}
	res = db.WithContext(WithContext(context.Background(), 2)).
		Where("id = ?", target.ID).Delete(&scopedThing{})
	if res.Error != nil || res.RowsAffected != 0 {
		t.Fatalf("跨租户删除应打空, err=%v rows=%d", res.Error, res.RowsAffected)
	}
	// 本租户正常
	res = db.WithContext(WithContext(context.Background(), 1)).
		Model(&scopedThing{}).Where("id = ?", target.ID).Update("name", "t1-a2")
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("本租户更新应生效, err=%v rows=%d", res.Error, res.RowsAffected)
	}

	// 无条件全局更新：仍应被 gorm ErrMissingWhereClause 拦截（插件不注水条件）
	res = db.WithContext(WithContext(context.Background(), 1)).
		Model(&scopedThing{}).Update("name", "boom")
	if !errors.Is(res.Error, gorm.ErrMissingWhereClause) {
		t.Fatalf("全局更新保护不应被降级, got %v", res.Error)
	}
}
