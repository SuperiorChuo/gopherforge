import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Spin, Avatar } from 'antd'
import { SafetyOutlined, CheckCircleOutlined, UserOutlined, MailOutlined } from '@ant-design/icons'
import { getToken } from '@/utils/request'
import { getOAuth2Authorize, postOAuth2Authorize } from '@/api/oauth2'
import type { OAuth2AuthorizeView } from '@/api/oauth2'

// scope → 面向用户的说明（授权确认页展示）
const SCOPE_LABELS: Record<string, { title: string; desc: string; icon: React.ReactNode }> = {
  profile: { title: '基本资料', desc: '读取你的用户名、昵称与头像', icon: <UserOutlined /> },
  email: { title: '邮箱地址', desc: '读取你的邮箱地址', icon: <MailOutlined /> },
}

/**
 * OAuth2 授权确认页。放在 MainLayout 之外：第三方授权流程的资源所有者
 * 不应看到管理台骨架。沿用登录页的深空/极光/玻璃拟态风格。
 */
export default function OAuth2AuthorizePage() {
  const search = window.location.search
  const [view, setView] = useState<OAuth2AuthorizeView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const autoSubmitted = useRef(false)

  const params = useMemo(() => new URLSearchParams(search), [search])

  const submit = useCallback(async (approved: boolean) => {
    setSubmitting(true)
    try {
      const res = await postOAuth2Authorize({
        client_id: params.get('client_id') ?? '',
        redirect_uri: params.get('redirect_uri') ?? '',
        response_type: params.get('response_type') ?? 'code',
        scope: params.get('scope') ?? '',
        state: params.get('state') ?? '',
        code_challenge: params.get('code_challenge') ?? '',
        code_challenge_method: params.get('code_challenge_method') ?? '',
        approved,
      })
      // 跳回第三方应用（可能是外站，用整页跳转而非 SPA 路由）
      window.location.href = res.redirect_url
    } catch (err) {
      setError(err instanceof Error ? err.message : '授权失败')
      setSubmitting(false)
    }
  }, [params])

  useEffect(() => {
    // 未登录：先去登录，带回跳参数回到本页
    if (!getToken()) {
      const back = window.location.pathname + window.location.search
      window.location.replace(`/login?redirect=${encodeURIComponent(back)}`)
      return
    }
    let alive = true
    getOAuth2Authorize(search)
      .then((v) => {
        if (!alive) return
        setView(v)
        setLoading(false)
        // 自动授权 / 已授权过：静默完成，不打扰用户
        if ((v.auto_approve || v.already_approved) && !autoSubmitted.current) {
          autoSubmitted.current = true
          submit(true)
        }
      })
      .catch((err) => {
        if (!alive) return
        setError(err instanceof Error ? err.message : '授权请求无效')
        setLoading(false)
      })
    return () => { alive = false }
  }, [search, submit])

  const renderBody = () => {
    if (loading) {
      return <div className="oauth-consent-loading"><Spin size="large" /></div>
    }
    if (error) {
      return (
        <div className="oauth-consent-error">
          <h2 className="login-form-title">无法完成授权</h2>
          <p className="login-form-sub">{error}</p>
          <p className="oauth-consent-hint">请返回应用重试，或联系应用方确认接入配置。</p>
        </div>
      )
    }
    if (!view) return null
    if (view.auto_approve || view.already_approved) {
      // 静默授权中：只显示一个过渡态
      return <div className="oauth-consent-loading"><Spin size="large" tip="正在跳转…" /></div>
    }
    const scopes = view.scopes.length ? view.scopes : ['profile']
    return (
      <>
        <div className="oauth-consent-app">
          <Avatar
            size={56}
            src={view.logo || undefined}
            icon={<SafetyOutlined />}
            style={{ background: view.logo ? undefined : 'rgba(99,102,241,.28)' }}
          />
          <h2 className="login-form-title" style={{ marginTop: 12 }}>{view.client_name}</h2>
          <p className="login-form-sub">请求访问你的 Go Admin Kit 账户</p>
          {view.description && <p className="oauth-consent-desc">{view.description}</p>}
        </div>

        <ul className="oauth-consent-scopes">
          {scopes.map((s) => {
            const meta = SCOPE_LABELS[s]
            return (
              <li key={s}>
                <span className="oauth-consent-scope-icon">{meta?.icon ?? <CheckCircleOutlined />}</span>
                <span>
                  <b>{meta?.title ?? s}</b>
                  <em>{meta?.desc ?? `授予 ${s} 权限`}</em>
                </span>
              </li>
            )
          })}
        </ul>

        <div className="oauth-consent-actions">
          <Button block size="large" onClick={() => submit(false)} disabled={submitting}>拒绝</Button>
          <Button block size="large" type="primary" loading={submitting} onClick={() => submit(true)}>
            授权
          </Button>
        </div>
        <p className="oauth-consent-foot">
          授权后，你将被带回 <code>{safeHost(view.redirect_uri)}</code>
        </p>
      </>
    )
  }

  return (
    <div className="login-page">
      <div className="login-aurora login-aurora-1" />
      <div className="login-aurora login-aurora-2" />
      <div className="login-aurora login-aurora-3" />
      <div className="login-grid" />
      <div className="oauth-consent-card login-liquid is-alive">
        <div className="oauth-consent-brand">
          <span className="login-logo-mark"><SafetyOutlined /></span>
          <span className="login-logo-name">Go Admin Kit</span>
        </div>
        {renderBody()}
      </div>
    </div>
  )
}

// 只取回调地址的 host 展示，避免长 URL 撑破卡片
function safeHost(uri: string): string {
  try {
    return new URL(uri).host
  } catch {
    return uri
  }
}
