import { useEffect, useMemo, useState } from 'react'
import '@/styles/app-layout.css'
import '@/list-pages.css'
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
  UserOutlined,
  LogoutOutlined,
  LockOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  MoreOutlined,
  SearchOutlined,
  SafetyOutlined,
  SunOutlined,
  MoonOutlined,
  HomeOutlined,
  VerticalAlignTopOutlined,
  ColumnHeightOutlined,
  GlobalOutlined,
} from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, logout } from '@/store/slices/authSlice'
import { getToken } from '@/utils/request'
import { usePermission } from '@/hooks/usePermission'
import { ROUTE_PERMISSIONS } from '@/router/route-permissions'
import { changePassword } from '@/api/auth'
import { message } from '@/utils/feedback'
import NotificationBell from '@/components/common/NotificationBell'
import ErrorBoundary from '@/components/common/ErrorBoundary'
import CommandPalette from '@/components/common/CommandPalette'
import { useThemeMode } from '@/theme/ThemeContext'
import { useLocale } from '@/i18n/LocaleContext'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n/init'
import { MENU_DEFS, GROUP_META, pathBreadcrumbMap, type MenuDef } from './menu-defs'
import { apiMenusToDefs, buildMenuItems, buildPaletteItems } from './menu-build'

const { Header, Sider, Content } = Layout

export default function MainLayout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  // 分字段订阅：整片订阅下任一 auth action 都换 slice 引用，会让菜单树与
  // 命令面板（都依赖 hasPerm/menus）无谓全量重建
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const loading = useAppSelector((s) => s.auth.loading)
  const permissions = useAppSelector((s) => s.auth.permissions)
  const menus = useAppSelector((s) => s.auth.menus)
  const { hasPerm, isSuperAdmin } = usePermission()
  const { mode, toggle: toggleTheme } = useThemeMode()
  const { locale, setLocale } = useLocale()
  const { t } = useTranslation()
  const token = getToken()
  // super_admin 可不依赖 permissions 列表；普通用户需要 permissions 才渲染侧栏
  const authReady = !!userInfo && (isSuperAdmin || permissions.length > 0 || !Object.keys(ROUTE_PERMISSIONS).length)

  // 侧栏以后端 /user/menus 为准（菜单管理页的增删改即时生效）；为空时回落到静态定义
  const menuDefs = useMemo(() => {
    const dynamic = apiMenusToDefs(menus)
    return dynamic.length > 0 ? dynamic : MENU_DEFS
  }, [menus])
  // 叶子路径 → 祖先分组链（自顶向下）。系统管理拆组后子菜单路径前缀不再等于分组
  // key（如 /system/login-log 挂在 /logs 下），展开与面包屑分组一律以菜单树为准
  const menuTrails = useMemo(() => {
    const map = new Map<string, MenuDef[]>()
    const walk = (nodes: MenuDef[], trail: MenuDef[]) => {
      nodes.forEach((d) => {
        if (d.children) walk(d.children, [...trail, d])
        else if (trail.length) map.set(d.key, trail)
      })
    }
    walk(menuDefs, [])
    return map
  }, [menuDefs])
  const menuItems = useMemo(() => buildMenuItems(menuDefs, hasPerm), [menuDefs, hasPerm, locale, i18n.language])
  const paletteItems = useMemo(() => buildPaletteItems(menuDefs, hasPerm), [menuDefs, hasPerm, locale, i18n.language])
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
    if (!isMobile || collapsed) return
    const previousOverflow = document.body.style.overflow
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setCollapsed(true)
    }
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [collapsed, isMobile])

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
  // 当前路径的分组链：优先按菜单树取祖先；树里查不到（带参数的子路由、隐藏页）
  // 时回落到一级路径段匹配 GROUP_META
  const trailOf = (p: string): MenuDef[] => {
    const trail = menuTrails.get(p)
    if (trail) return trail
    const seg = p.split('/')[1]
    const key = seg ? `/${seg}` : ''
    const meta = key ? GROUP_META[key] : undefined
    return meta ? [{ label: meta.label, key, icon: meta.icon }] : []
  }
  const [openKeys, setOpenKeys] = useState<string[]>(() => trailOf(pathname).map((d) => d.key))

  // 直达子路由（面包屑/外部跳转）或菜单树就绪时自动展开所属分组（含嵌套分组全链）
  useEffect(() => {
    const keys = trailOf(pathname).map((d) => d.key)
    if (keys.length) {
      setOpenKeys((prev) => {
        const missing = keys.filter((k) => !prev.includes(k))
        return missing.length ? [...prev, ...missing] : prev
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname, menuTrails])

  useEffect(() => {
    if (!token) {
      // 带上来源路径，登录后回跳（授权页等深链依赖此行为）；根路径不带以免冗余
      const from = window.location.pathname + window.location.search
      const target = from && from !== '/' ? `/login?redirect=${encodeURIComponent(from)}` : '/login'
      navigate(target, { replace: true })
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
      message.success(t('密码修改成功'))
      forcePwdForm.resetFields()
      dispatch(fetchCurrentUser())
    } catch {
      message.error(t('密码修改失败，请检查当前密码是否正确'))
    } finally {
      setForcePwdSubmitting(false)
    }
  }

  const roleText =
    userInfo.roles && userInfo.roles.length > 0
      ? userInfo.roles.map((r) => r.name).join(' · ')
      : ''

  const openCommandPalette = () => {
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'k', metaKey: isMac, ctrlKey: !isMac }),
    )
  }

  const displayMenuItems: MenuProps['items'] = [
    {
      key: 'search',
      icon: <SearchOutlined />,
      label: t('搜索'),
      onClick: openCommandPalette,
    },
    {
      key: 'density',
      icon: <ColumnHeightOutlined />,
      label: density === 'compact' ? t('切换为舒适密度') : t('切换为紧凑密度'),
      onClick: () => setDensity((d) => (d === 'compact' ? 'comfortable' : 'compact')),
    },
    {
      key: 'locale',
      icon: <GlobalOutlined />,
      label: locale === 'zh' ? 'English' : '中文',
      onClick: () => setLocale(locale === 'en' ? 'zh' : 'en'),
    },
    {
      key: 'fullscreen',
      icon: fullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />,
      label: fullscreen ? t('退出全屏') : t('全屏'),
      onClick: toggleFullscreen,
    },
  ]

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'userinfo',
      disabled: true,
      className: 'user-drop-info-item',
      style: { cursor: 'default', height: 'auto', lineHeight: 'inherit' },
      label: (
        <div className="user-drop-head">
          <div className="user-drop-name">{isSuperAdmin ? t('管理员') : userInfo.nickname || userInfo.username}</div>
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
      label: t('个人中心'),
      onClick: () => navigate('/profile'),
    },
    {
      key: 'password',
      icon: <LockOutlined />,
      label: t('修改密码'),
      onClick: () => navigate('/profile'),
    },
    { type: 'divider' },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: t('退出登录'),
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
  // 面包屑分组取最近一层分组，与侧栏展开同源（菜单树驱动）
  const groupTrail = trailOf(currentPath)
  const groupMeta = groupTrail.length ? groupTrail[groupTrail.length - 1] : null

  const breadcrumbItems = [
    {
      title: (
        <button
          type="button"
          className="app-bc-link"
          onClick={() => navigate('/dashboard')}
          title={t('回到仪表盘')}
        >
          <HomeOutlined />
          <span>{t('首页')}</span>
        </button>
      ),
    },
    ...(groupMeta
      ? [
          {
            title: (
              <span className="app-bc-mid">
                {groupMeta.icon}
                <span>{t(groupMeta.label)}</span>
              </span>
            ),
          },
        ]
      : []),
    ...(breadcrumbTitle
      ? [
          {
            title: <span className="app-bc-current">{t(breadcrumbTitle)}</span>,
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
          {!collapsed && <span className="app-logo-text">GopherForge</span>}
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
          <Space size={16} className="app-header-leading">
            <span
              className="app-trigger"
              onClick={() => setCollapsed(!collapsed)}
              onKeyDown={(event) => {
                if (event.key !== 'Enter' && event.key !== ' ') return
                event.preventDefault()
                setCollapsed(!collapsed)
              }}
              role="button"
              tabIndex={0}
              aria-label={collapsed ? t('展开导航菜单') : t('收起导航菜单')}
              aria-expanded={!collapsed}
            >
              {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            </span>
            <Breadcrumb
              className="app-breadcrumb"
              separator={<span className="app-bc-sep">/</span>}
              items={breadcrumbItems}
             />
            <span className="app-mobile-title">
              <span className="app-mobile-title-icon">{groupMeta?.icon || <HomeOutlined />}</span>
              <span className="app-mobile-title-text">{breadcrumbTitle ? t(breadcrumbTitle) : groupMeta?.label ? t(groupMeta.label) : 'Go Admin Kit'}</span>
            </span>
          </Space>

          <Space size={8} className="app-header-actions">
            <span
              className="app-search-hint"
              onClick={() =>
                window.dispatchEvent(
                  new KeyboardEvent('keydown', { key: 'k', metaKey: isMac, ctrlKey: !isMac }),
                )
              }
            >
              <SearchOutlined />
              {t('搜索')}
              <kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd>
            </span>
            <span
              className="app-trigger"
              onClick={(e) => {
                const rect = e.currentTarget.getBoundingClientRect()
                toggleTheme({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
              }}
              title={mode === 'dark' ? t('切换为白蓝亮色') : t('切换为深空暗色')}
            >
              {mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
            </span>
            <span
              className="app-trigger app-density-trigger"
              onClick={() => setDensity((d) => (d === 'compact' ? 'comfortable' : 'compact'))}
              title={density === 'compact' ? t('切换为舒适密度') : t('切换为紧凑密度')}
            >
              <ColumnHeightOutlined />
            </span>
            <span
              className="app-trigger app-lang-trigger"
              onClick={() => setLocale(locale === 'en' ? 'zh' : 'en')}
              title={t('切换语言')}
            >
              <GlobalOutlined />
              {locale === 'zh' ? 'EN' : '中文'}
            </span>
            <span className="app-trigger app-fullscreen-trigger" onClick={toggleFullscreen} title={fullscreen ? t('退出全屏') : t('全屏')}>
              {fullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
            </span>
            <Dropdown placement="bottomRight" trigger={['click']} menu={{ items: displayMenuItems }}>
              <span
                className="app-trigger app-display-more"
                title={t('显示与语言')}
                role="button"
                tabIndex={0}
                aria-label={t('显示与语言')}
              >
                <MoreOutlined />
              </span>
            </Dropdown>
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
                  src={userInfo.avatar || undefined}
                  icon={<UserOutlined />}
                  style={{ background: 'linear-gradient(135deg, #6366f1, #4f46e5)' }}
                />
                <span className="app-user-name">{isSuperAdmin ? t('管理员') : userInfo.nickname || userInfo.username}</span>
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
        title={t('首次登录请修改密码')}
        open={!!userInfo.must_change_password}
        onOk={handleForcePwdSubmit}
        confirmLoading={forcePwdSubmitting}
        closable={false}
        maskClosable={false}
        keyboard={false}
        okText={t('确认修改')}
        cancelButtonProps={{ style: { display: 'none' } }}
        destroyOnHidden
      >
        <Form form={forcePwdForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="old_password"
            label={t('当前密码')}
            rules={[{ required: true, message: t('请输入当前密码') }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            name="new_password"
            label={t('新密码')}
            rules={[
              { required: true, message: t('请输入新密码') },
              { min: 6, message: t('密码至少 6 位') },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm_password"
            label={t('确认新密码')}
            dependencies={['new_password']}
            rules={[
              { required: true, message: t('请确认新密码') },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('new_password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error(t('两次输入的密码不一致')))
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
          aria-label={t('回到顶部')}
          onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
        >
          <VerticalAlignTopOutlined />
        </button>
      )}
    </Layout>
  )
}
