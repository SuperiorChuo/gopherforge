import { useEffect, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, Spin } from 'antd'
import {
  AppstoreOutlined,
  HomeOutlined,
  LogoutOutlined,
  MoonOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { changePassword } from '@/api/auth'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { useThemeMode } from '@/theme/ThemeContext'
import { fetchCurrentUser, logout } from '@/store/slices/authSlice'
import { message } from '@/utils/feedback'
import { getToken } from '@/utils/request'
import { MOBILE_BASE, mobileLoginPath } from '@/utils/mobile-path'
import { mobileTitleKey } from './nav'
import { isNativeApp } from './native'
import { listenForInstallPrompt, registerMobileServiceWorker } from './pwa'
import './mobile.css'

export default function MobileLayout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const { mode, toggle } = useThemeMode()
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const token = getToken()
  // 移动壳不拦无权限用户：首页卡片按权限自行降级，有 userInfo 即可渲染。
  const authReady = !!userInfo
  const [pwdSubmitting, setPwdSubmitting] = useState(false)
  const [bootError, setBootError] = useState<string | null>(null)
  const [booting, setBooting] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    if (isNativeApp()) {
      document.documentElement.dataset.nativeApp = '1'
      return
    }
    listenForInstallPrompt()
    registerMobileServiceWorker()
  }, [])

  useEffect(() => {
    if (!token) {
      navigate(mobileLoginPath(location.pathname + location.search), { replace: true })
      return
    }
    if (authReady || booting) return
    setBooting(true)
    setBootError(null)
    void dispatch(fetchCurrentUser())
      .unwrap()
      .catch(() => setBootError(t('加载账号失败')))
      .finally(() => setBooting(false))
  }, [token, authReady, booting, dispatch, navigate, location.pathname, location.search, t])

  if (!token) return null

  if (!userInfo) {
    return (
      <div className="m-app-loading">
        {bootError ? (
          <div className="m-force-card">
            <h2>{t('加载账号失败')}</h2>
            <p>{t('请检查网络后重试')}</p>
            <Button
              type="primary"
              block
              loading={booting}
              onClick={() => {
                setBootError(null)
                setBooting(false)
              }}
            >
              {t('重试')}
            </Button>
          </div>
        ) : (
          <Spin size="large" />
        )}
      </div>
    )
  }

  const handleLogout = async () => {
    await dispatch(logout())
    navigate(mobileLoginPath(MOBILE_BASE), { replace: true })
  }

  const handleForcePwd = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setPwdSubmitting(true)
    try {
      await changePassword({ old_password: values.old_password, new_password: values.new_password })
      message.success(t('密码修改成功'))
      form.resetFields()
      dispatch(fetchCurrentUser())
    } catch {
      message.error(t('密码修改失败，请检查当前密码是否正确'))
    } finally {
      setPwdSubmitting(false)
    }
  }

  if (userInfo.must_change_password) {
    return (
      <div className="m-app-blocked">
        <div className="m-force-card">
          <h2>{t('首次登录请修改密码')}</h2>
          <p>{t('修改后即可进入移动控制台')}</p>
          <Form form={form} layout="vertical" onFinish={handleForcePwd}>
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
                    if (!value || getFieldValue('new_password') === value) return Promise.resolve()
                    return Promise.reject(new Error(t('两次输入的密码不一致')))
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={pwdSubmitting} block>
              {t('确认修改')}
            </Button>
          </Form>
        </div>
      </div>
    )
  }

  const path = location.pathname
  const title = t(mobileTitleKey(path))
  const tabs = [
    { key: 'home', to: '/m', label: t('态势'), icon: <HomeOutlined />, active: path === '/m' },
    { key: 'workbench', to: '/m/workbench', label: t('工作台'), icon: <AppstoreOutlined />, active: path.startsWith('/m/workbench') || path.startsWith('/m/soon') || path.startsWith('/m/tasks') || path.startsWith('/m/ops') || path.startsWith('/m/security') || path.startsWith('/m/directory') || path.startsWith('/m/catalog') || path.startsWith('/m/notices') },
    { key: 'me', to: '/m/me', label: t('我的'), icon: <UserOutlined />, active: path.startsWith('/m/me') },
  ] as const

  return (
    <div className="m-app">
      <header className="m-topbar">
        <div className="m-brand">
          <p className="m-brand-kicker">Go Admin Kit</p>
          <h1 className="m-brand-title">{title}</h1>
        </div>
        <div className="m-topbar-actions">
          <button
            type="button"
            className="m-icon-btn"
            aria-label={mode === 'dark' ? t('切换亮色') : t('切换深色')}
            onClick={(e) => toggle({ x: e.clientX, y: e.clientY })}
          >
            {mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
          </button>
          <button type="button" className="m-text-btn" onClick={() => void handleLogout()}>
            <LogoutOutlined /> {t('退出')}
          </button>
        </div>
      </header>
      <div className="m-main">
        <Outlet />
      </div>
      <nav className="m-tabbar" aria-label={t('功能切换')}>
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            className={`m-tab${tab.active ? ' is-on' : ''}`}
            aria-current={tab.active ? 'page' : undefined}
            onClick={() => navigate(tab.to)}
          >
            {tab.icon}
            <span>{tab.label}</span>
          </button>
        ))}
      </nav>
    </div>
  )
}
