import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Form, Input, Button, Spin } from 'antd'
import type { InputRef } from 'antd'
import { message } from '@/utils/feedback'
import {
  UserOutlined,
  LockOutlined,
  SafetyOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
  SafetyCertificateOutlined,
  RadarChartOutlined,
  CloudOutlined,
  DownOutlined,
  UpOutlined,
  CheckOutlined,
  SunOutlined,
  MoonOutlined,
} from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, login } from '@/store/slices/authSlice'
import { getCaptcha } from '@/api/auth'
import { setTokens } from '@/utils/request'
import { useThemeMode } from '@/theme/ThemeContext'

export default function LoginPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { mode, toggle: toggleTheme } = useThemeMode()
  const { loading } = useAppSelector((s) => s.auth)
  const [form] = Form.useForm()
  const [totpForm] = Form.useForm()
  const [error, setError] = useState<string | null>(null)
  const [totpStep, setTotpStep] = useState(false)
  const [challengeId, setChallengeId] = useState<string | null>(null)
  const [totpLoading, setTotpLoading] = useState(false)
  const [tenantOpen, setTenantOpen] = useState(false)
  const [success, setSuccess] = useState(false)

  const [captchaImg, setCaptchaImg] = useState('')
  const [captchaId, setCaptchaId] = useState('')
  const [captchaLoading, setCaptchaLoading] = useState(false)
  const [captchaFlash, setCaptchaFlash] = useState(false)

  const usernameRef = useRef<InputRef>(null)
  const totpRef = useRef<InputRef>(null)
  const totpSubmitting = useRef(false)

  /** 成功微过渡：按钮 ✓ + 卡片轻收，再跳转 */
  const finishWithSuccess = useCallback(() => {
    setSuccess(true)
    message.success('登录成功')
    window.setTimeout(() => {
      navigate('/dashboard', { replace: true })
    }, 280)
  }, [navigate])

  const refreshCaptcha = useCallback(async () => {
    setCaptchaLoading(true)
    setCaptchaFlash(true)
    try {
      const res = await getCaptcha()
      setCaptchaId(res.key)
      setCaptchaImg(res.image.startsWith('data:') ? res.image : `data:image/png;base64,${res.image}`)
      form.setFieldValue('captcha_code', '')
    } catch {
      setError('验证码加载失败，请点击刷新')
    } finally {
      setCaptchaLoading(false)
      window.setTimeout(() => setCaptchaFlash(false), 450)
    }
  }, [form])

  useEffect(() => {
    refreshCaptcha()
  }, [refreshCaptcha])

  // 租户：URL / 子域预填时自动展开；否则默认折叠
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const q = params.get('tenant') || params.get('tenant_code')
    if (q) {
      form.setFieldValue('tenant_code', q)
      setTenantOpen(true)
      return
    }
    const host = window.location.hostname.toLowerCase()
    const parts = host.split('.')
    if (parts.length >= 2) {
      const label = parts[0]
      if (label && !['www', 'api', 'app', 'admin', 'localhost'].includes(label) && !/^\d+$/.test(label)) {
        form.setFieldValue('tenant_code', label)
        setTenantOpen(true)
      }
    }
  }, [form])

  // 首焦：用户名
  useEffect(() => {
    if (!totpStep) {
      const t = window.setTimeout(() => usernameRef.current?.focus(), 80)
      return () => window.clearTimeout(t)
    }
  }, [totpStep])

  // 2FA：进入后聚焦
  useEffect(() => {
    if (totpStep) {
      const t = window.setTimeout(() => totpRef.current?.focus(), 80)
      return () => window.clearTimeout(t)
    }
  }, [totpStep])

  const onFinish = async (values: {
    username: string
    password: string
    captcha_code: string
    tenant_code?: string
  }) => {
    setError(null)
    try {
      const result = await dispatch(
        login({
          username: values.username,
          password: values.password,
          captcha_id: captchaId,
          captcha_code: values.captcha_code,
          tenant_code: values.tenant_code?.trim() || undefined,
        }),
      ).unwrap()
      if (result.require_totp && result.totp_challenge_id) {
        setChallengeId(result.totp_challenge_id)
        setTotpStep(true)
        totpForm.resetFields()
        return
      }
      try {
        await dispatch(fetchCurrentUser()).unwrap()
      } catch {
        // 登录已成功，拉取失败时仍进入系统
      }
      finishWithSuccess()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '用户名或密码错误'
      setError(msg)
      refreshCaptcha()
    }
  }

  const onTotpFinish = async (values: { code: string }) => {
    if (!challengeId || totpSubmitting.current || success) return
    totpSubmitting.current = true
    setError(null)
    setTotpLoading(true)
    try {
      const { verifyTotpLogin } = await import('@/api/auth')
      const res = await verifyTotpLogin({ challenge_id: challengeId, code: values.code })
      if (res.access_token && res.refresh_token) {
        setTokens(res.access_token, res.refresh_token)
      }
      finishWithSuccess()
    } catch {
      setError('验证码不正确，请重试')
      totpForm.setFieldValue('code', '')
      window.setTimeout(() => totpRef.current?.focus(), 40)
      setTotpLoading(false)
      totpSubmitting.current = false
    }
  }

  const onTotpCodeChange = (e: ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value.replace(/\D/g, '').slice(0, 6)
    totpForm.setFieldValue('code', raw)
    if (raw.length === 6) {
      void totpForm.submit()
    }
  }

  return (
    <div className="login-page">
      <div className="login-aurora login-aurora-1" />
      <div className="login-aurora login-aurora-2" />
      <div className="login-aurora login-aurora-3" />
      <div className="login-grid" />

      <button
        type="button"
        className="login-theme-toggle"
        title={mode === 'dark' ? '切换亮色' : '切换深色'}
        aria-label={mode === 'dark' ? '切换亮色' : '切换深色'}
        onClick={(e) => {
          const rect = e.currentTarget.getBoundingClientRect()
          toggleTheme({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
        }}
      >
        {mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
      </button>

      <div className={`login-shell login-liquid is-alive${success ? ' is-success' : ''}`}>
        <div className="login-pointer-glow" aria-hidden="true" />
        <div className="login-liquid-sheen" aria-hidden="true">
          <i />
          <i />
        </div>

        <div className="login-brand">
          <div className="login-logo">
            <div className="login-logo-mark">
              <SafetyOutlined />
            </div>
            <span className="login-logo-name">Go Admin Kit</span>
          </div>

          <div className="login-brand-copy">
            <h1 className="login-headline">
              以工程之美，
              <br />
              驱动<em>企业级</em>管理
            </h1>
            <p className="login-subline">
              以克制的架构与现代交互，
              <br />
              构筑可托付的企业级中台。
            </p>
          </div>

          <ul className="login-features">
            <li>
              <span className="login-feature-icon"><ThunderboltOutlined /></span>
              Go 与 React 协同 · 从容承载复杂业务
            </li>
            <li>
              <span className="login-feature-icon"><SafetyCertificateOutlined /></span>
              权限精密可控 · 身份双重守护
            </li>
            <li>
              <span className="login-feature-icon"><RadarChartOutlined /></span>
              全程可观测 · 每一次操作皆可追溯
            </li>
          </ul>
        </div>

        <div className="login-form-panel">
          <div className="login-form-inner">
            <p className="login-mobile-tagline">以工程之美，驱动企业级管理</p>
            {!totpStep ? (
              <>
                <h2 className="login-form-title">进入控制台</h2>
                <p className="login-form-sub">使用企业账户继续</p>
              </>
            ) : (
              <>
                <div className="login-step-rail" aria-hidden="true">
                  <span className="login-step done">1 凭证</span>
                  <span className="login-step-line" />
                  <span className="login-step active">2 二次验证</span>
                </div>
                <h2 className="login-form-title">身份核验</h2>
                <p className="login-form-sub">请输入身份验证器中的 6 位动态码</p>
              </>
            )}

            {error && (
              <div className="login-error" role="alert">
                <span className="login-error-text">{error}</span>
                <button
                  type="button"
                  className="login-error-close"
                  aria-label="关闭错误提示"
                  onClick={() => setError(null)}
                >
                  ×
                </button>
              </div>
            )}

            {!totpStep ? (
              <Form
                form={form}
                name="login"
                onFinish={onFinish}
                autoComplete="off"
                size="large"
                className="login-form"
                requiredMark={false}
              >
                <div className="login-tenant">
                  <button
                    type="button"
                    className="login-tenant-toggle"
                    onClick={() => setTenantOpen((v) => !v)}
                    aria-expanded={tenantOpen}
                  >
                    <CloudOutlined />
                    <span>切换组织</span>
                    {tenantOpen ? <UpOutlined /> : <DownOutlined />}
                  </button>
                  {tenantOpen && (
                    <Form.Item name="tenant_code" className="login-tenant-field">
                      <Input
                        prefix={<CloudOutlined />}
                        placeholder="组织标识（可选，默认 default）"
                        aria-label="组织标识"
                      />
                    </Form.Item>
                  )}
                </div>

                <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                  <Input
                    ref={usernameRef}
                    prefix={<UserOutlined />}
                    placeholder="用户名"
                    aria-label="用户名"
                    autoComplete="username"
                  />
                </Form.Item>
                <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                  <Input.Password
                    prefix={<LockOutlined />}
                    placeholder="密码"
                    aria-label="密码"
                    autoComplete="current-password"
                  />
                </Form.Item>
                <div className="login-captcha-row">
                  <Form.Item
                    name="captcha_code"
                    rules={[{ required: true, message: '请输入验证码' }]}
                    className="login-captcha-field"
                  >
                    <Input
                      prefix={<SafetyOutlined />}
                      placeholder="验证码"
                      maxLength={6}
                      aria-label="验证码"
                    />
                  </Form.Item>
                  <button
                    type="button"
                    className={`login-captcha-img${captchaFlash ? ' is-flash' : ''}`}
                    onClick={refreshCaptcha}
                    title="点击刷新验证码"
                    aria-label="刷新验证码"
                  >
                    {captchaImg && !captchaLoading ? (
                      <img src={captchaImg} alt="图形验证码" />
                    ) : (
                      <Spin size="small" indicator={<ReloadOutlined spin />} />
                    )}
                  </button>
                </div>
                <Form.Item className="login-submit-item">
                  <Button
                    type="primary"
                    htmlType="submit"
                    loading={loading && !success}
                    disabled={success}
                    block
                    className={success ? 'is-success' : undefined}
                    icon={success ? <CheckOutlined /> : undefined}
                  >
                    {success ? '已验证' : '登 录'}
                  </Button>
                </Form.Item>
              </Form>
            ) : (
              <Form
                form={totpForm}
                name="totp"
                onFinish={onTotpFinish}
                autoComplete="one-time-code"
                size="large"
                className="login-form"
                requiredMark={false}
              >
                <Form.Item
                  name="code"
                  rules={[
                    { required: true, message: '请输入 6 位验证码' },
                    { len: 6, message: '请输入 6 位数字' },
                  ]}
                >
                  <Input
                    ref={totpRef}
                    className="login-totp-input"
                    placeholder="······"
                    maxLength={6}
                    inputMode="numeric"
                    aria-label="6 位动态验证码"
                    onChange={onTotpCodeChange}
                    disabled={success}
                  />
                </Form.Item>
                <Form.Item className="login-submit-item">
                  <Button
                    type="primary"
                    htmlType="submit"
                    loading={totpLoading && !success}
                    disabled={success}
                    block
                    className={success ? 'is-success' : undefined}
                    icon={success ? <CheckOutlined /> : undefined}
                  >
                    {success ? '已验证' : '验 证'}
                  </Button>
                </Form.Item>
                <Button
                  type="link"
                  block
                  className="login-back-link"
                  onClick={() => {
                    setTotpStep(false)
                    setChallengeId(null)
                    setError(null)
                    totpForm.resetFields()
                    refreshCaptcha()
                  }}
                >
                  返回登录
                </Button>
              </Form>
            )}

            <div className="login-footer">© {new Date().getFullYear()} Go Admin Kit</div>
          </div>
        </div>
      </div>
    </div>
  )
}
