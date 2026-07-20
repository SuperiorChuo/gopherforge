/**
 * 演示模式（VITE_DEMO=1）：给 axios 装自定义 adapter，所有 /api 请求
 * 在浏览器内存中应答——不需要后端，专供 GitHub Pages 在线演示。
 * 正常构建时 main.tsx 的动态 import 会被 Vite 静态消除，零体积影响。
 * 增删改只改内存数组，刷新即重置。
 */
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import request from '@/utils/request'

/* ---------------------------------- 工具 ---------------------------------- */

class DemoError {
  code: number
  message: string
  constructor(code: number, message: string) {
    this.code = code
    this.message = message
  }
}

const unsupported = (what: string) => {
  throw new DemoError(400, `演示模式不支持${what}，请本地 docker compose 一键启动体验完整功能`)
}

let idSeq = 1000
const nextID = () => ++idSeq

const now = () => new Date().toISOString()
const daysAgo = (n: number, h = 10) => {
  const d = new Date()
  d.setDate(d.getDate() - n)
  d.setHours(h, 24 - n, 5, 0)
  return d.toISOString()
}

function paged<T>(list: T[], query: URLSearchParams) {
  const page = Number(query.get('page') || 1)
  const size = Number(query.get('page_size') || 10)
  return { list: list.slice((page - 1) * size, page * size), total: list.length, page, page_size: size }
}

/* --------------------------------- 演示数据 -------------------------------- */

const roles = [
  { id: 1, name: '超级管理员', code: 'super_admin', description: '拥有全部权限', data_scope: 'all', created_at: daysAgo(90) },
  { id: 2, name: '运营管理员', code: 'ops_admin', description: '日常运营与内容管理', data_scope: 'dept_and_child', created_at: daysAgo(60) },
  { id: 3, name: '只读访客', code: 'viewer', description: '仅可查看', data_scope: 'self', created_at: daysAgo(30) },
]

const departments = [
  { id: 1, name: '总部', code: 'HQ', parent_id: 0, sort: 1, status: 1, leader: '管理员', created_at: daysAgo(90) },
  { id: 2, name: '技术部', code: 'TECH', parent_id: 1, sort: 1, status: 1, leader: '张工', created_at: daysAgo(90) },
  { id: 3, name: '运营部', code: 'OPS', parent_id: 1, sort: 2, status: 1, leader: '李经理', created_at: daysAgo(90) },
]

const deptTree = [{ ...departments[0], children: [departments[1], departments[2]] }]

const users = [
  { id: 1, username: 'admin', nickname: '管理员', email: 'admin@example.com', phone: '13800000001', status: 1, department_id: 1, roles: [roles[0]], created_at: daysAgo(90) },
  { id: 2, username: 'zhangsan', nickname: '张三', email: 'zhangsan@example.com', phone: '13800000002', status: 1, department_id: 2, roles: [roles[1]], created_at: daysAgo(45) },
  { id: 3, username: 'lisi', nickname: '李四', email: 'lisi@example.com', phone: '13800000003', status: 1, department_id: 3, roles: [roles[2]], created_at: daysAgo(20) },
  { id: 4, username: 'wangwu', nickname: '王五', email: 'wangwu@example.com', phone: '13800000004', status: 0, department_id: 2, roles: [roles[2]], created_at: daysAgo(7) },
]

// 与后端 menu_seed 一致的菜单树
const menuRows = [
  { id: 1, name: 'dashboard', title: '仪表盘', icon: 'dashboard', path: '/dashboard', component: 'Layout', parent_id: 0, sort: 0, status: 1, hidden: 0 },
  { id: 2, name: 'dashboard-index', title: '系统概览', icon: 'dashboard', path: '/dashboard/index', component: 'dashboard/index', parent_id: 1, sort: 1, status: 1, hidden: 0, permission: 'dashboard.view' },
  { id: 10, name: 'system', title: '系统管理', icon: 'setting', path: '/system', component: 'Layout', parent_id: 0, sort: 1, status: 1, hidden: 0 },
  { id: 11, name: 'user', title: '用户管理', icon: 'user', path: '/system/user', component: 'system/user/index', parent_id: 10, sort: 1, status: 1, hidden: 0, permission: 'system:user:list' },
  { id: 12, name: 'role', title: '角色管理', icon: 'user-safety', path: '/system/role', component: 'system/role/index', parent_id: 10, sort: 2, status: 1, hidden: 0, permission: 'system:role:list' },
  { id: 13, name: 'permission', title: '权限管理', icon: 'secured', path: '/system/permission', component: 'system/permission/index', parent_id: 10, sort: 3, status: 1, hidden: 0, permission: 'system:permission:list' },
  { id: 14, name: 'menu', title: '菜单管理', icon: 'menu', path: '/system/menu', component: 'system/menu/index', parent_id: 10, sort: 4, status: 1, hidden: 0, permission: 'system:menu:list' },
  { id: 15, name: 'department', title: '部门管理', icon: 'root-list', path: '/system/department', component: 'system/department/index', parent_id: 10, sort: 5, status: 1, hidden: 0, permission: 'system:department:list' },
  { id: 16, name: 'file', title: '文件管理', icon: 'file', path: '/system/file', component: 'system/file/index', parent_id: 10, sort: 6, status: 1, hidden: 0, permission: 'system:file:list' },
  { id: 17, name: 'dict', title: '字典管理', icon: 'data-base', path: '/system/dict', component: 'system/dict/index', parent_id: 10, sort: 7, status: 1, hidden: 0, permission: 'system:dict:list' },
  { id: 18, name: 'notice', title: '通知公告', icon: 'notification', path: '/system/notice', component: 'system/notice/index', parent_id: 10, sort: 8, status: 1, hidden: 0, permission: 'system:notice:list' },
  { id: 19, name: 'online-user', title: '在线用户', icon: 'user-list', path: '/system/online-user', component: 'system/online-user/index', parent_id: 10, sort: 9, status: 1, hidden: 0, permission: 'system:online-user:list' },
  { id: 20, name: 'operation-log', title: '操作日志', icon: 'time', path: '/system/operation-log', component: 'system/operation-log/index', parent_id: 10, sort: 10, status: 1, hidden: 0, permission: 'system:log:operation' },
  { id: 21, name: 'login-log', title: '登录日志', icon: 'time', path: '/system/login-log', component: 'system/login-log/index', parent_id: 10, sort: 11, status: 1, hidden: 0, permission: 'system:log:login' },
  { id: 22, name: 'audit-log', title: '审计日志', icon: 'secured', path: '/system/audit-log', component: 'system/audit-log/index', parent_id: 10, sort: 12, status: 1, hidden: 0, permission: 'system:log:audit' },
  { id: 23, name: 'setting', title: '系统设置', icon: 'setting', path: '/system/setting', component: 'system/setting/index', parent_id: 10, sort: 13, status: 1, hidden: 0, permission: 'system:setting:list' },
  { id: 24, name: 'tenant', title: '租户管理', icon: 'team', path: '/system/tenant', component: 'system/tenant/index', parent_id: 10, sort: 14, status: 1, hidden: 0, permission: 'system:tenant:list' },
  { id: 25, name: 'codegen', title: '代码生成', icon: 'code', path: '/system/codegen', component: 'system/codegen/index', parent_id: 10, sort: 15, status: 1, hidden: 0, permission: 'system:codegen:list' },
  { id: 30, name: 'monitor', title: '系统监控', icon: 'chart-analytics', path: '/monitor', component: 'Layout', parent_id: 0, sort: 2, status: 1, hidden: 0 },
  { id: 31, name: 'monitor-job', title: '定时任务', icon: 'time', path: '/monitor/job', component: 'monitor/job/index', parent_id: 30, sort: 1, status: 1, hidden: 0, permission: 'system:job:list' },
  { id: 32, name: 'monitor-server', title: '服务器监控', icon: 'server', path: '/monitor/server', component: 'monitor/server/index', parent_id: 30, sort: 2, status: 1, hidden: 0, permission: 'system:monitor:server' },
  { id: 33, name: 'monitor-mysql', title: '数据库监控', icon: 'data-base', path: '/monitor/mysql', component: 'monitor/mysql/index', parent_id: 30, sort: 3, status: 1, hidden: 0, permission: 'system:monitor:mysql' },
  { id: 34, name: 'monitor-redis', title: '缓存监控', icon: 'data', path: '/monitor/redis', component: 'monitor/redis/index', parent_id: 30, sort: 4, status: 1, hidden: 0, permission: 'system:monitor:redis' },
  { id: 40, name: 'profile', title: '个人中心', icon: 'user-circle', path: '/profile', component: 'Layout', parent_id: 0, sort: 99, status: 1, hidden: 1 },
  { id: 41, name: 'profile-index', title: '个人中心', icon: 'user', path: '/profile/index', component: 'profile/index', parent_id: 40, sort: 1, status: 1, hidden: 0 },
]

function menuTree() {
  const byParent = new Map<number, typeof menuRows>()
  menuRows.forEach((m) => {
    const arr = byParent.get(m.parent_id) ?? []
    arr.push(m)
    byParent.set(m.parent_id, arr)
  })
  const build = (pid: number): unknown[] =>
    (byParent.get(pid) ?? []).map((m) => ({ ...m, children: build(m.id) }))
  return build(0)
}

const permissions = menuRows
  .filter((m) => m.permission)
  .map((m, i) => ({
    id: i + 1, name: m.title, code: m.permission!, type: 2,
    path: `/api/v1/${m.name}`, method: 'GET', parent_id: 0, created_at: daysAgo(90),
  }))

const dictTypes = [
  { id: 1, name: '用户状态', code: 'user_status', status: 1, created_at: daysAgo(90) },
  { id: 2, name: '公告类型', code: 'notice_type', status: 1, created_at: daysAgo(90) },
]
const dictItems = [
  { id: 1, label: '启用', value: '1', sort: 1, status: 1, dict_type_id: 1, created_at: daysAgo(90) },
  { id: 2, label: '停用', value: '0', sort: 2, status: 1, dict_type_id: 1, created_at: daysAgo(90) },
  { id: 3, label: '通知', value: '1', sort: 1, status: 1, dict_type_id: 2, created_at: daysAgo(90) },
  { id: 4, label: '公告', value: '2', sort: 2, status: 1, dict_type_id: 2, created_at: daysAgo(90) },
]

const notices = [
  { id: 1, title: '🎉 欢迎体验 Go Admin Kit 在线演示', content: '这是纯前端演示模式：任意账号可登录，数据为浏览器内存假数据，刷新即重置。完整功能请克隆仓库后 docker compose 一键启动。', type: 2, status: 1, created_at: daysAgo(1) },
  { id: 2, title: '演示环境说明', content: '上传、导出、下载等依赖后端的动作在演示模式中被禁用。', type: 1, status: 1, created_at: daysAgo(3) },
]

const loginLogs = Array.from({ length: 23 }, (_, i) => ({
  id: 23 - i, user_id: (i % 3) + 1, username: ['admin', 'zhangsan', 'lisi'][i % 3],
  ip: `203.0.113.${10 + i}`, location: ['广东 深圳', '北京', '上海', '浙江 杭州'][i % 4],
  status: i % 7 === 3 ? 0 : 1, login_type: 1, browser: ['Chrome 126', 'Safari 17', 'Edge 125'][i % 3],
  os: ['macOS 14', 'Windows 11', 'Ubuntu 22.04'][i % 3],
  message: i % 7 === 3 ? '密码错误' : '登录成功', created_at: daysAgo(Math.floor(i / 2), 9 + (i % 12)),
}))

const operationLogs = Array.from({ length: 18 }, (_, i) => ({
  id: 18 - i, user_id: 1, username: 'admin',
  method: ['POST', 'PUT', 'DELETE', 'GET'][i % 4], path: ['/api/v1/users', '/api/v1/roles/2', '/api/v1/notices/1', '/api/v1/menus'][i % 4],
  status: i % 9 === 5 ? 500 : 200, module: ['用户管理', '角色管理', '通知公告', '菜单管理'][i % 4],
  action: ['新增', '修改', '删除', '查询'][i % 4], ip: '203.0.113.10', latency: 12 + (i % 40),
  request_id: `demo-${1000 + i}`, created_at: daysAgo(Math.floor(i / 3), 8 + (i % 10)),
}))

const auditLogs = loginLogs.slice(0, 10).map((l, i) => ({
  id: i + 1, user_id: l.user_id, username: l.username, event: i % 2 ? 'user.login' : 'user.logout',
  detail: i % 2 ? '用户登录' : '用户登出', ip: l.ip, created_at: l.created_at,
}))

const onlineUsers = [
  { user_id: 1, username: 'admin', nickname: '管理员', ip: '203.0.113.10', location: '广东 深圳', browser: 'Chrome 126', os: 'macOS 14', login_time: daysAgo(0, 9), token_id: 'demo-token-1' },
  { user_id: 2, username: 'zhangsan', nickname: '张三', ip: '203.0.113.11', location: '北京', browser: 'Edge 125', os: 'Windows 11', login_time: daysAgo(0, 10), token_id: 'demo-token-2' },
]

const files = [
  { id: 1, file_name: 'logo.png', file_path: '/uploads/logo.png', file_size: 34815, file_type: 'image/png', storage_type: 'local', user_id: 1, created_at: daysAgo(12) },
  { id: 2, file_name: '产品手册.pdf', file_path: '/uploads/manual.pdf', file_size: 2048576, file_type: 'application/pdf', storage_type: 'minio', user_id: 2, created_at: daysAgo(5) },
  { id: 3, file_name: '数据导出.xlsx', file_path: '/uploads/export.xlsx', file_size: 88123, file_type: 'application/vnd.ms-excel', storage_type: 'local', user_id: 1, created_at: daysAgo(2) },
]

const jobs = [
  { id: 1, name: 'PG 每日备份', group_name: 'ops', cron_expression: '0 3 * * *', invoke_target: 'backup_postgres', description: '全量备份到对象存储', status: 1, concurrent: 0, last_run_time: daysAgo(0, 3), next_run_time: daysAgo(-1, 3), created_at: daysAgo(60) },
  { id: 2, name: '日志轮转清理', group_name: 'ops', cron_expression: '30 4 * * 0', invoke_target: 'rotate_logs', description: '清理 30 天前日志', status: 1, concurrent: 0, last_run_time: daysAgo(2, 4), next_run_time: daysAgo(-5, 4), created_at: daysAgo(60) },
  { id: 3, name: '在线用户对账', group_name: 'system', cron_expression: '*/10 * * * *', invoke_target: 'reconcile_online', description: 'Redis 与 DB 对账', status: 0, concurrent: 1, last_run_time: daysAgo(1, 8), next_run_time: undefined, created_at: daysAgo(30) },
]

const tenants = [
  { id: 1, code: 'default', name: '默认租户', status: 1, plan: 'pro', max_users: 200, created_at: daysAgo(90), updated_at: daysAgo(3) },
]

const settings: Array<{ setting_key: string; value_json: Record<string, unknown>; updated_at?: string }> = [
  { setting_key: 'site.basic', value_json: { site_name: 'Go Admin Kit 演示站', icp: '', logo_url: '' }, updated_at: daysAgo(9) },
]

const demoUser = () => ({
  id: 1, tenant_id: 1, is_platform_admin: true, username: 'admin', nickname: '演示管理员',
  email: 'admin@example.com', phone: '13800000001', avatar: '', status: 1,
  roles: [{ id: 1, name: '超级管理员', code: 'super_admin' }],
  permissions: permissions.map((p) => p.code), totp_enabled: false, department_id: 1, created_at: daysAgo(90),
})

const captchaSVG = () => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40"><rect width="120" height="40" fill="#eef4ff"/><text x="60" y="27" font-size="22" font-family="monospace" letter-spacing="6" text-anchor="middle" fill="#3b82f6">DEMO</text></svg>`
  return `data:image/svg+xml;base64,${btoa(svg)}`
}

const serverInfo = {
  cpu: { model_name: 'Demo vCPU (4) @ 2.50GHz', cores: 4, used_percent: 23.6 },
  memory: { total: 8 * 1024 ** 3, used: 3.1 * 1024 ** 3, free: 4.9 * 1024 ** 3, used_percent: 38.7 },
  disk: { total: 120 * 1024 ** 3, used: 42 * 1024 ** 3, free: 78 * 1024 ** 3, used_percent: 35.0 },
  os: { go_os: 'linux', arch: 'amd64', compiler: 'gc', go_version: 'go1.26', num_goroutine: 86, hostname: 'demo-node-1', platform: 'debian', boot_time: '2026-07-01 08:00:00' },
}

const mysqlInfo = {
  version: 'PostgreSQL 16.3', uptime_seconds: 1728000,
  database: { host: 'postgres', port: 5432, name: 'go_admin_kit', charset: 'UTF8', collation: '', table_count: 32, size_bytes: 268435456, size: '256.0 MB' },
  connections: { max_open_conns: 50, open_conns: 8, in_use: 2, idle: 6, wait_count: 0, wait_duration: '0s', threads_connected: 8, threads_running: 2, max_connections: 100, max_used_connections: 12, total_connections: 8 },
  queries: { questions: 182340, qps: 10.5, slow_queries: 0, selects: 1203400, inserts: 5230, updates: 3120, deletes: 89 },
  traffic: { bytes_received: 734003200, bytes_sent: 5368709120, bytes_received_human: '700.0 MB', bytes_sent_human: '5.0 GB' },
}

const redisInfo = {
  server: { version: '7.2.5', os: 'Linux 6.8', mode: 'standalone', uptime: '1728000', uptime_seconds: 1728000, arch_bits: '64', process_id: 1, tcp_port: 6379 },
  memory: { used: '18.5M', peak: '24.1M', lua: '0B', fragmentation: '1.08', used_bytes: 19398656, peak_bytes: 25270272, rss: '20.0M', maxmemory: '512.0 MB', mem_allocator: 'jemalloc-5.3.0', dataset: '12.2M', overhead: '6291456' },
  stats: { connections: '6', ops: '42', keys: 1286, hit_rate: '98.6%', total_connections_received: 1830, total_commands_processed: 4203911, keyspace_hits: 402011, keyspace_misses: 5721, expired_keys: 1203, evicted_keys: 0 },
  clients: { connected: 6, blocked: 0, tracking: 0 },
  pool: { hits: 5021, misses: 12, timeouts: 0, total_conns: 10, idle_conns: 8, stale_conns: 0 },
  keyspace: { dbsize: 1286, dbs: [{ name: 'db0', keys: 1286, expires: 320 }] },
}

const codegenColumns = [
  { name: 'id', db_type: 'bigint', go_type: 'int64', ts_type: 'number', nullable: false, primary_key: true, go_field: 'ID', label: 'id' },
  { name: 'name', db_type: 'varchar', go_type: 'string', ts_type: 'string', nullable: false, primary_key: false, go_field: 'Name', label: 'name' },
  { name: 'amount_cents', db_type: 'bigint', go_type: 'int64', ts_type: 'number', nullable: true, primary_key: false, go_field: 'AmountCents', label: 'amount_cents' },
  { name: 'active', db_type: 'boolean', go_type: 'bool', ts_type: 'boolean', nullable: true, primary_key: false, go_field: 'Active', label: 'active' },
  { name: 'created_at', db_type: 'timestamptz', go_type: 'time.Time', ts_type: 'string', nullable: true, primary_key: false, go_field: 'CreatedAt', label: 'created_at' },
  { name: 'updated_at', db_type: 'timestamptz', go_type: 'time.Time', ts_type: 'string', nullable: true, primary_key: false, go_field: 'UpdatedAt', label: 'updated_at' },
]

const codegenPreview = (module: string) => ({
  files: [
    { path: `server/${module}/model.go`, content: `package ${module}\n\nimport "time"\n\n// Demo 演示：真实环境会按你选的表和字段实时渲染。\ntype Item struct {\n\tID uint64 \`gorm:"primaryKey" json:"id"\`\n\tName string \`gorm:"column:name" json:"name"\`\n\tAmountCents int64 \`gorm:"column:amount_cents" json:"amount_cents"\`\n\tActive bool \`gorm:"column:active" json:"active"\`\n\tCreatedAt time.Time \`json:"created_at"\`\n\tUpdatedAt time.Time \`json:"updated_at"\`\n}\n` },
    { path: `server/${module}/store.go`, content: `package ${module}\n\n// 演示模式为静态示例；本地运行时由后端模板按所选字段实时生成\n// List/Get/Create/Update/Delete 五件套与关键字搜索。\n` },
    { path: `server/${module}/handlers.go`, content: `package ${module}\n\n// gin handlers：List/Create/Update/Delete，{code,message,data} 信封。\n` },
    { path: `web/src/pages/${module}/index.tsx`, content: `// React 列表页：筛选卡片 + 表格 + 弹窗表单，风格与本站一致。\n` },
    { path: `menu-${module}.sql`, content: `-- 菜单 seed 示例\nINSERT INTO menus (name, title, path, component, ...) VALUES ('${module}', '演示模块', '/${module}', '${module}/index', ...);\n` },
  ],
})

/* --------------------------------- 路由表 --------------------------------- */

type Handler = (m: RegExpMatchArray, body: Record<string, unknown>, query: URLSearchParams, cfg: InternalAxiosRequestConfig) => unknown

const routes: Array<[string, RegExp, Handler]> = [
  // 认证
  ['get', /^\/api\/v1\/captcha$/, () => ({ key: `demo-${Date.now()}`, type: 'image', image: captchaSVG(), width: 120, height: 40 })],
  ['post', /^\/api\/v1\/login$/, (_m, body) => ({
    access_token: 'demo-access-token', refresh_token: 'demo-refresh-token',
    user: { ...demoUser(), username: String(body.username || 'admin') || 'admin' },
  })],
  ['post', /^\/api\/v1\/refresh$/, () => ({ access_token: 'demo-access-token', refresh_token: 'demo-refresh-token' })],
  ['post', /^\/api\/v1\/logout$/, () => ({})],
  ['get', /^\/api\/v1\/user\/me$/, () => demoUser()],
  ['get', /^\/api\/v1\/user\/menus$/, () => menuTree()],
  ['put', /^\/api\/v1\/user\/profile$/, () => demoUser()],
  ['put', /^\/api\/v1\/user\/password$/, () => unsupported('修改密码')],
  ['post', /^\/api\/v1\/user\/2fa\/\w+/, () => unsupported('两步验证配置')],
  ['post', /^\/api\/v1\/ws\/notifications\/ticket$/, () => ({ ticket: 'demo-ticket' })],

  // 用户
  ['get', /^\/api\/v1\/users$/, (_m, _b, q) => paged(users, q)],
  ['get', /^\/api\/v1\/users\/(\d+)$/, (m) => users.find((u) => u.id === Number(m[1])) ?? users[0]],
  ['post', /^\/api\/v1\/users$/, (_m, body) => {
    const u = { id: nextID(), status: 1, roles: [roles[2]], created_at: now(), ...body } as (typeof users)[0]
    users.unshift(u)
    return u
  }],
  ['put', /^\/api\/v1\/users\/(\d+)\/status$/, (m, body) => {
    const u = users.find((x) => x.id === Number(m[1]))
    if (u) u.status = Number(body.status ?? u.status)
    return u ?? {}
  }],
  ['post', /^\/api\/v1\/users\/(\d+)\/roles$/, () => ({})],
  ['put', /^\/api\/v1\/users\/(\d+)$/, (m, body) => {
    const i = users.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) users[i] = { ...users[i], ...body }
    return users[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/users\/(\d+)$/, (m) => {
    const i = users.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) users.splice(i, 1)
    return {}
  }],

  // 角色 / 权限
  ['get', /^\/api\/v1\/roles\/all$/, () => ({ list: roles, total: roles.length })],
  ['get', /^\/api\/v1\/roles$/, (_m, _b, q) => paged(roles, q)],
  ['get', /^\/api\/v1\/roles\/(\d+)$/, (m) => roles.find((r) => r.id === Number(m[1])) ?? roles[0]],
  ['post', /^\/api\/v1\/roles$/, (_m, body) => {
    const r = { id: nextID(), data_scope: 'self', created_at: now(), ...body } as (typeof roles)[0]
    roles.push(r)
    return r
  }],
  ['post', /^\/api\/v1\/roles\/(\d+)\/permissions$/, () => ({})],
  ['put', /^\/api\/v1\/roles\/(\d+)$/, (m, body) => {
    const i = roles.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) roles[i] = { ...roles[i], ...body }
    return roles[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/roles\/(\d+)$/, (m) => {
    const i = roles.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) roles.splice(i, 1)
    return {}
  }],
  ['get', /^\/api\/v1\/permissions\/tree$/, () => ({ list: permissions, total: permissions.length })],
  ['get', /^\/api\/v1\/permissions$/, (_m, _b, q) => paged(permissions, q)],
  ['get', /^\/api\/v1\/permissions\/(\d+)$/, (m) => permissions.find((p) => p.id === Number(m[1])) ?? permissions[0]],
  ['post', /^\/api\/v1\/permissions$/, () => unsupported('新增权限点')],
  ['put', /^\/api\/v1\/permissions\/(\d+)$/, () => unsupported('修改权限点')],
  ['delete', /^\/api\/v1\/permissions\/(\d+)$/, () => unsupported('删除权限点')],

  // 菜单 / 部门
  // tree 接口前端契约同样是裸数组
  ['get', /^\/api\/v1\/menus\/tree$/, () => menuTree()],
  ['get', /^\/api\/v1\/menus$/, () => ({ list: menuTree(), total: menuRows.length })],
  ['get', /^\/api\/v1\/menus\/(\d+)$/, (m) => menuRows.find((x) => x.id === Number(m[1])) ?? menuRows[0]],
  ['post', /^\/api\/v1\/menus$/, () => unsupported('新增菜单')],
  ['put', /^\/api\/v1\/menus\/(\d+)$/, () => unsupported('修改菜单')],
  ['delete', /^\/api\/v1\/menus\/(\d+)$/, () => unsupported('删除菜单')],
  ['get', /^\/api\/v1\/departments\/tree$/, () => deptTree],
  ['get', /^\/api\/v1\/departments\/all$/, () => ({ list: departments, total: departments.length })],
  ['get', /^\/api\/v1\/departments$/, (_m, _b, q) => paged(departments, q)],
  ['get', /^\/api\/v1\/departments\/(\d+)$/, (m) => departments.find((x) => x.id === Number(m[1])) ?? departments[0]],
  ['post', /^\/api\/v1\/departments$/, () => unsupported('新增部门')],
  ['put', /^\/api\/v1\/departments\/(\d+)$/, () => unsupported('修改部门')],
  ['delete', /^\/api\/v1\/departments\/(\d+)$/, () => unsupported('删除部门')],

  // 字典 / 公告 / 文件
  ['get', /^\/api\/v1\/dict-types\/all$/, () => ({ list: dictTypes, total: dictTypes.length })],
  ['get', /^\/api\/v1\/dict-types$/, (_m, _b, q) => paged(dictTypes, q)],
  ['get', /^\/api\/v1\/dict-types\/(\d+)\/items$/, (m) => ({ list: dictItems.filter((d) => d.dict_type_id === Number(m[1])), total: 2 })],
  ['get', /^\/api\/v1\/dict-types\/(\d+)$/, (m) => dictTypes.find((x) => x.id === Number(m[1])) ?? dictTypes[0]],
  ['post', /^\/api\/v1\/dict-types$/, (_m, body) => {
    const d = { id: nextID(), status: 1, created_at: now(), ...body } as (typeof dictTypes)[0]
    dictTypes.push(d)
    return d
  }],
  ['put', /^\/api\/v1\/dict-types\/(\d+)$/, (m, body) => {
    const i = dictTypes.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) dictTypes[i] = { ...dictTypes[i], ...body }
    return dictTypes[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/dict-types\/(\d+)$/, (m) => {
    const i = dictTypes.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) dictTypes.splice(i, 1)
    return {}
  }],
  ['get', /^\/api\/v1\/dict-items$/, (_m, _b, q) => {
    const tid = Number(q.get('dict_type_id') || 0)
    return paged(tid ? dictItems.filter((d) => d.dict_type_id === tid) : dictItems, q)
  }],
  ['get', /^\/api\/v1\/dicts\/?/, () => ({})],
  ['post', /^\/api\/v1\/dict-items$/, (_m, body) => {
    const d = { id: nextID(), sort: 99, status: 1, created_at: now(), ...body } as (typeof dictItems)[0]
    dictItems.push(d)
    return d
  }],
  ['put', /^\/api\/v1\/dict-items\/(\d+)$/, (m, body) => {
    const i = dictItems.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) dictItems[i] = { ...dictItems[i], ...body }
    return dictItems[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/dict-items\/(\d+)$/, (m) => {
    const i = dictItems.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) dictItems.splice(i, 1)
    return {}
  }],
  // 前端契约是裸数组（见 src/api/system/notice.ts），包 {list} 会让仪表盘崩进错误边界
  ['get', /^\/api\/v1\/notices\/active$/, () => notices.filter((n) => n.status === 1)],
  ['get', /^\/api\/v1\/notices$/, (_m, _b, q) => paged(notices, q)],
  ['get', /^\/api\/v1\/notices\/(\d+)$/, (m) => notices.find((x) => x.id === Number(m[1])) ?? notices[0]],
  ['post', /^\/api\/v1\/notices$/, (_m, body) => {
    const n = { id: nextID(), type: 1, status: 1, created_at: now(), ...body } as (typeof notices)[0]
    notices.unshift(n)
    return n
  }],
  ['put', /^\/api\/v1\/notices\/(\d+)\/status$/, (m, body) => {
    const n = notices.find((x) => x.id === Number(m[1]))
    if (n) n.status = Number(body.status ?? n.status)
    return n ?? {}
  }],
  ['put', /^\/api\/v1\/notices\/(\d+)$/, (m, body) => {
    const i = notices.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) notices[i] = { ...notices[i], ...body }
    return notices[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/notices\/(\d+)$/, (m) => {
    const i = notices.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) notices.splice(i, 1)
    return {}
  }],
  ['get', /^\/api\/v1\/files\/stats$/, () => ({ total_count: files.length, total_size: files.reduce((s, f) => s + f.file_size, 0) })],
  ['get', /^\/api\/v1\/files\/my$/, (_m, _b, q) => paged(files, q)],
  ['get', /^\/api\/v1\/files$/, (_m, _b, q) => paged(files, q)],
  ['post', /^\/api\/v1\/files\/upload/, () => unsupported('文件上传')],
  ['delete', /^\/api\/v1\/files/, () => unsupported('删除文件')],
  ['get', /^\/api\/v1\/files\/\d+/, () => files[0]],

  // 日志 / 在线用户
  ['get', /^\/api\/v1\/login-logs\/last$/, () => loginLogs[0]],
  ['get', /^\/api\/v1\/login-logs\/trend$/, (_m, _b, q) => {
    const days = Number(q.get('days') || 14)
    return Array.from({ length: days }, (_, i) => {
      const d = new Date()
      d.setDate(d.getDate() - (days - 1 - i))
      const count = 6 + ((i * 7) % 12)
      const failed = i % 4 === 2 ? 2 : 0
      return { date: d.toISOString().slice(0, 10), count, success: count - failed, failed }
    })
  }],
  ['get', /^\/api\/v1\/login-logs\/stats$/, () => ({ total: loginLogs.length, success: loginLogs.filter((l) => l.status === 1).length, failed: loginLogs.filter((l) => l.status === 0).length })],
  ['get', /^\/api\/v1\/login-logs\/my$/, (_m, _b, q) => paged(loginLogs.filter((l) => l.username === 'admin'), q)],
  ['get', /^\/api\/v1\/login-logs$/, (_m, _b, q) => paged(loginLogs, q)],
  ['delete', /^\/api\/v1\/login-logs\/clear$/, () => unsupported('清空日志')],
  ['get', /^\/api\/v1\/operation-logs\/stats$/, () => ({ total: operationLogs.length, error_count: operationLogs.filter((l) => l.status >= 400).length })],
  ['get', /^\/api\/v1\/operation-logs\/export$/, () => unsupported('导出')],
  ['get', /^\/api\/v1\/operation-logs\/(\d+)$/, (m) => operationLogs.find((x) => x.id === Number(m[1])) ?? operationLogs[0]],
  ['get', /^\/api\/v1\/operation-logs$/, (_m, _b, q) => paged(operationLogs, q)],
  ['delete', /^\/api\/v1\/operation-logs\/clear$/, () => unsupported('清空日志')],
  ['get', /^\/api\/v1\/logs\/audit$/, (_m, _b, q) => paged(auditLogs, q)],
  ['get', /^\/api\/v1\/online-users\/count$/, () => ({ count: onlineUsers.length })],
  ['get', /^\/api\/v1\/online-users$/, (_m, _b, q) => paged(onlineUsers, q)],
  ['delete', /^\/api\/v1\/online-users\//, () => unsupported('强制下线')],

  // 设置 / 租户 / 天气
  ['get', /^\/api\/v1\/system-settings$/, () => settings],
  ['get', /^\/api\/v1\/system-settings\/([^/]+)$/, (m) => settings.find((s) => s.setting_key === decodeURIComponent(m[1])) ?? { setting_key: decodeURIComponent(m[1]), value_json: {} }],
  ['put', /^\/api\/v1\/system-settings\/([^/]+)$/, (m, body) => {
    const key = decodeURIComponent(m[1])
    const i = settings.findIndex((s) => s.setting_key === key)
    const row = { setting_key: key, value_json: (body.value_json as Record<string, unknown>) ?? {}, updated_at: now() }
    if (i >= 0) settings[i] = row
    else settings.push(row)
    return row
  }],
  ['post', /^\/api\/v1\/system-settings\/batch$/, () => ({})],
  ['delete', /^\/api\/v1\/system-settings\//, () => ({})],
  ['get', /^\/api\/v1\/tenants$/, (_m, _b, q) => paged(tenants, q)],
  ['get', /^\/api\/v1\/tenants\/(\d+)$/, () => tenants[0]],
  ['post', /^\/api\/v1\/tenants$/, () => unsupported('新增租户')],
  ['put', /^\/api\/v1\/tenants\//, () => unsupported('修改租户')],
  ['get', /^\/api\/v1\/system\/weather$/, () => ({ city: '深圳市', adcode: '440300', weather: '晴', temperature: '27', humidity: '62', wind_dir: '东南', wind_power: '≤3', report_time: now().slice(0, 19).replace('T', ' '), temp_high: '30', temp_low: '25' })],

  // 监控
  ['get', /^\/api\/v1\/monitor\/server$/, () => serverInfo],
  ['get', /^\/api\/v1\/monitor\/mysql$/, () => mysqlInfo],
  ['get', /^\/api\/v1\/monitor\/redis$/, () => redisInfo],
  ['get', /^\/api\/v1\/monitor\/jobs\/health$/, () => ({ total: jobs.length, enabled: jobs.filter((j) => j.status === 1).length, paused: jobs.filter((j) => j.status !== 1).length, recent_failed: 0, last_run_time: jobs[0].last_run_time, window_hours: 24 })],
  ['get', /^\/api\/v1\/monitor\/jobs$/, (_m, _b, q) => paged(jobs, q)],
  ['post', /^\/api\/v1\/monitor\/jobs\/(\d+)\/(start|stop)$/, (m) => {
    const j = jobs.find((x) => x.id === Number(m[1]))
    if (j) j.status = m[2] === 'start' ? 1 : 0
    return {}
  }],
  ['post', /^\/api\/v1\/monitor\/jobs\/(\d+)\/run$/, (m) => {
    const j = jobs.find((x) => x.id === Number(m[1]))
    if (j) j.last_run_time = now()
    return {}
  }],
  ['post', /^\/api\/v1\/monitor\/jobs$/, (_m, body) => {
    const j = { id: nextID(), status: 1, created_at: now(), ...body } as (typeof jobs)[0]
    jobs.push(j)
    return j
  }],
  ['put', /^\/api\/v1\/monitor\/jobs\/(\d+)$/, (m, body) => {
    const i = jobs.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) jobs[i] = { ...jobs[i], ...body }
    return jobs[i] ?? {}
  }],
  ['delete', /^\/api\/v1\/monitor\/jobs\/(\d+)$/, (m) => {
    const i = jobs.findIndex((x) => x.id === Number(m[1]))
    if (i >= 0) jobs.splice(i, 1)
    return {}
  }],
  ['post', /^\/api\/v1\/monitor\/job-logs\/cleanup$/, () => ({ deleted_rows: 128 })],

  // 代码生成器
  ['get', /^\/api\/v1\/codegen\/tables$/, () => ({ list: [{ name: 'demo_assets' }, { name: 'demo_orders' }, { name: 'users' }, { name: 'roles' }, { name: 'menus' }], total: 5 })],
  ['get', /^\/api\/v1\/codegen\/tables\/[^/]+\/columns$/, () => ({ list: codegenColumns, total: codegenColumns.length })],
  ['post', /^\/api\/v1\/codegen\/preview$/, (_m, body) => codegenPreview(String(body.module || 'demo'))],
  ['post', /^\/api\/v1\/codegen\/download$/, () => unsupported('zip 下载')],
]

/* --------------------------------- adapter -------------------------------- */

export function installDemoAdapter() {
  request.defaults.adapter = async (config: InternalAxiosRequestConfig): Promise<AxiosResponse> => {
    const raw = config.url || ''
    const [path, qs] = raw.split('?')
    const query = new URLSearchParams(qs || '')
    // axios 会把 params 单独给；合并进 query
    if (config.params) {
      Object.entries(config.params as Record<string, unknown>).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== '') query.set(k, String(v))
      })
    }
    const method = (config.method || 'get').toLowerCase()
    let body: Record<string, unknown> = {}
    if (typeof config.data === 'string') {
      try {
        body = JSON.parse(config.data)
      } catch {
        body = {}
      }
    } else if (config.data && typeof config.data === 'object') {
      body = config.data as Record<string, unknown>
    }

    // 模拟一点网络延迟，让 loading 态可见
    await new Promise((r) => setTimeout(r, 120 + Math.random() * 180))

    const respond = (code: number, message: string, data: unknown): AxiosResponse => ({
      data: { code, message, data },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })

    for (const [m, re, handler] of routes) {
      if (m !== method) continue
      const match = path.match(re)
      if (!match) continue
      try {
        return respond(200, 'success', handler(match, body, query, config))
      } catch (e) {
        if (e instanceof DemoError) return respond(e.code, e.message, null)
        throw e
      }
    }

    // 未覆盖的读接口回空列表，写接口礼貌拒绝——保证任何页面都不会挂
    if (method === 'get') return respond(200, 'success', { list: [], total: 0 })
    return respond(400, '演示模式暂不支持该操作', null)
  }
}
