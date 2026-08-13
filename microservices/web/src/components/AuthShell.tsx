import type { MouseEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  MoonOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  SunOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { useLocale } from '@/i18n/LocaleContext'
import { useThemeMode } from '@/theme/ThemeContext'

export interface AuthShellProps {
  /** 完整自定义标题区（如登录 2FA 步骤条）；优先于 title/subtitle */
  header?: ReactNode
  title?: ReactNode
  subtitle?: ReactNode
  children: ReactNode
  /** 附加到 login-shell，如 is-success */
  shellClassName?: string
  /** 覆盖默认 © 页脚；传 null 隐藏 */
  footer?: ReactNode | null
}

/**
 * 未登录认证页公共壳：极光背景 + 品牌栏 + 表单栏 + 主题/语言切换。
 * 登录 / 忘记密码 / 注册 / 重置密码共用，避免卫星页掉回旧 Card 壳。
 */
export default function AuthShell({
  header,
  title,
  subtitle,
  children,
  shellClassName,
  footer,
}: AuthShellProps) {
  const { t } = useTranslation()
  const { mode, toggle: toggleTheme } = useThemeMode()
  const { locale, setLocale } = useLocale()
  const year = new Date().getFullYear()

  const onThemeClick = (event: MouseEvent<HTMLButtonElement>) => {
    if (event.clientX || event.clientY) {
      toggleTheme({ x: event.clientX, y: event.clientY })
      return
    }
    const rect = event.currentTarget.getBoundingClientRect()
    toggleTheme({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
  }

  return (
    <div className="login-page">
      <div className="login-aurora login-aurora-1" />
      <div className="login-aurora login-aurora-2" />
      <div className="login-aurora login-aurora-3" />
      <div className="login-grid" />

      <div className="login-chrome-toggles">
        <button
          type="button"
          className="login-theme-toggle login-locale-toggle"
          title={t('切换语言')}
          aria-label={t('切换语言')}
          onClick={() => setLocale(locale === 'en' ? 'zh' : 'en')}
        >
          {locale === 'zh' ? 'EN' : '中文'}
        </button>
        <button
          type="button"
          className="login-theme-toggle"
          title={mode === 'dark' ? t('切换亮色') : t('切换深色')}
          aria-label={mode === 'dark' ? t('切换亮色') : t('切换深色')}
          onClick={onThemeClick}
        >
          {mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
        </button>
      </div>

      <div className={['login-shell', 'login-liquid', 'is-alive', shellClassName].filter(Boolean).join(' ')}>
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
              {t('以工程之美，')}
              <br />
              {t('驱动')}
              <em>{t('企业级')}</em>
              {t('管理')}
            </h1>
            <p className="login-subline">
              {t('以克制的架构与现代交互，')}
              <br />
              {t('构筑可托付的企业级中台。')}
            </p>
          </div>

          <ul className="login-features">
            <li>
              <span className="login-feature-icon">
                <ThunderboltOutlined />
              </span>
              {t('Go 与 React 协同 · 从容承载复杂业务')}
            </li>
            <li>
              <span className="login-feature-icon">
                <SafetyCertificateOutlined />
              </span>
              {t('权限精密可控 · 身份双重守护')}
            </li>
            <li>
              <span className="login-feature-icon">
                <RadarChartOutlined />
              </span>
              {t('全程可观测 · 每一次操作皆可追溯')}
            </li>
          </ul>
        </div>

        <div className="login-form-panel">
          <div className="login-form-inner">
            <p className="login-mobile-tagline">{t('以工程之美，驱动企业级管理')}</p>
            {header ?? (
              <>
                {title != null ? <h2 className="login-form-title">{title}</h2> : null}
                {subtitle != null ? <p className="login-form-sub">{subtitle}</p> : null}
              </>
            )}
            {children}
            {footer === null ? null : (
              <div className="login-footer">{footer ?? `© ${year} Go Admin Kit`}</div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
