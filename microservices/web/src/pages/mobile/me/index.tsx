import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { usePermission } from '@/hooks/usePermission'
import { logout } from '@/store/slices/authSlice'
import { message } from '@/utils/feedback'
import { MOBILE_BASE, mobileLoginPath } from '@/utils/mobile-path'
import { openDesktopConsole } from '../open-desktop'
import { isNativeApp } from '../native'
import {
  canPromptInstall,
  isAppleMobile,
  isStandaloneDisplay,
  promptInstall,
} from '../pwa'

export default function MobileMePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const dispatch = useAppDispatch()
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const { isSuperAdmin } = usePermission()
  const [standalone, setStandalone] = useState(() => isStandaloneDisplay())
  const [installReady, setInstallReady] = useState(() => canPromptInstall())
  const [installing, setInstalling] = useState(false)

  useEffect(() => {
    const sync = () => {
      setStandalone(isStandaloneDisplay())
      setInstallReady(canPromptInstall())
    }
    window.addEventListener('gak-pwa-ready', sync)
    window.addEventListener('gak-pwa-installed', sync)
    return () => {
      window.removeEventListener('gak-pwa-ready', sync)
      window.removeEventListener('gak-pwa-installed', sync)
    }
  }, [])

  const handleInstall = async () => {
    setInstalling(true)
    try {
      const outcome = await promptInstall()
      if (outcome === 'accepted') {
        message.success(t('已添加到主屏幕'))
        setStandalone(true)
      } else if (outcome === 'unavailable') {
        message.info(t('请用浏览器菜单把本页添加到主屏幕'))
      }
    } finally {
      setInstalling(false)
      setInstallReady(canPromptInstall())
    }
  }

  const handleLogout = async () => {
    await dispatch(logout())
    navigate(mobileLoginPath(MOBILE_BASE), { replace: true })
  }

  return (
    <main className="m-page">
      <article className="m-sheet-card is-static">
        <h3>{userInfo?.nickname || userInfo?.username}</h3>
        <p className="m-meta">
          {isSuperAdmin ? t('管理员') : userInfo?.username}
          {userInfo?.email ? ` · ${userInfo.email}` : ''}
        </p>
        <p className="m-meta">
          {t('二次验证')} · {userInfo?.totp_enabled ? t('已开启') : t('未开启')}
        </p>
      </article>

      <div className="m-list">
        <button type="button" className="m-row" onClick={() => navigate('/m/me/password')}>
          <strong>{t('修改密码')}</strong>
          <p>{t('改完立即生效，下次登录用新密码')}</p>
        </button>
        <button type="button" className="m-row" onClick={() => navigate('/m/me/totp')}>
          <strong>{t('二次验证')}</strong>
          <p>{userInfo?.totp_enabled ? t('已开启，可关闭或重出恢复码') : t('未开启，可在手机上绑定')}</p>
        </button>
        <button type="button" className="m-row" onClick={() => navigate('/m/me/logins')}>
          <strong>{t('最近登录')}</strong>
          <p>{t('查看本账号登录记录')}</p>
        </button>
        {isNativeApp() ? (
          <div className="m-row is-static">
            <strong>{t('已安装原生壳')}</strong>
            <p>{t('内容仍走现网 /m，热更新不用重装')}</p>
          </div>
        ) : standalone ? (
          <div className="m-row is-static">
            <strong>{t('已添加到主屏幕')}</strong>
            <p>{t('下次从图标打开就是独立窗口')}</p>
          </div>
        ) : installReady ? (
          <button type="button" className="m-row" disabled={installing} onClick={() => void handleInstall()}>
            <strong>{t('添加到主屏幕')}</strong>
            <p>{t('像 App 一样从桌面打开')}</p>
          </button>
        ) : (
          <div className="m-row is-static">
            <strong>{t('添加到主屏幕')}</strong>
            <p>
              {isAppleMobile()
                ? t('Safari 点分享，再选添加到主屏幕')
                : t('浏览器菜单里选「安装应用」或「添加到主屏幕」')}
            </p>
          </div>
        )}
        <button type="button" className="m-row" onClick={() => openDesktopConsole('/profile')}>
          <strong>{t('打开完整控制台')}</strong>
          <p>{t('用户、角色、菜单等配置在电脑上完成')}</p>
        </button>
      </div>

      <button type="button" className="m-danger-btn" onClick={() => void handleLogout()}>
        {t('退出登录')}
      </button>
    </main>
  )
}
