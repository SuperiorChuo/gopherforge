import { useEffect, useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import {
  Layout,
  Menu,
  Avatar,
  Dropdown,
  Space,
  Breadcrumb,
  Spin,
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
} from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, logout } from '@/store/slices/authSlice'
import { getToken } from '@/utils/request'

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

const menuItems: MenuItem2[] = [
  makeItem('仪表盘', '/dashboard', <DashboardOutlined />),
  makeItem('系统管理', 'system', <SettingOutlined />, [
    makeItem('用户管理', '/system/user', <UserOutlined />),
    makeItem('角色管理', '/system/role', <TeamOutlined />),
    makeItem('权限管理', '/system/permission', <SafetyOutlined />),
    makeItem('菜单管理', '/system/menu', <MenuOutlined />),
    makeItem('部门管理', '/system/department', <ApartmentOutlined />),
    makeItem('字典管理', '/system/dict', <DatabaseOutlined />),
    makeItem('文件管理', '/system/file', <FileOutlined />),
    makeItem('公告管理', '/system/notice', <NotificationOutlined />),
    makeItem('登录日志', '/system/login-log', <LoginOutlined />),
    makeItem('操作日志', '/system/operation-log', <FileTextOutlined />),
    makeItem('在线用户', '/system/online-user', <MonitorOutlined />),
    makeItem('系统设置', '/system/setting', <SettingOutlined />),
  ]),
  makeItem('运维监控', 'monitor', <CloudServerOutlined />, [
    makeItem('服务器监控', '/monitor/server', <CloudServerOutlined />),
    makeItem('MySQL 监控', '/monitor/mysql', <DatabaseOutlined />),
    makeItem('Redis 监控', '/monitor/redis', <BarsOutlined />),
    makeItem('定时任务', '/monitor/job', <ScheduleOutlined />),
  ]),
]

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
  '/system/online-user': '在线用户',
  '/system/setting': '系统设置',
  '/monitor/server': '服务器监控',
  '/monitor/mysql': 'MySQL 监控',
  '/monitor/redis': 'Redis 监控',
  '/monitor/job': '定时任务',
}

export default function MainLayout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  const { userInfo, loading } = useAppSelector((s) => s.auth)
  const token = getToken()

  const [collapsed, setCollapsed] = useState(false)

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

  const userMenuItems: MenuProps['items'] = [
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
  const breadcrumbTitle = pathBreadcrumbMap[currentPath] || ''
  const isSystem = currentPath.startsWith('/system/')
  const isMonitor = currentPath.startsWith('/monitor/')

  const breadcrumbItems = [
    { title: '首页' },
    ...(isSystem ? [{ title: '系统管理' }] : []),
    ...(isMonitor ? [{ title: '运维监控' }] : []),
    ...(breadcrumbTitle ? [{ title: breadcrumbTitle }] : []),
  ]

  const defaultOpenKeys = isSystem ? ['system'] : isMonitor ? ['monitor'] : []

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        style={{ background: '#001529' }}
        width={220}
      >
        <div style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#fff',
          fontSize: collapsed ? 14 : 18,
          fontWeight: 600,
          letterSpacing: 1,
          overflow: 'hidden',
          whiteSpace: 'nowrap',
        }}>
          {collapsed ? 'GA' : 'Go Admin Kit'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[currentPath]}
          defaultOpenKeys={defaultOpenKeys}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ borderRight: 0 }}
        />
      </Sider>

      <Layout>
        <Header style={{
          padding: '0 24px',
          background: '#fff',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid #f0f0f0',
        }}>
          <Breadcrumb items={breadcrumbItems} />

          <Dropdown menu={{ items: userMenuItems }} placement="bottomRight" arrow>
            <Space style={{ cursor: 'pointer' }}>
              <Avatar icon={<UserOutlined />} style={{ backgroundColor: '#1890ff' }} />
              <span style={{ fontWeight: 500 }}>{userInfo.nickname || userInfo.username}</span>
            </Space>
          </Dropdown>
        </Header>

        <Content style={{ margin: 24, minHeight: 280 }}>
          {loading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 100 }}>
              <Spin size="large" />
            </div>
          ) : (
            <Outlet />
          )}
        </Content>
      </Layout>
    </Layout>
  )
}
