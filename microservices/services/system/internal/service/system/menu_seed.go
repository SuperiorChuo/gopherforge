package system

import (
	"context"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
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

	// ============ 系统管理拆组（2026-07-29，迁移 000036 同步线上库） ============
	// 原 24 个子菜单全堆在 /system 下，拆为 4 个一级分组：系统管理（组织+权限）/
	// 消息中心 / 日志审计 / 系统工具，AI 运营三件套挪入「AI 智能」。
	// 子菜单 path/component/ID 全部不变（保住权限关联与书签），只动 ParentID 与 Sort。
	// 分组容器 ID 用 134-136（种子已占到 133，自增序列在 904+，两头不撞）。
	{ID: 10, Name: "system", Title: "系统管理", Icon: "setting", Path: "/system", Component: "Layout", ParentID: 0, Sort: 1, Status: 1, Hidden: 0},
	{ID: 11, Name: "user", Title: "用户管理", Icon: "user", Path: "/system/user", Component: "system/user/index", ParentID: 10, Sort: 1, Status: 1, Hidden: 0, Permission: "system:user:list"},
	{ID: 12, Name: "role", Title: "角色管理", Icon: "user-safety", Path: "/system/role", Component: "system/role/index", ParentID: 10, Sort: 2, Status: 1, Hidden: 0, Permission: "system:role:list"},
	{ID: 13, Name: "permission", Title: "权限管理", Icon: "secured", Path: "/system/permission", Component: "system/permission/index", ParentID: 10, Sort: 3, Status: 1, Hidden: 0, Permission: "system:permission:list"},
	{ID: 140, Name: "permission-diagnostics", Title: "权限诊断", Icon: "safety", Path: "/system/permission-diagnostics", Component: "system/permission-diagnostics/index", ParentID: 10, Sort: 4, Status: 1, Hidden: 0, Permission: "system:permission:diagnose"},
	{ID: 14, Name: "menu", Title: "菜单管理", Icon: "menu", Path: "/system/menu", Component: "system/menu/index", ParentID: 10, Sort: 5, Status: 1, Hidden: 0, Permission: "system:menu:list"},
	{ID: 15, Name: "department", Title: "部门管理", Icon: "root-list", Path: "/system/department", Component: "system/department/index", ParentID: 10, Sort: 6, Status: 1, Hidden: 0, Permission: "system:department:list"},
	// 岗位管理：岗位 CRUD + 用户选岗（identity-service）
	{ID: 121, Name: "post", Title: "岗位管理", Icon: "idcard", Path: "/system/post", Component: "system/posts", ParentID: 10, Sort: 7, Status: 1, Hidden: 0, Permission: "system:post:list"},
	{ID: 24, Name: "tenant", Title: "租户管理", Icon: "team", Path: "/system/tenant", Component: "system/tenant/index", ParentID: 10, Sort: 8, Status: 1, Hidden: 0, Permission: "system:tenant:list"},
	// 租户套餐：权限包 CRUD + 租户绑定，租户内角色分配受套餐约束
	{ID: 126, Name: "tenant-packages", Title: "租户套餐", Icon: "appstore", Path: "/system/tenant-packages", Component: "system/tenant-packages", ParentID: 10, Sort: 9, Status: 1, Hidden: 0, Permission: "system:tenant-package:list"},
	{ID: 23, Name: "setting", Title: "系统设置", Icon: "setting", Path: "/system/setting", Component: "system/setting/index", ParentID: 10, Sort: 10, Status: 1, Hidden: 0, Permission: "system:setting:list"},
	// 边缘证书菜单由迁移 000074 按 path 幂等插入，避免与固定 ID 种子重复。

	// 消息中心：通知公告/站内信/通知模板/短信（notify + sms 一家）
	{ID: 135, Name: "msg-center", Title: "消息中心", Icon: "notification", Path: "/msg", Component: "Layout", ParentID: 0, Sort: 2, Status: 1, Hidden: 0},
	{ID: 18, Name: "notice", Title: "通知公告", Icon: "notification", Path: "/system/notice", Component: "system/notice/index", ParentID: 135, Sort: 1, Status: 1, Hidden: 0, Permission: "system:notice:list"},
	// 个人站内信收件箱（notify-service）；登录即可见，无独立权限码
	{ID: 115, Name: "inbox", Title: "站内信", Icon: "mail", Path: "/system/inbox", Component: "system/inbox/index", ParentID: 135, Sort: 2, Status: 1, Hidden: 0},
	// 通知模板运营（改文案/渠道/试发）；117 已被 im-sensitive 占用
	{ID: 118, Name: "notify-templates", Title: "通知模板", Icon: "notification", Path: "/system/notify-templates", Component: "system/notify-templates/index", ParentID: 135, Sort: 3, Status: 1, Hidden: 0, Permission: "notify:template:read"},
	// 短信管理：渠道/模板/发送日志（119；118 已被 notify-templates 占用）
	{ID: 119, Name: "sms", Title: "短信管理", Icon: "mail", Path: "/system/sms", Component: "system/sms/index", ParentID: 135, Sort: 4, Status: 1, Hidden: 0, Permission: "system:sms-channel:list"},

	// 日志审计：三种日志 + 在线用户，纯查看
	{ID: 134, Name: "log-audit", Title: "日志审计", Icon: "file-text", Path: "/logs", Component: "Layout", ParentID: 0, Sort: 3, Status: 1, Hidden: 0},
	{ID: 20, Name: "operation-log", Title: "操作日志", Icon: "time", Path: "/system/operation-log", Component: "system/operation-log/index", ParentID: 134, Sort: 1, Status: 1, Hidden: 0, Permission: "system:log:operation"},
	{ID: 21, Name: "login-log", Title: "登录日志", Icon: "time", Path: "/system/login-log", Component: "system/login-log/index", ParentID: 134, Sort: 2, Status: 1, Hidden: 0, Permission: "system:log:login"},
	{ID: 22, Name: "audit-log", Title: "审计日志", Icon: "secured", Path: "/system/audit-log", Component: "system/audit-log/index", ParentID: 134, Sort: 3, Status: 1, Hidden: 0, Permission: "system:log:audit"},
	{ID: 138, Name: "security-events", Title: "安全事件", Icon: "warning", Path: "/system/security-events", Component: "system/security-events/index", ParentID: 134, Sort: 5, Status: 1, Hidden: 0, Permission: "system:security:list"},
	{ID: 139, Name: "login-security", Title: "登录安全", Icon: "secured", Path: "/system/login-security", Component: "system/login-security/index", ParentID: 134, Sort: 6, Status: 1, Hidden: 0, Permission: "system:login-security:list"},
	{ID: 19, Name: "online-user", Title: "在线用户", Icon: "user-list", Path: "/system/online-user", Component: "system/online-user/index", ParentID: 134, Sort: 4, Status: 1, Hidden: 0, Permission: "system:online-user:list"},

	// 系统工具：开发者/运维工具，普通管理员低频
	{ID: 136, Name: "sys-tools", Title: "系统工具", Icon: "tool", Path: "/tools", Component: "Layout", ParentID: 0, Sort: 4, Status: 1, Hidden: 0},
	{ID: 112, Name: "codegen", Title: "代码生成", Icon: "code", Path: "/system/codegen", Component: "system/codegen/index", ParentID: 136, Sort: 1, Status: 1, Hidden: 0, Permission: "system:codegen:list"},
	{ID: 17, Name: "dict", Title: "字典管理", Icon: "data-base", Path: "/system/dict", Component: "system/dict/index", ParentID: 136, Sort: 2, Status: 1, Hidden: 0, Permission: "system:dict:list"},
	{ID: 16, Name: "file", Title: "文件管理", Icon: "file", Path: "/system/file", Component: "system/file/index", ParentID: 136, Sort: 3, Status: 1, Hidden: 0, Permission: "system:file:list"},
	// 错误码管理：错误文案在线改，30s 热生效
	{ID: 120, Name: "errcodes", Title: "错误码管理", Icon: "warning", Path: "/system/errcodes", Component: "system/errcodes/index", ParentID: 136, Sort: 4, Status: 1, Hidden: 0, Permission: "system:errcode:list"},
	// /system/oauth2 由 goose 迁移创建并安排父节点；保留存量菜单身份。
	// 本表中的 ID 仅用于描述种子父子关系，持久化时按 name/path 解析现有 ID。

	// 顶级 Sort=7 预留给 /hermes（迁移 000034 插入，不在本 seed）
	{ID: 30, Name: "monitor", Title: "系统监控", Icon: "chart-analytics", Path: "/monitor", Component: "Layout", ParentID: 0, Sort: 5, Status: 1, Hidden: 0},
	{ID: 31, Name: "monitor-job", Title: "任务中心", Icon: "time", Path: "/monitor/job", Component: "monitor/job/index", ParentID: 30, Sort: 1, Status: 1, Hidden: 0, Permission: "system:job:list"},
	{ID: 32, Name: "monitor-server", Title: "服务器监控", Icon: "server", Path: "/monitor/server", Component: "monitor/server/index", ParentID: 30, Sort: 2, Status: 1, Hidden: 0, Permission: "system:monitor:server"},
	{ID: 33, Name: "monitor-mysql", Title: "数据库监控", Icon: "data-base", Path: "/monitor/mysql", Component: "monitor/mysql/index", ParentID: 30, Sort: 3, Status: 1, Hidden: 0, Permission: "system:monitor:mysql"},
	{ID: 34, Name: "monitor-redis", Title: "缓存监控", Icon: "data", Path: "/monitor/redis", Component: "monitor/redis/index", ParentID: 30, Sort: 4, Status: 1, Hidden: 0, Permission: "system:monitor:redis"},
	{ID: 110, Name: "monitor-grafana", Title: "实时看板", Icon: "dashboard", Path: "/monitor/grafana", Component: "monitor/grafana/index", ParentID: 30, Sort: 5, Status: 1, Hidden: 0},
	{ID: 42, Name: "monitor-jaeger", Title: "链路追踪", Icon: "share", Path: "/monitor/jaeger", Component: "monitor/jaeger/index", ParentID: 30, Sort: 6, Status: 1, Hidden: 0},

	{ID: 40, Name: "profile", Title: "个人中心", Icon: "user-circle", Path: "/profile", Component: "Layout", ParentID: 0, Sort: 99, Status: 1, Hidden: 1},
	{ID: 41, Name: "profile-index", Title: "个人中心", Icon: "user", Path: "/profile/index", Component: "profile/index", ParentID: 40, Sort: 1, Status: 1, Hidden: 0},

	// AI/IM 页面无独立权限码，登录即可见（与路由行为一致）
	{ID: 50, Name: "ai", Title: "AI 智能", Icon: "robot", Path: "/ai", Component: "Layout", ParentID: 0, Sort: 6, Status: 1, Hidden: 0},
	{ID: 51, Name: "ai-assistant", Title: "AI 助手", Icon: "robot", Path: "/ai/assistant", Component: "ai/assistant/index", ParentID: 50, Sort: 1, Status: 1, Hidden: 0},
	{ID: 52, Name: "ai-knowledge", Title: "AI 知识库", Icon: "book", Path: "/ai/knowledge", Component: "ai/knowledge/index", ParentID: 50, Sort: 2, Status: 1, Hidden: 0},
	// AI 运营三件套（ai-service 的 /api/v1/ai/{models,usage,quotas}）是平台管理面，
	// 后端与菜单统一使用动作级权限。path 保留 /system 前缀（书签/路由不动）。
	{ID: 131, Name: "ai-models", Title: "AI 模型配置", Icon: "control", Path: "/system/ai-models", Component: "system/ai-models/index", ParentID: 50, Sort: 3, Status: 1, Hidden: 0, Permission: "ai:model:list"},
	{ID: 132, Name: "ai-usage", Title: "AI 用量报表", Icon: "chart", Path: "/system/ai-usage", Component: "system/ai-usage/index", ParentID: 50, Sort: 4, Status: 1, Hidden: 0, Permission: "ai:usage:list"},
	{ID: 133, Name: "ai-quotas", Title: "AI 配额管理", Icon: "fund", Path: "/system/ai-quotas", Component: "system/ai-quotas/index", ParentID: 50, Sort: 5, Status: 1, Hidden: 0, Permission: "ai:quota:list"},

	{ID: 60, Name: "im", Title: "智能客服", Icon: "message", Path: "/im", Component: "Layout", ParentID: 0, Sort: 8, Status: 1, Hidden: 0},
	{ID: 61, Name: "im-desk", Title: "坐席工作台", Icon: "message", Path: "/im/desk", Component: "im/desk/index", ParentID: 60, Sort: 1, Status: 1, Hidden: 0, Permission: "im:agent:access"},
	{ID: 62, Name: "im-sites", Title: "埋码站点", Icon: "global", Path: "/im/sites", Component: "im/sites/index", ParentID: 60, Sort: 2, Status: 1, Hidden: 0, Permission: "im:admin:manage"},
	{ID: 63, Name: "im-skills", Title: "技能组", Icon: "team", Path: "/im/skills", Component: "im/skills/index", ParentID: 60, Sort: 3, Status: 1, Hidden: 0, Permission: "im:admin:manage"},
	{ID: 64, Name: "im-stats", Title: "客服统计", Icon: "data", Path: "/im/stats", Component: "im/stats/index", ParentID: 60, Sort: 4, Status: 1, Hidden: 0, Permission: "im:agent:access"},
	// 工单中心：挂在智能客服下，承接「会话 → 工单 → 关单」闭环。
	{ID: 113, Name: "ticket", Title: "工单中心", Icon: "file-list", Path: "/ticket", Component: "ticket/index", ParentID: 60, Sort: 5, Status: 1, Hidden: 0, Permission: "ticket:read"},
	// 机器人未命中知识库的问题回流，方便补文档
	{ID: 114, Name: "im-kb-misses", Title: "待补充知识", Icon: "book", Path: "/im/kb-misses", Component: "im/kb-misses/index", ParentID: 60, Sort: 6, Status: 1, Hidden: 0, Permission: "im:agent:access"},
	// 访客低分评价回流，质检复盘 / 转工单
	{ID: 116, Name: "im-low-ratings", Title: "低分会话", Icon: "star", Path: "/im/low-ratings", Component: "im/low-ratings/index", ParentID: 60, Sort: 7, Status: 1, Hidden: 0, Permission: "im:agent:access"},
	// 敏感词词库：会话文本打码/拦截，控制台维护热生效
	{ID: 117, Name: "im-sensitive", Title: "敏感词", Icon: "safety", Path: "/im/sensitive", Component: "im/sensitive/index", ParentID: 60, Sort: 8, Status: 1, Hidden: 0, Permission: "im:admin:manage"},

	{ID: 80, Name: "crm", Title: "销售 CRM", Icon: "fund", Path: "/crm", Component: "Layout", ParentID: 0, Sort: 10, Status: 1, Hidden: 0},
	{ID: 81, Name: "crm-dashboard", Title: "销售看板", Icon: "fund", Path: "/crm/dashboard", Component: "crm/dashboard/index", ParentID: 80, Sort: 1, Status: 1, Hidden: 0, Permission: "crm:read"},
	{ID: 82, Name: "crm-leads", Title: "所有名片", Icon: "user-list", Path: "/crm/leads", Component: "crm/leads/index", ParentID: 80, Sort: 2, Status: 1, Hidden: 0, Permission: "crm:read"},
	{ID: 83, Name: "crm-customers", Title: "客户管理", Icon: "team", Path: "/crm/customers", Component: "crm/customers/index", ParentID: 80, Sort: 3, Status: 1, Hidden: 0, Permission: "crm:read"},
	{ID: 84, Name: "crm-pipeline", Title: "商机管道", Icon: "fork", Path: "/crm/pipeline", Component: "crm/pipeline/index", ParentID: 80, Sort: 4, Status: 1, Hidden: 0, Permission: "crm:read"},
	{ID: 85, Name: "crm-contracts", Title: "合同回款", Icon: "file", Path: "/crm/contracts", Component: "crm/contracts/index", ParentID: 80, Sort: 5, Status: 1, Hidden: 0, Permission: "crm:read"},
	{ID: 100, Name: "crm-pool-rules", Title: "回收规则", Icon: "setting", Path: "/crm/pool-rules", Component: "crm/pool-rules/index", ParentID: 80, Sort: 6, Status: 1, Hidden: 0, Permission: "crm:read"},

	{ID: 70, Name: "cc", Title: "呼叫中心", Icon: "phone", Path: "/cc", Component: "Layout", ParentID: 0, Sort: 9, Status: 1, Hidden: 0},
	// 呼叫中心按功能域分 5 组（08-07 菜单重组）：容器挂 Layout，叶子挂各自子组。
	{ID: 200, Name: "cc-group-work", Title: "工作台", Icon: "desktop", Path: "/cc/work", Component: "Layout", ParentID: 70, Sort: 0, Status: 1, Hidden: 0},
	{ID: 111, Name: "cc-workbench", Title: "坐席工作台", Icon: "desktop", Path: "/cc/workbench", Component: "cc/workbench/index", ParentID: 200, Sort: 0, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 72, Name: "cc-console", Title: "呼叫控制台", Icon: "control", Path: "/cc/console", Component: "cc/console/index", ParentID: 200, Sort: 1, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 99, Name: "cc-monitor", Title: "话务监控", Icon: "radar", Path: "/cc/monitor", Component: "cc/monitor/index", ParentID: 200, Sort: 2, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 201, Name: "cc-group-calls", Title: "通话", Icon: "phone", Path: "/cc/calls-grp", Component: "Layout", ParentID: 70, Sort: 1, Status: 1, Hidden: 0},
	{ID: 71, Name: "cc-calls", Title: "呼叫话单", Icon: "phone", Path: "/cc/calls", Component: "cc/calls/index", ParentID: 201, Sort: 0, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 74, Name: "cc-recordings", Title: "通话录音", Icon: "sound", Path: "/cc/recordings", Component: "cc/recordings/index", ParentID: 201, Sort: 1, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 106, Name: "cc-voicemail", Title: "语音留言", Icon: "mail", Path: "/cc/voicemail", Component: "cc/voicemail/index", ParentID: 201, Sort: 2, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 79, Name: "cc-blacklist", Title: "呼叫风控", Icon: "stop", Path: "/cc/blacklist", Component: "cc/blacklist/index", ParentID: 201, Sort: 3, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 205, Name: "cc-voice", Title: "语音通知", Icon: "audio", Path: "/cc/voice", Component: "cc/voice/index", ParentID: 201, Sort: 4, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 202, Name: "cc-group-routing", Title: "呼叫路由", Icon: "fork", Path: "/cc/routing", Component: "Layout", ParentID: 70, Sort: 2, Status: 1, Hidden: 0},
	{ID: 77, Name: "cc-queues", Title: "呼叫队列", Icon: "queue", Path: "/cc/queues", Component: "cc/queues/index", ParentID: 202, Sort: 0, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 76, Name: "cc-ivr", Title: "IVR 导航", Icon: "fork", Path: "/cc/ivr", Component: "cc/ivr/index", ParentID: 202, Sort: 1, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 101, Name: "cc-time-routes", Title: "时间路由", Icon: "clock", Path: "/cc/timeroutes", Component: "cc/timeroutes/index", ParentID: 202, Sort: 2, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 102, Name: "cc-ring-groups", Title: "响铃组", Icon: "bell", Path: "/cc/ringgroups", Component: "cc/ringgroups/index", ParentID: 202, Sort: 3, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 98, Name: "cc-conference", Title: "会议室", Icon: "video", Path: "/cc/conference", Component: "cc/conference/index", ParentID: 202, Sort: 4, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 203, Name: "cc-group-lines", Title: "线路与资源", Icon: "gateway", Path: "/cc/lines", Component: "Layout", ParentID: 70, Sort: 3, Status: 1, Hidden: 0},
	{ID: 75, Name: "cc-trunks", Title: "SIP 线路", Icon: "gateway", Path: "/cc/trunks", Component: "cc/trunks/index", ParentID: 203, Sort: 0, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 73, Name: "cc-extensions", Title: "分机管理", Icon: "team", Path: "/cc/extensions", Component: "cc/extensions/index", ParentID: 203, Sort: 1, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 78, Name: "cc-moh", Title: "排队音乐", Icon: "music", Path: "/cc/moh", Component: "cc/moh/index", ParentID: 203, Sort: 2, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 97, Name: "cc-prompts", Title: "提示音库", Icon: "audio", Path: "/cc/prompts", Component: "cc/prompts/index", ParentID: 203, Sort: 3, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 204, Name: "cc-group-qa", Title: "质检报表", Icon: "chart", Path: "/cc/qa", Component: "Layout", ParentID: 70, Sort: 4, Status: 1, Hidden: 0},
	{ID: 105, Name: "cc-qc", Title: "质检中心", Icon: "safety", Path: "/cc/qc", Component: "cc/qc/index", ParentID: 204, Sort: 0, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 103, Name: "cc-reports", Title: "呼叫报表", Icon: "chart", Path: "/cc/reports", Component: "cc/reports/index", ParentID: 204, Sort: 1, Status: 1, Hidden: 0, Permission: "cc:read"},
	{ID: 104, Name: "cc-billing", Title: "计费管理", Icon: "money", Path: "/cc/billing", Component: "cc/billing/index", ParentID: 204, Sort: 2, Status: 1, Hidden: 0, Permission: "cc:read"},

	{ID: 92, Name: "visibility", Title: "搜索与 AI 可见度", Icon: "aim", Path: "/visibility", Component: "Layout", ParentID: 0, Sort: 11, Status: 1, Hidden: 0},
	// 子项路径必须与容器(92)区分开：父子同为 /visibility 会让 antd Menu 的父
	// key 与子 key 撞车（重复 key + 选中态歧义）。约定同 dashboard / pay：
	// 容器占目录路径，子项挂具体子路径。
	{ID: 93, Name: "visibility-overview", Title: "可见度概览", Icon: "aim", Path: "/visibility/overview", Component: "visibility/overview/index", ParentID: 92, Sort: 1, Status: 1, Hidden: 0, Permission: "visibility:read"},
	{ID: 94, Name: "visibility-brands", Title: "品牌与竞品", Icon: "team", Path: "/visibility/brands", Component: "visibility/brands/index", ParentID: 92, Sort: 2, Status: 1, Hidden: 0, Permission: "visibility:read"},
	{ID: 95, Name: "visibility-seo", Title: "搜索排名", Icon: "search", Path: "/visibility/seo", Component: "visibility/seo/index", ParentID: 92, Sort: 3, Status: 1, Hidden: 0, Permission: "visibility:read"},
	{ID: 96, Name: "visibility-geo", Title: "AI 提及检测", Icon: "thunderbolt", Path: "/visibility/geo", Component: "visibility/geo/index", ParentID: 92, Sort: 4, Status: 1, Hidden: 0, Permission: "visibility:read"},
	{ID: 137, Name: "visibility-geo-compare", Title: "跨引擎对比", Icon: "swap", Path: "/visibility/geo-compare", Component: "visibility/geo-compare/index", ParentID: 92, Sort: 5, Status: 1, Hidden: 0, Permission: "visibility:read"},

	// 公众号运营：作为社媒中心(107)下的一个渠道分组（三级菜单）。子菜单 87-91 仍挂 86。
	{ID: 86, Name: "mp", Title: "公众号运营", Icon: "wechat", Path: "/mp", Component: "Layout", ParentID: 107, Sort: 3, Status: 1, Hidden: 0},
	{ID: 87, Name: "mp-accounts", Title: "公众号账号", Icon: "wechat", Path: "/mp/accounts", Component: "mp/accounts/index", ParentID: 86, Sort: 1, Status: 1, Hidden: 0, Permission: "social:read"},
	{ID: 88, Name: "mp-articles", Title: "AI 文章", Icon: "edit", Path: "/mp/articles", Component: "mp/articles/index", ParentID: 86, Sort: 2, Status: 1, Hidden: 0, Permission: "social:read"},
	{ID: 89, Name: "mp-messages", Title: "消息记录", Icon: "message", Path: "/mp/messages", Component: "mp/messages/index", ParentID: 86, Sort: 3, Status: 1, Hidden: 0, Permission: "social:read"},
	{ID: 90, Name: "mp-fans", Title: "粉丝管理", Icon: "team", Path: "/mp/fans", Component: "mp/fans/index", ParentID: 86, Sort: 4, Status: 1, Hidden: 0, Permission: "social:read"},
	{ID: 91, Name: "mp-menu", Title: "自定义菜单", Icon: "menu", Path: "/mp/menu", Component: "mp/menu/index", ParentID: 86, Sort: 5, Status: 1, Hidden: 0, Permission: "social:read"},

	// 社媒中心（内容中台）：平台无关内容 + 多渠道发布/导出。微信公众号(/mp)作为其中
	// 一个渠道的运营台并存。ID 107 起（106 已被 cc-voicemail 占）。
	// 审批中心（bpm-service M1）：流程定义（管理员）+ 待办/我发起的（登录即见）
	{ID: 122, Name: "bpm", Title: "审批中心", Icon: "audit", Path: "/bpm", Component: "Layout", ParentID: 0, Sort: 12, Status: 1, Hidden: 0},
	{ID: 123, Name: "bpm-tasks", Title: "待办中心", Icon: "check", Path: "/bpm/tasks", Component: "bpm/tasks/index", ParentID: 122, Sort: 1, Status: 1, Hidden: 0},
	{ID: 124, Name: "bpm-instances", Title: "我发起的", Icon: "send", Path: "/bpm/instances", Component: "bpm/instances/index", ParentID: 122, Sort: 2, Status: 1, Hidden: 0},
	// 130：原 126 与 tenant-packages 相撞，DB 里 126 已被租户套餐占据导致本条从未种上
	{ID: 130, Name: "bpm-start", Title: "发起申请", Icon: "form", Path: "/bpm/start", Component: "bpm/start/index", ParentID: 122, Sort: 0, Status: 1, Hidden: 0},
	{ID: 125, Name: "bpm-definitions", Title: "流程定义", Icon: "fork", Path: "/bpm/definitions", Component: "bpm/definitions/index", ParentID: 122, Sort: 3, Status: 1, Hidden: 0, Permission: "bpm:definition:list"},

	// 支付中心（pay-service M1）：订单/退款/渠道/回调管理面；收银台是匿名路由不入菜单
	{ID: 127, Name: "pay", Title: "支付中心", Icon: "money-collect", Path: "/pay", Component: "Layout", ParentID: 0, Sort: 14, Status: 1, Hidden: 0},
	{ID: 128, Name: "pay-center", Title: "支付管理", Icon: "transaction", Path: "/pay/index", Component: "pay/index", ParentID: 127, Sort: 1, Status: 1, Hidden: 0, Permission: "pay:read"},

	{ID: 107, Name: "social", Title: "社媒中心", Icon: "share", Path: "/social", Component: "Layout", ParentID: 0, Sort: 13, Status: 1, Hidden: 0},
	{ID: 108, Name: "social-content", Title: "内容中心", Icon: "edit", Path: "/social/content", Component: "social/content/index", ParentID: 107, Sort: 1, Status: 1, Hidden: 0, Permission: "social:read"},
	{ID: 109, Name: "social-publish", Title: "发布中心", Icon: "share", Path: "/social/publish", Component: "social/publish/index", ParentID: 107, Sort: 2, Status: 1, Hidden: 0, Permission: "social:read"},
}
