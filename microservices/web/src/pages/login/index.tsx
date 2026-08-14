import { useCallback, useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Form, Input, Button, Spin } from 'antd'
import type { InputRef } from 'antd'
import { message } from '@/utils/feedback'
import {
  UserOutlined,
  LockOutlined,
  SafetyOutlined,
  ReloadOutlined,
  CloudOutlined,
  DownOutlined,
  UpOutlined,
  CheckOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser, login } from '@/store/slices/authSlice'
import { getCaptcha } from '@/api/auth'
import { setTokens } from '@/utils/request'
import { prefetchMainLayout } from '@/router'
import AuthShell from '@/components/auth/AuthShell'

/**
 * 读取 ?redirect= 并做开放重定向防护：只接受站内绝对路径（单个前导斜杠，
 * 拒绝 //host、http(s):// 等外站跳转）。用于登录后回到来源页（如 OAuth2 授权页）。
 */
function safeRedirectTarget(): string {
  const raw = new URLSearchParams(window.location.search).get('redirect')
  if (raw && /^\/(?!\/)/.test(raw)) {
    return raw
  }
  return '/dashboard'
}

export default function LoginPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { t } = useTranslation()

  // 登录成功后立刻要用管理台骨架，趁用户填账密的空档预取
  useEffect(() => prefetchMainLayout(), [])
  const loading = useAppSelector((s) => s.auth.loading)
  const [form] = Form.useForm()
  const [totpForm] = Form.useForm()
  const [error, setError] = useState<string | null>(null)
  // 同一条错误重复出现时靠 nonce 重挂载横幅，重新触发摇晃动画
  const [errorNonce, setErrorNonce] = useState(0)
  const [totpStep, setTotpStep] = useState(false)
  const [challengeId, setChallengeId] = useState<string | null>(null)
  const [totpLoading, setTotpLoading] = useState(false)
  const [tenantOpen, setTenantOpen] = useState(false)
  const [success, setSuccess] = useState(false)
  const [capsLock, setCapsLock] = useState(false)

  const [captchaImg, setCaptchaImg] = useState('')
  const [captchaId, setCaptchaId] = useState('')
  const [captchaLoading, setCaptchaLoading] = useState(false)
  const [captchaFlash, setCaptchaFlash] = useState(false)

  const usernameRef = useRef<InputRef>(null)
  const captchaRef = useRef<InputRef>(null)
  const totpRef = useRef<InputRef>(null)
  const totpSubmitting = useRef(false)

  const showError = useCallback((msg: string) => {
    setError(msg)
    setErrorNonce((n) => n + 1)
  }, [])

  // Caps Lock 开着打密码时给出警示
  const onPasswordKey = (e: KeyboardEvent<HTMLInputElement>) => {
    setCapsLock(e.getModifierState?.('CapsLock') ?? false)
  }

  /** 成功微过渡：按钮 ✓ + 卡片轻收，再跳转 */
  const finishWithSuccess = useCallback(() => {
    setSuccess(true)
    message.success(t('登录成功'))
    window.setTimeout(() => {
      navigate(safeRedirectTarget(), { replace: true })
    }, 280)
  }, [navigate, t])

  const refreshCaptcha = useCallback(async () => {
    setCaptchaLoading(true)
    setCaptchaFlash(true)
    try {
      const res = await getCaptcha()
      setCaptchaId(res.key)
      setCaptchaImg(res.image.startsWith('data:') ? res.image : `data:image/png;base64,${res.image}`)
      form.setFieldValue('captcha_code', '')
    } catch {
      showError(t('验证码加载失败，请点击刷新'))
    } finally {
      setCaptchaLoading(false)
      window.setTimeout(() => setCaptchaFlash(false), 450)
    }
  }, [form, showError, t])

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
      const timer = window.setTimeout(() => usernameRef.current?.focus(), 80)
      return () => window.clearTimeout(timer)
    }
  }, [totpStep])

  // 2FA：进入后聚焦
  useEffect(() => {
    if (totpStep) {
      const timer = window.setTimeout(() => totpRef.current?.focus(), 80)
      return () => window.clearTimeout(timer)
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
      if (result.requires_totp && result.totp_challenge_id) {
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
      const msg = err instanceof Error ? err.message : t('用户名或密码错误')
      showError(msg)
      refreshCaptcha()
      // 失败后把焦点送回验证码框，重试免鼠标
      window.setTimeout(() => captchaRef.current?.focus(), 80)
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
      showError(t('验证码不正确，请重试'))
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

  const header = !totpStep ? (
    <>
      <h2 className="login-form-title">{t('进入控制台')}</h2>
      <p className="login-form-sub">{t('使用企业账户继续')}</p>
    </>
  ) : (
    <>
      <div className="login-step-rail" aria-hidden="true">
        <span className="login-step done">{t('1 凭证')}</span>
        <span className="login-step-line" />
        <span className="login-step active">{t('2 二次验证')}</span>
      </div>
      <h2 className="login-form-title">{t('身份核验')}</h2>
      <p className="login-form-sub">{t('请输入身份验证器中的 6 位动态码')}</p>
    </>
  )

  return (
    <AuthShell header={header} shellClassName={success ? 'is-success' : undefined}>
      {error && (
        <div className="login-error" role="alert" key={errorNonce}>
          <span className="login-error-text">{error}</span>
          <button
            type="button"
            className="login-error-close"
            aria-label={t('关闭错误提示')}
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
              <span>{t('切换组织')}</span>
              {tenantOpen ? <UpOutlined /> : <DownOutlined />}
            </button>
            {tenantOpen && (
              <Form.Item name="tenant_code" className="login-tenant-field">
                <Input
                  prefix={<CloudOutlined />}
                  placeholder={t('组织标识（可选，默认 default）')}
                  aria-label={t('组织标识')}
                />
              </Form.Item>
            )}
          </div>

          <Form.Item name="username" rules={[{ required: true, message: t('请输入用户名') }]}>
            <Input
              ref={usernameRef}
              prefix={<UserOutlined />}
              placeholder={t('用户名')}
              aria-label={t('用户名')}
              autoComplete="username"
            />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: t('请输入密码') }]}>
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={t('密码')}
              aria-label={t('密码')}
              autoComplete="current-password"
              onKeyDown={onPasswordKey}
              onKeyUp={onPasswordKey}
              onBlur={() => setCapsLock(false)}
            />
          </Form.Item>
          <div className="login-forgot-row">
            <a className="login-forgot-link" href="/forgot-password">
              {t('忘记密码？')}
            </a>
          </div>
          {capsLock && (
            <div className="login-caps-hint" role="status">
              <WarningOutlined />
              {t('大写锁定已开启')}
            </div>
          )}
          <div className="login-captcha-row">
            <Form.Item
              name="captcha_code"
              rules={[{ required: true, message: t('请输入验证码') }]}
              className="login-captcha-field"
            >
              <Input
                ref={captchaRef}
                prefix={<SafetyOutlined />}
                placeholder={t('验证码')}
                maxLength={6}
                aria-label={t('验证码')}
              />
            </Form.Item>
            <button
              type="button"
              className={`login-captcha-img${captchaFlash ? ' is-flash' : ''}`}
              onClick={refreshCaptcha}
              title={t('点击刷新验证码')}
              aria-label={t('刷新验证码')}
            >
              {captchaImg && !captchaLoading ? (
                <img src={captchaImg} alt={t('图形验证码')} />
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
              {success ? t('已验证') : t('登 录')}
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
              { required: true, message: t('请输入 6 位验证码') },
              { len: 6, message: t('请输入 6 位数字') },
            ]}
          >
            <Input
              ref={totpRef}
              className="login-totp-input"
              placeholder="······"
              maxLength={6}
              inputMode="numeric"
              aria-label={t('6 位动态验证码')}
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
              {success ? t('已验证') : t('验 证')}
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
            {t('返回登录')}
          </Button>
        </Form>
      )}
    </AuthShell>
  )
}
