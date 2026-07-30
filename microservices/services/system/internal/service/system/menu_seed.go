package system

import (
	"context"
	"time"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

type MenuBootstrapResult struct {
	Menus int `json:"menus"`
}

func BootstrapDefaultMenusContext(ctx context.Context, db *gorm.DB) (MenuBootstrapResult, error) {
	var result MenuBootstrapResult
	created, err := systemdao.NewMenuSeedDAO(db).BootstrapDefaultMenusContext(ctx, DefaultMenus(), time.Now())
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

	// ============ 系统管理拆组（2026-07-29，迁移 000030 同步已有库） ============
	// 原 19 个子菜单全堆在 /system 下，拆为 4 个一级分组：系统管理（组织+权限）/
	// 消息中心 / 日志审计 / 系统工具。子菜单 path/component/ID 全部不变（保住权限
	// 关联与书签），只动 ParentID 与 Sort。分组容器沿用主项目的 ID 134-136；
	// 42 起的 identity id 已被租户/OAuth2 等迁移占用，不能复用。
	{ID: 10, Name: "system", Title: "系统管理", Icon: "setting", Path: "/system", Component: "Layout", ParentID: 0, Sort: 1, Status: 1, Hidden: 0},
	{ID: 11, Name: "user", Title: "用户管理", Icon: "user", Path: "/system/user", Component: "system/user/index", ParentID: 10, Sort: 1, Status: 1, Hidden: 0, Permission: "system:user:list"},
	{ID: 12, Name: "role", Title: "角色管理", Icon: "user-safety", Path: "/system/role", Component: "system/role/index", ParentID: 10, Sort: 2, Status: 1, Hidden: 0, Permission: "system:role:list"},
	{ID: 13, Name: "permission", Title: "权限管理", Icon: "secured", Path: "/system/permission", Component: "system/permission/index", ParentID: 10, Sort: 3, Status: 1, Hidden: 0, Permission: "system:permission:list"},
	{ID: 14, Name: "menu", Title: "菜单管理", Icon: "menu", Path: "/system/menu", Component: "system/menu/index", ParentID: 10, Sort: 4, Status: 1, Hidden: 0, Permission: "system:menu:list"},
	{ID: 15, Name: "department", Title: "部门管理", Icon: "root-list", Path: "/system/department", Component: "system/department/index", ParentID: 10, Sort: 5, Status: 1, Hidden: 0, Permission: "system:department:list"},
	// 岗位管理：岗位 CRUD + 用户选岗（identity-service）
	{ID: 28, Name: "post", Title: "岗位管理", Icon: "idcard", Path: "/system/post", Component: "system/posts", ParentID: 10, Sort: 6, Status: 1, Hidden: 0, Permission: "system:post:list"},
	{ID: 24, Name: "tenant", Title: "租户管理", Icon: "team", Path: "/system/tenant", Component: "system/tenant/index", ParentID: 10, Sort: 7, Status: 1, Hidden: 0, Permission: "system:tenant:list"},
	// 租户套餐：权限包 CRUD + 租户绑定，租户内角色分配受套餐约束（identity-service）
	{ID: 29, Name: "tenant-packages", Title: "租户套餐", Icon: "appstore", Path: "/system/tenant-packages", Component: "system/tenant-packages", ParentID: 10, Sort: 8, Status: 1, Hidden: 0, Permission: "system:tenant-package:list"},
	{ID: 23, Name: "setting", Title: "系统设置", Icon: "setting", Path: "/system/setting", Component: "system/setting/index", ParentID: 10, Sort: 9, Status: 1, Hidden: 0, Permission: "system:setting:list"},

	// 消息中心：通知公告 / 短信
	{ID: 135, Name: "msg-center", Title: "消息中心", Icon: "notification", Path: "/msg", Component: "Layout", ParentID: 0, Sort: 2, Status: 1, Hidden: 0},
	{ID: 18, Name: "notice", Title: "通知公告", Icon: "notification", Path: "/system/notice", Component: "system/notice/index", ParentID: 135, Sort: 1, Status: 1, Hidden: 0, Permission: "system:notice:list"},
	// 短信管理：渠道/模板/发送日志
	{ID: 26, Name: "sms", Title: "短信管理", Icon: "mail", Path: "/system/sms", Component: "system/sms/index", ParentID: 135, Sort: 2, Status: 1, Hidden: 0, Permission: "system:sms-channel:list"},

	// 日志审计：三种日志 + 在线用户，纯查看
	{ID: 134, Name: "log-audit", Title: "日志审计", Icon: "file-text", Path: "/logs", Component: "Layout", ParentID: 0, Sort: 3, Status: 1, Hidden: 0},
	{ID: 20, Name: "operation-log", Title: "操作日志", Icon: "time", Path: "/system/operation-log", Component: "system/operation-log/index", ParentID: 134, Sort: 1, Status: 1, Hidden: 0, Permission: "system:log:operation"},
	{ID: 21, Name: "login-log", Title: "登录日志", Icon: "time", Path: "/system/login-log", Component: "system/login-log/index", ParentID: 134, Sort: 2, Status: 1, Hidden: 0, Permission: "system:log:login"},
	{ID: 22, Name: "audit-log", Title: "审计日志", Icon: "secured", Path: "/system/audit-log", Component: "system/audit-log/index", ParentID: 134, Sort: 3, Status: 1, Hidden: 0, Permission: "system:log:audit"},
	{ID: 19, Name: "online-user", Title: "在线用户", Icon: "user-list", Path: "/system/online-user", Component: "system/online-user/index", ParentID: 134, Sort: 4, Status: 1, Hidden: 0, Permission: "system:online-user:list"},

	// 系统工具：开发者/运维工具，普通管理员低频
	{ID: 136, Name: "sys-tools", Title: "系统工具", Icon: "tool", Path: "/tools", Component: "Layout", ParentID: 0, Sort: 4, Status: 1, Hidden: 0},
	{ID: 25, Name: "codegen", Title: "代码生成", Icon: "code", Path: "/system/codegen", Component: "system/codegen/index", ParentID: 136, Sort: 1, Status: 1, Hidden: 0, Permission: "system:codegen:list"},
	{ID: 17, Name: "dict", Title: "字典管理", Icon: "data-base", Path: "/system/dict", Component: "system/dict/index", ParentID: 136, Sort: 2, Status: 1, Hidden: 0, Permission: "system:dict:list"},
	{ID: 16, Name: "file", Title: "文件管理", Icon: "file", Path: "/system/file", Component: "system/file/index", ParentID: 136, Sort: 3, Status: 1, Hidden: 0, Permission: "system:file:list"},
	// 错误码管理：错误文案在线改，30s 热生效
	{ID: 27, Name: "errcodes", Title: "错误码管理", Icon: "warning", Path: "/system/errcodes", Component: "system/errcodes/index", ParentID: 136, Sort: 4, Status: 1, Hidden: 0, Permission: "system:errcode:list"},
	// 注意：/system/oauth2 刻意不在本表里。它由 000024_add_oauth2_server.sql 用自增 ID
	// 插入，000030 已把它改挂系统工具（136）Sort 5。本 seed 只按 ID 去重、不按 path 去重，
	// 再加一条会在同一路径上种出第二个菜单。

	{ID: 30, Name: "monitor", Title: "系统监控", Icon: "chart-analytics", Path: "/monitor", Component: "Layout", ParentID: 0, Sort: 5, Status: 1, Hidden: 0},
	{ID: 31, Name: "monitor-job", Title: "定时任务", Icon: "time", Path: "/monitor/job", Component: "monitor/job/index", ParentID: 30, Sort: 1, Status: 1, Hidden: 0, Permission: "system:job:list"},
	{ID: 32, Name: "monitor-server", Title: "服务器监控", Icon: "server", Path: "/monitor/server", Component: "monitor/server/index", ParentID: 30, Sort: 2, Status: 1, Hidden: 0, Permission: "system:monitor:server"},
	{ID: 33, Name: "monitor-mysql", Title: "数据库监控", Icon: "data-base", Path: "/monitor/mysql", Component: "monitor/mysql/index", ParentID: 30, Sort: 3, Status: 1, Hidden: 0, Permission: "system:monitor:mysql"},
	{ID: 34, Name: "monitor-redis", Title: "缓存监控", Icon: "data", Path: "/monitor/redis", Component: "monitor/redis/index", ParentID: 30, Sort: 4, Status: 1, Hidden: 0, Permission: "system:monitor:redis"},

	// 审批中心（bpm-service）：流程定义（需权限）+ 待办/我发起的（登录即见）
	{ID: 35, Name: "bpm", Title: "审批中心", Icon: "audit", Path: "/bpm", Component: "Layout", ParentID: 0, Sort: 6, Status: 1, Hidden: 0},
	{ID: 36, Name: "bpm-tasks", Title: "待办中心", Icon: "check", Path: "/bpm/tasks", Component: "bpm/tasks/index", ParentID: 35, Sort: 1, Status: 1, Hidden: 0},
	{ID: 37, Name: "bpm-instances", Title: "我发起的", Icon: "send", Path: "/bpm/instances", Component: "bpm/instances/index", ParentID: 35, Sort: 2, Status: 1, Hidden: 0},
	{ID: 38, Name: "bpm-definitions", Title: "流程定义", Icon: "fork", Path: "/bpm/definitions", Component: "bpm/definitions/index", ParentID: 35, Sort: 3, Status: 1, Hidden: 0, Permission: "bpm:definition:list"},

	{ID: 40, Name: "profile", Title: "个人中心", Icon: "user-circle", Path: "/profile", Component: "Layout", ParentID: 0, Sort: 99, Status: 1, Hidden: 1},
	{ID: 41, Name: "profile-index", Title: "个人中心", Icon: "user", Path: "/profile/index", Component: "profile/index", ParentID: 40, Sort: 1, Status: 1, Hidden: 0},
}
