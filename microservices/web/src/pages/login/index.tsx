import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Form, Input, Button, Alert, Spin } from 'antd'
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
} from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, login } from '@/store/slices/authSlice'
import { getCaptcha } from '@/api/auth'
import { setTokens } from '@/utils/request'

export default function LoginPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { loading } = useAppSelector((s) => s.auth)
  const [form] = Form.useForm()
  const [error, setError] = useState<string | null>(null)
  const [totpStep, setTotpStep] = useState(false)
  const [challengeId, setChallengeId] = useState<string | null>(null)

  const [captchaImg, setCaptchaImg] = useState('')
  const [captchaId, setCaptchaId] = useState('')
  const [captchaLoading, setCaptchaLoading] = useState(false)

  const refreshCaptcha = useCallback(async () => {
    setCaptchaLoading(true)
    try {
      const res = await getCaptcha()
      setCaptchaId(res.key)
      setCaptchaImg(res.image.startsWith('data:') ? res.image : `data:image/png;base64,${res.image}`)
      form.setFieldValue('captcha_code', '')
    } catch {
      setError('验证码加载失败，请点击刷新')
    } finally {
      setCaptchaLoading(false)
    }
  }, [form])

  useEffect(() => {
    refreshCaptcha()
  }, [refreshCaptcha])

  // M4: prefill tenant from ?tenant= or subdomain (acme.localhost → acme)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const q = params.get('tenant') || params.get('tenant_code')
    if (q) {
      form.setFieldValue('tenant_code', q)
      return
    }
    const host = window.location.hostname.toLowerCase()
    const parts = host.split('.')
    if (parts.length >= 2) {
      const label = parts[0]
      if (label && !['www', 'api', 'app', 'admin', 'localhost'].includes(label) && !/^\d+$/.test(label)) {
        form.setFieldValue('tenant_code', label)
      }
    }
  }, [form])

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
        return
      }
      // 再拉一次 /user/me + /user/menus，保证侧栏权限与后端菜单一致
      try {
        await dispatch(fetchCurrentUser()).unwrap()
      } catch {
        // 登录已成功，拉取失败时仍进入系统（login 响应里已有 user）
      }
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '用户名或密码错误'
      setError(msg)
      refreshCaptcha()
    }
  }

  const onTotpFinish = async (values: { code: string }) => {
    if (!challengeId) return
    setError(null)
    try {
      const { verifyTotpLogin } = await import('@/api/auth')
      const res = await verifyTotpLogin({ challenge_id: challengeId, code: values.code })
      if (res.access_token && res.refresh_token) {
        setTokens(res.access_token, res.refresh_token)
      }
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch {
      setError('验证码错误，请重试')
    }
  }

  return (
    <div className="login-page">
      <div className="login-aurora login-aurora-1" />
      <div className="login-aurora login-aurora-2" />
      <div className="login-aurora login-aurora-3" />
      <div className="login-grid" />

      <div className="login-shell">
        <div className="login-brand">
          <div className="login-logo">
            <div className="login-logo-mark">
              <SafetyOutlined />
            </div>
            <span className="login-logo-name">Go Admin Kit</span>
          </div>

          <div>
            <h1 className="login-headline">
              以工程之美，
              <br />
              驱动<em>企业级</em>管理
            </h1>
            <p className="login-subline">
              高性能 Go 后端与现代 React 前端的完整解决方案，
              <br />
              安全、可靠、开箱即用。
            </p>
          </div>

          <ul className="login-features">
            <li>
              <span className="login-feature-icon"><ThunderboltOutlined /></span>
              Go + React 19 现代技术栈，极致响应
            </li>
            <li>
              <span className="login-feature-icon"><SafetyCertificateOutlined /></span>
              RBAC 细粒度权限 · TOTP 两步验证
            </li>
            <li>
              <span className="login-feature-icon"><RadarChartOutlined /></span>
              实时监控 · 操作留痕 · 审计一体化
            </li>
          </ul>
        </div>

        <div className="login-form-panel">
          <div className="login-form-inner">
            <h2 className="login-form-title">{totpStep ? '两步验证' : '欢迎回来'}</h2>
            <p className="login-form-sub">
              {totpStep ? '请输入身份验证器中的 6 位验证码' : '登录您的账户，继续您的工作'}
            </p>

            {error && (
              <Alert
                message={error}
                type="error"
                showIcon
                style={{ margin: '20px 0 0' }}
                closable
                onClose={() => setError(null)}
              />
            )}

            {!totpStep ? (
              <Form
                form={form}
                name="login"
                onFinish={onFinish}
                autoComplete="off"
                size="large"
                style={{ marginTop: 28 }}
              >
                <Form.Item name="tenant_code">
                  <Input prefix={<CloudOutlined />} placeholder="租户 Code（可选，默认 default）" />
                </Form.Item>
                <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                  <Input prefix={<UserOutlined />} placeholder="用户名" />
                </Form.Item>
                <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                  <Input.Password prefix={<LockOutlined />} placeholder="密码" />
                </Form.Item>
                <div style={{ display: 'flex', gap: 12 }}>
                  <Form.Item
                    name="captcha_code"
                    rules={[{ required: true, message: '请输入验证码' }]}
                    style={{ flex: 1 }}
                  >
                    <Input prefix={<SafetyOutlined />} placeholder="验证码" maxLength={6} />
                  </Form.Item>
                  <div
                    className="login-captcha-img"
                    onClick={refreshCaptcha}
                    title="点击刷新验证码"
                  >
                    {captchaImg && !captchaLoading ? (
                      <img src={captchaImg} alt="验证码" />
                    ) : (
                      <Spin size="small" indicator={<ReloadOutlined spin />} />
                    )}
                  </div>
                </div>
                <Form.Item style={{ marginBottom: 0 }}>
                  <Button type="primary" htmlType="submit" loading={loading} block>
                    登 录
                  </Button>
                </Form.Item>
              </Form>
            ) : (
              <Form
                name="totp"
                onFinish={onTotpFinish}
                autoComplete="off"
                size="large"
                style={{ marginTop: 28 }}
              >
                <Form.Item name="code" rules={[{ required: true, message: '请输入验证码' }]}>
                  <Input
                    placeholder="6 位验证码"
                    maxLength={6}
                    style={{ letterSpacing: 8, textAlign: 'center', fontSize: 18 }}
                  />
                </Form.Item>
                <Form.Item style={{ marginBottom: 8 }}>
                  <Button type="primary" htmlType="submit" block>
                    验 证
                  </Button>
                </Form.Item>
                <Button
                  type="link"
                  block
                  onClick={() => {
                    setTotpStep(false)
                    setChallengeId(null)
                  }}
                >
                  返回登录
                </Button>
              </Form>
            )}

            <div className="login-footer">
              © {new Date().getFullYear()} Go Admin Kit · All rights reserved
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
