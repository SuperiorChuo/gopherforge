import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { disableTotp, enableTotp, generateTotpSetup, regenerateTotpRecoveryCodes } from '@/api/auth'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { fetchCurrentUser } from '@/store/slices/authSlice'
import { copyToClipboard } from '@/utils/clipboard'
import { message } from '@/utils/feedback'

type Mode = 'idle' | 'enable' | 'disable' | 'regen'

export default function MobileTotpPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const dispatch = useAppDispatch()
  const enabled = !!useAppSelector((s) => s.auth.userInfo)?.totp_enabled
  const [mode, setMode] = useState<Mode>('idle')
  const [step, setStep] = useState(0)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [qrCode, setQrCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const resetForm = () => {
    setPassword('')
    setCode('')
    setQrCode('')
    setStep(0)
    setMode('idle')
  }

  const handleEnableNext = async (event: FormEvent) => {
    event.preventDefault()
    if (!password) {
      message.error(t('请输入当前密码'))
      return
    }
    setSubmitting(true)
    try {
      const res = await generateTotpSetup({ current_password: password }) as { qr_code?: string }
      setQrCode(String(res.qr_code ?? ''))
      setStep(1)
    } catch {
      message.error(t('获取二维码失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleEnableConfirm = async (event: FormEvent) => {
    event.preventDefault()
    if (!code) {
      message.error(t('请输入 6 位验证码'))
      return
    }
    setSubmitting(true)
    try {
      const res = await enableTotp({ code, current_password: password }) as { recovery_codes?: string[] } | null
      message.success(t('TOTP 已启用'))
      resetForm()
      if (res?.recovery_codes?.length) setRecoveryCodes(res.recovery_codes)
      void dispatch(fetchCurrentUser())
    } catch {
      message.error(t('验证失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDisable = async (event: FormEvent) => {
    event.preventDefault()
    if (!password || !code) {
      message.error(t('请输入验证码'))
      return
    }
    setSubmitting(true)
    try {
      await disableTotp({ code, current_password: password })
      message.success(t('TOTP 已禁用'))
      resetForm()
      void dispatch(fetchCurrentUser())
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleRegen = async (event: FormEvent) => {
    event.preventDefault()
    if (!password || !code) {
      message.error(t('请输入验证码'))
      return
    }
    setSubmitting(true)
    try {
      const res = await regenerateTotpRecoveryCodes({ code, current_password: password }) as { recovery_codes?: string[] } | null
      resetForm()
      if (res?.recovery_codes?.length) {
        setRecoveryCodes(res.recovery_codes)
      } else {
        message.success(t('恢复码已重新生成'))
      }
    } catch {
      message.error(t('操作失败，请检查验证码和密码'))
    } finally {
      setSubmitting(false)
    }
  }

  const copyCodes = async () => {
    if (!recoveryCodes) return
    const ok = await copyToClipboard(recoveryCodes.join('\n'))
    if (ok) message.success(t('已复制到剪贴板'))
    else message.error(t('复制失败，请手动选择复制'))
  }

  return (
    <main className="m-page">
      <article className="m-sheet-card is-static">
        <h3>{t('二次验证')}</h3>
        <p className="m-meta">
          {enabled
            ? t('登录时需要输入 Authenticator 动态验证码，账号受两步验证保护。')
            : t('启用后，登录除密码外还需验证器动态验证码，可有效防止密码泄露带来的风险。')}
        </p>
        <p className="m-meta">
          {t('当前状态')} · {enabled ? t('已开启') : t('未开启')}
        </p>
        {mode === 'idle' && !recoveryCodes ? (
          <div className="m-actions">
            {enabled ? (
              <>
                <button type="button" className="m-danger-btn" onClick={() => setMode('disable')}>
                  {t('禁用 TOTP')}
                </button>
                <button type="button" className="m-text-btn" onClick={() => setMode('regen')}>
                  {t('重新生成恢复码')}
                </button>
              </>
            ) : (
              <button type="button" className="m-primary-btn" onClick={() => setMode('enable')}>
                {t('启用 TOTP')}
              </button>
            )}
            <button type="button" className="m-text-btn" onClick={() => navigate('/m/me')}>
              {t('返回')}
            </button>
          </div>
        ) : null}
      </article>

      {mode === 'enable' && (
        <form
          className="m-sheet-card is-static"
          onSubmit={(event) => void (step === 0 ? handleEnableNext(event) : handleEnableConfirm(event))}
        >
          <h3>{step === 0 ? t('验证身份') : t('扫描二维码')}</h3>
          {step === 0 ? (
            <label className="m-field">
              <span>{t('当前密码')}</span>
              <input
                className="m-input"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
          ) : (
            <>
              {qrCode ? (
                <div className="m-qr-box">
                  <img src={qrCode} alt={t('TOTP 二维码')} width={200} height={200} />
                </div>
              ) : null}
              <label className="m-field">
                <span>{t('验证码')}</span>
                <input
                  className="m-input"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </label>
            </>
          )}
          <div className="m-actions">
            {step === 1 ? (
              <button type="button" className="m-text-btn" onClick={() => setStep(0)}>
                {t('上一步')}
              </button>
            ) : (
              <button type="button" className="m-text-btn" onClick={resetForm}>
                {t('取消')}
              </button>
            )}
            <button type="submit" className="m-primary-btn" disabled={submitting}>
              {step === 0 ? t('下一步') : t('确认启用')}
            </button>
          </div>
        </form>
      )}

      {mode === 'disable' && (
        <form className="m-sheet-card is-static" onSubmit={(event) => void handleDisable(event)}>
          <h3>{t('禁用 TOTP')}</h3>
          <label className="m-field">
            <span>{t('TOTP 验证码')}</span>
            <input
              className="m-input"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </label>
          <label className="m-field">
            <span>{t('当前密码')}</span>
            <input
              className="m-input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <div className="m-actions">
            <button type="submit" className="m-danger-btn" disabled={submitting}>
              {t('确认关闭')}
            </button>
            <button type="button" className="m-text-btn" onClick={resetForm}>
              {t('取消')}
            </button>
          </div>
        </form>
      )}

      {mode === 'regen' && (
        <form className="m-sheet-card is-static" onSubmit={(event) => void handleRegen(event)}>
          <h3>{t('重新生成恢复码')}</h3>
          <p className="m-meta">{t('重新生成后，旧的恢复码将全部失效。')}</p>
          <label className="m-field">
            <span>{t('TOTP 验证码')}</span>
            <input
              className="m-input"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </label>
          <label className="m-field">
            <span>{t('当前密码')}</span>
            <input
              className="m-input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <div className="m-actions">
            <button type="submit" className="m-primary-btn" disabled={submitting}>
              {t('确认生成')}
            </button>
            <button type="button" className="m-text-btn" onClick={resetForm}>
              {t('取消')}
            </button>
          </div>
        </form>
      )}

      {recoveryCodes && (
        <article className="m-sheet-card is-static">
          <h3>{t('请保存您的恢复码')}</h3>
          <p className="m-meta">{t('恢复码仅显示这一次。丢失验证器设备时，它是找回账号的唯一途径，请妥善离线保存。')}</p>
          <div className="m-code-grid">
            {recoveryCodes.map((item) => (
              <code key={item}>{item}</code>
            ))}
          </div>
          <div className="m-actions">
            <button type="button" className="m-primary-btn" onClick={() => void copyCodes()}>
              {t('复制全部')}
            </button>
            <button type="button" className="m-text-btn" onClick={() => setRecoveryCodes(null)}>
              {t('我已保存')}
            </button>
          </div>
        </article>
      )}
    </main>
  )
}
