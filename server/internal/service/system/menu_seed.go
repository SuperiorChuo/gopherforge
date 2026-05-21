package system

import (
	"context"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
)

type MenuBootstrapResult struct {
	Menus int `json:"menus"`
}

// Deprecated: use BootstrapDefaultMenusContext instead.
func BootstrapDefaultMenus() (MenuBootstrapResult, error) {
	return BootstrapDefaultMenusContext(context.Background())
}

func BootstrapDefaultMenusContext(ctx context.Context) (MenuBootstrapResult, error) {
	var result MenuBootstrapResult
	created, err := (&systemdao.MenuSeedDAO{}).BootstrapDefaultMenusContext(ctx, DefaultMenus(), time.Now())
	result.Menus = created
	return result, err
}

func DefaultMenus() []model.Menu {
	menus := make([]model.Menu, len(defaultMenuSeed))
	copy(menus, defaultMenuSeed)
	return menus
}

var defaultMenuSeed = []model.Menu{
	{ID: 1, Name: "dashboard", Title: "仪表盘", Icon: "dashboard", Path: "/dashboard", Component: "Layout", ParentID: 0, Sort: 0, Status: 1, Hidden: 0},
	{ID: 2, Name: "dashboard-index", Title: "系统概览", Icon: "dashboard", Path: "/dashboard/index", Component: "dashboard/index", ParentID: 1, Sort: 1, Status: 1, Hidden: 0, Permission: "dashboard.view"},

	{ID: 10, Name: "system", Title: "系统管理", Icon: "setting", Path: "/system", Component: "Layout", ParentID: 0, Sort: 1, Status: 1, Hidden: 0},
	{ID: 11, Name: "user", Title: "用户管理", Icon: "user", Path: "/system/user", Component: "system/user/index", ParentID: 10, Sort: 1, Status: 1, Hidden: 0, Permission: "system:user:list"},
	{ID: 12, Name: "role", Title: "角色管理", Icon: "user-safety", Path: "/system/role", Component: "system/role/index", ParentID: 10, Sort: 2, Status: 1, Hidden: 0, Permission: "system:role:list"},
	{ID: 13, Name: "permission", Title: "权限管理", Icon: "secured", Path: "/system/permission", Component: "system/permission/index", ParentID: 10, Sort: 3, Status: 1, Hidden: 0, Permission: "system:permission:list"},
	{ID: 14, Name: "menu", Title: "菜单管理", Icon: "menu", Path: "/system/menu", Component: "system/menu/index", ParentID: 10, Sort: 4, Status: 1, Hidden: 0, Permission: "system:menu:list"},
	{ID: 15, Name: "department", Title: "部门管理", Icon: "root-list", Path: "/system/department", Component: "system/department/index", ParentID: 10, Sort: 5, Status: 1, Hidden: 0, Permission: "system:department:list"},
	{ID: 16, Name: "file", Title: "文件管理", Icon: "file", Path: "/system/file", Component: "system/file/index", ParentID: 10, Sort: 6, Status: 1, Hidden: 0, Permission: "system:file:list"},
	{ID: 17, Name: "dict", Title: "字典管理", Icon: "data-base", Path: "/system/dict", Component: "system/dict/index", ParentID: 10, Sort: 7, Status: 1, Hidden: 0, Permission: "system:dict:list"},
	{ID: 18, Name: "notice", Title: "通知公告", Icon: "notification", Path: "/system/notice", Component: "system/notice/index", ParentID: 10, Sort: 8, Status: 1, Hidden: 0, Permission: "system:notice:list"},
	{ID: 19, Name: "online-user", Title: "在线用户", Icon: "user-list", Path: "/system/online-user", Component: "system/online-user/index", ParentID: 10, Sort: 9, Status: 1, Hidden: 0, Permission: "system:online-user:list"},
	{ID: 20, Name: "operation-log", Title: "操作日志", Icon: "time", Path: "/system/operation-log", Component: "system/operation-log/index", ParentID: 10, Sort: 10, Status: 1, Hidden: 0, Permission: "system:log:operation"},
	{ID: 21, Name: "login-log", Title: "登录日志", Icon: "time", Path: "/system/login-log", Component: "system/login-log/index", ParentID: 10, Sort: 11, Status: 1, Hidden: 0, Permission: "system:log:login"},

	{ID: 30, Name: "monitor", Title: "系统监控", Icon: "chart-analytics", Path: "/monitor", Component: "Layout", ParentID: 0, Sort: 2, Status: 1, Hidden: 0},
	{ID: 31, Name: "monitor-job", Title: "定时任务", Icon: "time", Path: "/monitor/job", Component: "monitor/job/index", ParentID: 30, Sort: 1, Status: 1, Hidden: 0, Permission: "system:job:list"},
	{ID: 32, Name: "monitor-server", Title: "服务器监控", Icon: "server", Path: "/monitor/server", Component: "monitor/server/index", ParentID: 30, Sort: 2, Status: 1, Hidden: 0, Permission: "system:monitor:server"},
	{ID: 33, Name: "monitor-mysql", Title: "数据库监控", Icon: "data-base", Path: "/monitor/mysql", Component: "monitor/mysql/index", ParentID: 30, Sort: 3, Status: 1, Hidden: 0, Permission: "system:monitor:mysql"},
	{ID: 34, Name: "monitor-redis", Title: "缓存监控", Icon: "data", Path: "/monitor/redis", Component: "monitor/redis/index", ParentID: 30, Sort: 4, Status: 1, Hidden: 0, Permission: "system:monitor:redis"},

	{ID: 40, Name: "profile", Title: "个人中心", Icon: "user-circle", Path: "/profile", Component: "Layout", ParentID: 0, Sort: 99, Status: 1, Hidden: 1},
	{ID: 41, Name: "profile-index", Title: "个人中心", Icon: "user", Path: "/profile/index", Component: "profile/index", ParentID: 40, Sort: 1, Status: 1, Hidden: 0},
}
