import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { changePassword } from '@/api/auth'
import { message } from '@/utils/feedback'

export default function MobilePasswordPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!oldPassword || !newPassword) {
      message.error(t('请输入当前密码'))
      return
    }
    if (newPassword.length < 6) {
      message.error(t('密码至少 6 位'))
      return
    }
    if (newPassword !== confirmPassword) {
      message.error(t('两次输入的密码不一致'))
      return
    }
    setSubmitting(true)
    try {
      await changePassword({ old_password: oldPassword, new_password: newPassword })
      message.success(t('密码修改成功'))
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
      navigate('/m/me', { replace: true })
    } catch {
      message.error(t('密码修改失败，请检查当前密码是否正确'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="m-page">
      <form className="m-sheet-card is-static" onSubmit={(event) => void handleSubmit(event)}>
        <h3>{t('修改密码')}</h3>
        <p className="m-meta">{t('改完立即生效，下次登录用新密码')}</p>
        <label className="m-field">
          <span>{t('当前密码')}</span>
          <input
            className="m-input"
            type="password"
            autoComplete="current-password"
            value={oldPassword}
            onChange={(e) => setOldPassword(e.target.value)}
          />
        </label>
        <label className="m-field">
          <span>{t('新密码')}</span>
          <input
            className="m-input"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
          />
        </label>
        <label className="m-field">
          <span>{t('确认新密码')}</span>
          <input
            className="m-input"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
          />
        </label>
        <div className="m-actions">
          <button type="submit" className="m-primary-btn" disabled={submitting}>
            {submitting ? t('保存中…') : t('确认修改')}
          </button>
          <button type="button" className="m-text-btn" onClick={() => navigate('/m/me')}>
            {t('取消')}
          </button>
        </div>
      </form>
    </main>
  )
}
