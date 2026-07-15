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
  SunOutlined,
  MoonOutlined,
  HomeOutlined,
  VerticalAlignTopOutlined,
} from '@ant-design/icons'
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
  children?: MenuDef[]
}

// 叶子节点的权限码统一取 ROUTE_PERMISSIONS，保证菜单可见性和路由守卫一致
const MENU_DEFS: MenuDef[] = [
  { label: '仪表盘', key: '/dashboard', icon: <DashboardOutlined /> },
  {
    label: '系统管理',
    key: 'system',
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
    ],
  },
  {
    label: '运维监控',
    key: 'monitor',
    icon: <CloudServerOutlined />,
    children: [
      { label: '服务器监控', key: '/monitor/server', icon: <CloudServerOutlined /> },
      { label: '数据库监控', key: '/monitor/mysql', icon: <DatabaseOutlined /> },
      { label: 'Redis 监控', key: '/monitor/redis', icon: <BarsOutlined /> },
      { label: '定时任务', key: '/monitor/job', icon: <ScheduleOutlined /> },
    ],
  },
]

function buildMenuItems(defs: MenuDef[], hasPerm: (code?: string) => boolean): MenuItem2[] {
  return defs
    .map((d) => {
      if (d.children) {
        const children = buildMenuItems(d.children, hasPerm)
        return children.length > 0 ? makeItem(d.label, d.key, d.icon, children) : null
      }
      return hasPerm(ROUTE_PERMISSIONS[d.key]) ? makeItem(d.label, d.key, d.icon) : null
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
      } else if (hasPerm(ROUTE_PERMISSIONS[d.key])) {
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
  '/monitor/server': '服务器监控',
  '/monitor/mysql': '数据库监控',
  '/monitor/redis': 'Redis 监控',
  '/monitor/job': '定时任务',
}

export default function MainLayout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  const { userInfo, loading } = useAppSelector((s) => s.auth)
  const { hasPerm } = usePermission()
  const { mode, toggle: toggleTheme } = useThemeMode()
  const token = getToken()

  const menuItems = useMemo(() => buildMenuItems(MENU_DEFS, hasPerm), [hasPerm])
  const paletteItems = useMemo(() => buildPaletteItems(MENU_DEFS, hasPerm), [hasPerm])
  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform)

  const [collapsed, setCollapsed] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [showBackTop, setShowBackTop] = useState(false)
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
  const [openKeys, setOpenKeys] = useState<string[]>(() =>
    pathname.startsWith('/system/') ? ['system'] : pathname.startsWith('/monitor/') ? ['monitor'] : [],
  )

  // 直达子路由（面包屑/外部跳转）时自动展开所属分组
  useEffect(() => {
    const group = pathname.startsWith('/system/')
      ? 'system'
      : pathname.startsWith('/monitor/')
        ? 'monitor'
        : null
    if (group) {
      setOpenKeys((keys) => (keys.includes(group) ? keys : [...keys, group]))
    }
  }, [pathname])

  useEffect(() => {
    if (!token) {
      navigate('/login', { replace: true })
      return
    }
    if (!userInfo) {
      dispatch(fetchCurrentUser())
    }
  }, [token, userInfo, dispatch, navigate])

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

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'userinfo',
      disabled: true,
      style: { cursor: 'default', padding: 0 },
      label: (
        <div className="user-drop-head">
          <div className="user-drop-name">{userInfo.nickname || userInfo.username}</div>
          <div className="user-drop-meta">
            {userInfo.email || userInfo.username}
          </div>
          {userInfo.roles && userInfo.roles.length > 0 && (
            <div className="user-drop-roles">
              {userInfo.roles.map((r) => (
                <span key={r.id} className="user-drop-role">{r.name}</span>
              ))}
            </div>
          )}
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
  const isSystem = currentPath.startsWith('/system/')
  const isMonitor = currentPath.startsWith('/monitor/')

  const breadcrumbItems = [
    { title: <span><HomeOutlined style={{ marginRight: 4 }} />首页</span> },
    ...(isSystem ? [{ title: <span><SettingOutlined style={{ marginRight: 4 }} />系统管理</span> }] : []),
    ...(isMonitor ? [{ title: <span><CloudServerOutlined style={{ marginRight: 4 }} />运维监控</span> }] : []),
    ...(breadcrumbTitle ? [{ title: breadcrumbTitle }] : []),
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        trigger={null}
        width={224}
        breakpoint="lg"
        onBreakpoint={(broken) => setCollapsed(broken)}
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
            onClick={({ key }) => navigate(key)}
            style={{ borderRight: 0, background: 'transparent' }}
          />
        </div>
      </Sider>

      <Layout>
        <Header className="app-header">
          <Space size={16}>
            <span
              className="app-trigger"
              onClick={() => setCollapsed(!collapsed)}
            >
              {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            </span>
            <Breadcrumb items={breadcrumbItems} />
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
            <span className="app-trigger" onClick={toggleFullscreen} title={fullscreen ? '退出全屏' : '全屏'}>
              {fullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
            </span>
            <NotificationBell />
            <Dropdown menu={{ items: userMenuItems }} placement="bottomRight" arrow>
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
