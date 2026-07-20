import { useEffect, useMemo, useState } from 'react'
import { Outlet, Navigate, useNavigate, useLocation } from 'react-router-dom'
import {
  Layout,
  Menu,
  Avatar,
  Dropdown,
  Space,
  Breadcrumb,
  Spin,
  Modal,
  Form,
  Input,
  type MenuProps,
} from 'antd'
import {
  ApiOutlined,
  AudioOutlined,
  ControlOutlined,
  ForkOutlined,
  SoundOutlined,
  DashboardOutlined,
  UserOutlined,
  TeamOutlined,
  SafetyOutlined,
  MenuOutlined,
  ApartmentOutlined,
  DatabaseOutlined,
  FileOutlined,
  LoginOutlined,
  FileTextOutlined,
  NotificationOutlined,
  SettingOutlined,
  StopOutlined,
  MonitorOutlined,
  CloudServerOutlined,
  LogoutOutlined,
  LockOutlined,
  ScheduleOutlined,
  BarsOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  SearchOutlined,
  AimOutlined,
  ThunderboltOutlined,
  SunOutlined,
  MoonOutlined,
  HomeOutlined,
  VerticalAlignTopOutlined,
  ColumnHeightOutlined,
  BookOutlined,
  GlobalOutlined,
  PhoneOutlined,
  FundOutlined,
  RadarChartOutlined,
  VideoCameraOutlined,
  MailOutlined,
  ClockCircleOutlined,
  BellOutlined,
  BarChartOutlined,
  PayCircleOutlined,
  SafetyCertificateOutlined,
  DesktopOutlined,
  AppstoreOutlined,
  EditOutlined,
  ShareAltOutlined,
} from '@ant-design/icons'
import type { MenuItem as ApiMenuItem } from '@/types'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, logout } from '@/store/slices/authSlice'
import { getToken } from '@/utils/request'
import { usePermission } from '@/hooks/usePermission'
import { ROUTE_PERMISSIONS } from '@/router/route-permissions'
import { changePassword } from '@/api/auth'
import { message } from '@/utils/feedback'
import NotificationBell from '@/components/NotificationBell'
import ErrorBoundary from '@/components/ErrorBoundary'
import CommandPalette, { type PaletteItem } from '@/components/CommandPalette'
import { useThemeMode } from '@/theme/ThemeContext'

const { Header, Sider, Content } = Layout

type MenuItem2 = Required<MenuProps>['items'][number]

function makeItem(
  label: React.ReactNode,
  key: string,
  icon?: React.ReactNode,
  children?: MenuItem2[],
): MenuItem2 {
  return { label, key, icon, children } as MenuItem2
}

interface MenuDef {
  label: string
  key: string
  icon: React.ReactNode
  perm?: string
  children?: MenuDef[]
}

// 后端菜单 icon 字段（字符串）→ antd 图标
const ICON_MAP: Record<string, React.ReactNode> = {
  dashboard: <DashboardOutlined />,
  setting: <SettingOutlined />,
  user: <UserOutlined />,
  'user-safety': <TeamOutlined />,
  secured: <SafetyOutlined />,
  menu: <MenuOutlined />,
  'root-list': <ApartmentOutlined />,
  file: <FileOutlined />,
  'data-base': <DatabaseOutlined />,
  notification: <NotificationOutlined />,
  'user-list': <MonitorOutlined />,
  time: <ScheduleOutlined />,
  'chart-analytics': <CloudServerOutlined />,
  server: <CloudServerOutlined />,
  data: <BarsOutlined />,
  'user-circle': <UserOutlined />,
  team: <TeamOutlined />,
  book: <BookOutlined />,
  global: <GlobalOutlined />,
  phone: <PhoneOutlined />,
  control: <ControlOutlined />,
  sound: <SoundOutlined />,
  gateway: <ApiOutlined />,
  fork: <ForkOutlined />,
  fund: <FundOutlined />,
  queue: <TeamOutlined />,
  audio: <AudioOutlined />,
  stop: <StopOutlined />,
  radar: <RadarChartOutlined />,
  video: <VideoCameraOutlined />,
  mail: <MailOutlined />,
  clock: <ClockCircleOutlined />,
  bell: <BellOutlined />,
  chart: <BarChartOutlined />,
  money: <PayCircleOutlined />,
  safety: <SafetyCertificateOutlined />,
  desktop: <DesktopOutlined />,
  edit: <EditOutlined />,
  share: <ShareAltOutlined />,
  aim: <AimOutlined />,
  search: <SearchOutlined />,
  thunderbolt: <ThunderboltOutlined />,
}

function iconOf(name?: string): React.ReactNode {
  return (name && ICON_MAP[name]) || <AppstoreOutlined />
}

// /user/menus 树 → 侧栏定义。后端已按权限过滤，这里只做展示映射。
function apiMenusToDefs(menus: ApiMenuItem[], topLevel = true): MenuDef[] {
  return [...menus]
    .filter((m) => m.hidden !== 1)
    .sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0))
    .map((m): MenuDef | null => {
      const kids = m.children?.length ? apiMenusToDefs(m.children, false) : []
      const isContainer = (m.children?.length ?? 0) > 0 || m.component === 'Layout'
      if (isContainer) {
        if (kids.length === 0) return null
        // 单子项容器折叠为叶子（如"仪表盘"Layout + 唯一 index 页），跳容器自身路径
        if (kids.length === 1 && topLevel) {
          return { label: m.title, key: m.path, icon: iconOf(m.icon), perm: kids[0].perm }
        }
        return { label: m.title, key: m.path, icon: iconOf(m.icon), children: kids }
      }
      return { label: m.title, key: m.path, icon: iconOf(m.icon), perm: m.permission || undefined }
    })
    .filter((d): d is MenuDef => d !== null)
}

// 后端菜单为空（未播种/请求失败）时的静态兜底，与路由表保持一致
const MENU_DEFS: MenuDef[] = [
  { label: '仪表盘', key: '/dashboard', icon: <DashboardOutlined /> },
  {
    label: '系统管理',
    key: '/system',
    icon: <SettingOutlined />,
    children: [
      { label: '用户管理', key: '/system/user', icon: <UserOutlined /> },
      { label: '角色管理', key: '/system/role', icon: <TeamOutlined /> },
      { label: '权限管理', key: '/system/permission', icon: <SafetyOutlined /> },
      { label: '菜单管理', key: '/system/menu', icon: <MenuOutlined /> },
      { label: '部门管理', key: '/system/department', icon: <ApartmentOutlined /> },
      { label: '字典管理', key: '/system/dict', icon: <DatabaseOutlined /> },
      { label: '文件管理', key: '/system/file', icon: <FileOutlined /> },
      { label: '公告管理', key: '/system/notice', icon: <NotificationOutlined /> },
      { label: '登录日志', key: '/system/login-log', icon: <LoginOutlined /> },
      { label: '操作日志', key: '/system/operation-log', icon: <FileTextOutlined /> },
      { label: '审计日志', key: '/system/audit-log', icon: <SafetyOutlined /> },
      { label: '在线用户', key: '/system/online-user', icon: <MonitorOutlined /> },
      { label: '系统设置', key: '/system/setting', icon: <SettingOutlined /> },
      { label: '租户管理', key: '/system/tenant', icon: <TeamOutlined /> },
    ],
  },
  {
    label: '运维监控',
    key: '/monitor',
    icon: <CloudServerOutlined />,
    children: [
      { label: '服务器监控', key: '/monitor/server', icon: <CloudServerOutlined /> },
      { label: '数据库监控', key: '/monitor/mysql', icon: <DatabaseOutlined /> },
      { label: 'Redis 监控', key: '/monitor/redis', icon: <BarsOutlined /> },
      { label: '定时任务', key: '/monitor/job', icon: <ScheduleOutlined /> },
    ],
  },
]

// 叶子可见性：菜单自带权限码优先，否则回落到路由权限表；两者都无则登录即可见
function leafVisible(d: MenuDef, hasPerm: (code?: string) => boolean): boolean {
  return hasPerm(d.perm ?? ROUTE_PERMISSIONS[d.key])
}

function buildMenuItems(defs: MenuDef[], hasPerm: (code?: string) => boolean): MenuItem2[] {
  return defs
    .map((d) => {
      if (d.children) {
        const children = buildMenuItems(d.children, hasPerm)
        return children.length > 0 ? makeItem(d.label, d.key, d.icon, children) : null
      }
      return leafVisible(d, hasPerm) ? makeItem(d.label, d.key, d.icon) : null
    })
    .filter((item): item is MenuItem2 => item !== null)
}

// 命令面板数据：与菜单同源、同权限过滤
function buildPaletteItems(defs: MenuDef[], hasPerm: (code?: string) => boolean): PaletteItem[] {
  const result: PaletteItem[] = []
  const walk = (nodes: MenuDef[], group: string) => {
    nodes.forEach((d) => {
      if (d.children) {
        walk(d.children, d.label)
      } else if (leafVisible(d, hasPerm)) {
        result.push({ label: d.label, path: d.key, group, icon: d.icon })
      }
    })
  }
  walk(defs, '导航')
  result.push({ label: '个人中心', path: '/profile', group: '导航', icon: <UserOutlined /> })
  return result
}

const pathBreadcrumbMap: Record<string, string> = {
  '/dashboard': '仪表盘',
  '/profile': '个人中心',
  '/system/user': '用户管理',
  '/system/role': '角色管理',
  '/system/permission': '权限管理',
  '/system/menu': '菜单管理',
  '/system/department': '部门管理',
  '/system/dict': '字典管理',
  '/system/file': '文件管理',
  '/system/notice': '公告管理',
  '/system/login-log': '登录日志',
  '/system/operation-log': '操作日志',
  '/system/audit-log': '审计日志',
  '/system/online-user': '在线用户',
  '/system/setting': '系统设置',
  '/system/tenant': '租户管理',
  '/monitor/server': '服务器监控',
  '/monitor/mysql': '数据库监控',
  '/monitor/redis': 'Redis 监控',
  '/monitor/job': '定时任务',
}

// 分组 key（含前导 /）→ 面包屑上的分组名和图标
const GROUP_META: Record<string, { label: string; icon: React.ReactNode }> = {
  '/system': { label: '系统管理', icon: <SettingOutlined /> },
  '/monitor': { label: '运维监控', icon: <CloudServerOutlined /> },
}

export default function MainLayout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  const { userInfo, loading, permissions, menus } = useAppSelector((s) => s.auth)
  const { hasPerm, isSuperAdmin } = usePermission()
  const { mode, toggle: toggleTheme } = useThemeMode()
  const token = getToken()
  // super_admin 可不依赖 permissions 列表；普通用户需要 permissions 才渲染侧栏
  const authReady = !!userInfo && (isSuperAdmin || permissions.length > 0 || !Object.keys(ROUTE_PERMISSIONS).length)

  // 侧栏以后端 /user/menus 为准（菜单管理页的增删改即时生效）；为空时回落到静态定义
  const menuDefs = useMemo(() => {
    const dynamic = apiMenusToDefs(menus)
    return dynamic.length > 0 ? dynamic : MENU_DEFS
  }, [menus])
  const menuItems = useMemo(() => buildMenuItems(menuDefs, hasPerm), [menuDefs, hasPerm])
  const paletteItems = useMemo(() => buildPaletteItems(menuDefs, hasPerm), [menuDefs, hasPerm])
  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform)

  const [collapsed, setCollapsed] = useState(false)
  const [isMobile, setIsMobile] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [showBackTop, setShowBackTop] = useState(false)
  // 表格密度:comfortable(默认)/compact,写在 html[data-density] 上由 CSS 消费
  const [density, setDensity] = useState<'comfortable' | 'compact'>(
    () => (localStorage.getItem('app_density') === 'compact' ? 'compact' : 'comfortable'),
  )

  useEffect(() => {
    document.documentElement.dataset.density = density
    localStorage.setItem('app_density', density)
  }, [density])
  const [forcePwdSubmitting, setForcePwdSubmitting] = useState(false)
  const [forcePwdForm] = Form.useForm()
  const pathname = location.pathname

  useEffect(() => {
    const onFsChange = () => setFullscreen(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', onFsChange)
    return () => document.removeEventListener('fullscreenchange', onFsChange)
  }, [])

  // 长页滚动后浮出"回到顶部"玻璃钮
  useEffect(() => {
    const onScroll = () => setShowBackTop(window.scrollY > 480)
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  const toggleFullscreen = () => {
    if (document.fullscreenElement) {
      document.exitFullscreen()
    } else {
      document.documentElement.requestFullscreen().catch(() => {
        // 部分环境（iframe 等）不允许全屏，静默忽略
      })
    }
  }
  // 分组 key 即父级路径：/system/user → /system
  const groupOf = (p: string) => {
    const seg = p.split('/')[1]
    return seg && GROUP_META[`/${seg}`] ? `/${seg}` : null
  }
  const [openKeys, setOpenKeys] = useState<string[]>(() => {
    const g = groupOf(pathname)
    return g ? [g] : []
  })

  // 直达子路由（面包屑/外部跳转）时自动展开所属分组
  useEffect(() => {
    const group = groupOf(pathname)
    if (group) {
      setOpenKeys((keys) => (keys.includes(group) ? keys : [...keys, group]))
    }
  }, [pathname])

  useEffect(() => {
    if (!token) {
      navigate('/login', { replace: true })
      return
    }
    // 有 token 但用户/权限未就绪时拉取；避免 login 只写了 userInfo、permissions 仍为空
    if (!authReady) {
      dispatch(fetchCurrentUser())
    }
  }, [token, authReady, dispatch, navigate])

  if (!token) return null

  if (!userInfo) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spin size="large" />
      </div>
    )
  }

  const handleLogout = async () => {
    await dispatch(logout())
    navigate('/login', { replace: true })
  }

  const handleForcePwdSubmit = async () => {
    const values = await forcePwdForm.validateFields().catch(() => null)
    if (!values) return
    setForcePwdSubmitting(true)
    try {
      await changePassword({ old_password: values.old_password, new_password: values.new_password })
      message.success('密码修改成功')
      forcePwdForm.resetFields()
      dispatch(fetchCurrentUser())
    } catch {
      message.error('密码修改失败，请检查当前密码是否正确')
    } finally {
      setForcePwdSubmitting(false)
    }
  }

  const roleText =
    userInfo.roles && userInfo.roles.length > 0
      ? userInfo.roles.map((r) => r.name).join(' · ')
      : ''

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'userinfo',
      disabled: true,
      className: 'user-drop-info-item',
      style: { cursor: 'default', height: 'auto', lineHeight: 'inherit' },
      label: (
        <div className="user-drop-head">
          <div className="user-drop-name">{userInfo.nickname || userInfo.username}</div>
          <div className="user-drop-meta">
            {userInfo.email || userInfo.username}
            {roleText ? ` · ${roleText}` : ''}
          </div>
        </div>
      ),
    },
    { type: 'divider' },
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人中心',
      onClick: () => navigate('/profile'),
    },
    {
      key: 'password',
      icon: <LockOutlined />,
      label: '修改密码',
      onClick: () => navigate('/profile'),
    },
    { type: 'divider' },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
      danger: true,
    },
  ]

  const currentPath = location.pathname
  // 路由级守卫：无权限直接进 403（userInfo 已就绪，permissions 一定已加载）
  const requiredPerm = ROUTE_PERMISSIONS[currentPath]
  if (requiredPerm && !hasPerm(requiredPerm)) {
    return <Navigate to="/403" replace />
  }
  const breadcrumbTitle = pathBreadcrumbMap[currentPath] || ''
  const groupKey = groupOf(currentPath)
  const groupMeta = groupKey ? GROUP_META[groupKey] : null

  const breadcrumbItems = [
    {
      title: (
        <button
          type="button"
          className="app-bc-link"
          onClick={() => navigate('/dashboard')}
          title="回到仪表盘"
        >
          <HomeOutlined />
          <span>首页</span>
        </button>
      ),
    },
    ...(groupMeta
      ? [
          {
            title: (
              <span className="app-bc-mid">
                {groupMeta.icon}
                <span>{groupMeta.label}</span>
              </span>
            ),
          },
        ]
      : []),
    ...(breadcrumbTitle
      ? [
          {
            title: <span className="app-bc-current">{breadcrumbTitle}</span>,
          },
        ]
      : []),
  ]

  return (
    <Layout className={`app-shell${isMobile ? ' is-mobile' : ''}${!collapsed && isMobile ? ' is-sider-open' : ''}`} hasSider>
      {isMobile && !collapsed && (
        <div
          className="app-sider-mask"
          aria-hidden
          onClick={() => setCollapsed(true)}
        />
      )}
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        trigger={null}
        width={224}
        collapsedWidth={isMobile ? 0 : 80}
        breakpoint="lg"
        onBreakpoint={(broken) => {
          setIsMobile(broken)
          setCollapsed(broken)
        }}
        className="app-sider"
      >
        <div className="app-logo">
          <div className="app-logo-mark">
            <SafetyOutlined />
          </div>
          {!collapsed && <span className="app-logo-text">Go Admin Kit</span>}
        </div>
        <div className="app-menu-scroll">
          <Menu
            theme={mode === 'dark' ? 'dark' : 'light'}
            mode="inline"
            selectedKeys={[currentPath]}
            {...(collapsed ? {} : { openKeys, onOpenChange: setOpenKeys })}
            items={menuItems}
            onClick={({ key }) => {
              navigate(key)
              if (isMobile) setCollapsed(true)
            }}
            style={{ borderRight: 0, background: 'transparent' }}
          />
        </div>
      </Sider>

      <Layout className="app-main">
        <Header className="app-header">
          <Space size={16}>
            <span
              className="app-trigger"
              onClick={() => setCollapsed(!collapsed)}
            >
              {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            </span>
            <Breadcrumb
              className="app-breadcrumb"
              separator={<span className="app-bc-sep">/</span>}
              items={breadcrumbItems}
            />
          </Space>

          <Space size={8}>
            <span
              className="app-search-hint"
              onClick={() =>
                window.dispatchEvent(
                  new KeyboardEvent('keydown', { key: 'k', metaKey: isMac, ctrlKey: !isMac }),
                )
              }
            >
              <SearchOutlined />
              搜索
              <kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd>
            </span>
            <span
              className="app-trigger"
              onClick={(e) => {
                const rect = e.currentTarget.getBoundingClientRect()
                toggleTheme({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
              }}
              title={mode === 'dark' ? '切换为白蓝亮色' : '切换为深空暗色'}
            >
              {mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
            </span>
            <span
              className="app-trigger"
              onClick={() => setDensity((d) => (d === 'compact' ? 'comfortable' : 'compact'))}
              title={density === 'compact' ? '切换为舒适密度' : '切换为紧凑密度'}
            >
              <ColumnHeightOutlined />
            </span>
            <span className="app-trigger" onClick={toggleFullscreen} title={fullscreen ? '退出全屏' : '全屏'}>
              {fullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
            </span>
            <NotificationBell />
            <Dropdown
              placement="bottomRight"
              trigger={['click']}
              rootClassName="user-drop-popup"
              menu={{ items: userMenuItems, className: 'user-drop-menu' }}
            >
              <div className="app-user">
                <Avatar
                  size={34}
                  icon={<UserOutlined />}
                  style={{ background: 'linear-gradient(135deg, #6366f1, #4f46e5)' }}
                />
                <span className="app-user-name">{userInfo.nickname || userInfo.username}</span>
              </div>
            </Dropdown>
          </Space>
        </Header>

        <div className="app-content-glow" />
        <Content className="app-content" style={{ position: 'relative', zIndex: 1 }}>
          {loading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 100 }}>
              <Spin size="large" />
            </div>
          ) : (
            <div className="page-fade-in" key={currentPath}>
              <ErrorBoundary>
                <Outlet />
              </ErrorBoundary>
            </div>
          )}
        </Content>
      </Layout>

      <Modal
        title="首次登录请修改密码"
        open={!!userInfo.must_change_password}
        onOk={handleForcePwdSubmit}
        confirmLoading={forcePwdSubmitting}
        closable={false}
        maskClosable={false}
        keyboard={false}
        okText="确认修改"
        cancelButtonProps={{ style: { display: 'none' } }}
        destroyOnHidden
      >
        <Form form={forcePwdForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="old_password"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            name="new_password"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 6, message: '密码至少 6 位' },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm_password"
            label="确认新密码"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请确认新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('new_password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>

      <CommandPalette items={paletteItems} />

      {showBackTop && (
        <button
          type="button"
          className="back-top-btn"
          aria-label="回到顶部"
          onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
        >
          <VerticalAlignTopOutlined />
        </button>
      )}
    </Layout>
  )
}
