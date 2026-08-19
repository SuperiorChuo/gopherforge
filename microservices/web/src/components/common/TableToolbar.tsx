import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { TABLE_TOOLBAR_PRESETS } from './tableToolbarPresets'

interface TableToolbarProps {
  title: string
  total?: number
  extra?: ReactNode
  /** 覆盖预设徽章图标；标题不在预设表且不传时退回渐变竖线 */
  icon?: ReactNode
  gradient?: string
  glow?: string
  description?: string
}

export default function TableToolbar({
  title, total, extra, icon, gradient, glow, description,
}: TableToolbarProps) {
  const { t } = useTranslation()
  const preset = TABLE_TOOLBAR_PRESETS[title]
  const badgeIcon = icon ?? preset?.icon
  const badgeGradient = gradient ?? preset?.gradient
  const badgeGlow = glow ?? preset?.glow
  const desc = description ?? preset?.description

  return (
    <div className="table-toolbar">
      <div className={`table-toolbar-title ${badgeIcon ? 'table-toolbar-title-iconed' : ''}`}>
        {badgeIcon && (
          <span
            className="table-toolbar-badge"
            style={{ background: badgeGradient, '--badge-glow': badgeGlow } as React.CSSProperties}
          >
            {badgeIcon}
          </span>
        )}
        <span className="table-toolbar-text">
          <span className="table-toolbar-heading">
            {t(title)}
            {typeof total === 'number' && <span className="table-count">{total}</span>}
          </span>
          {desc && <span className="table-toolbar-desc">{t(desc)}</span>}
        </span>
      </div>
      {extra && <div className="table-toolbar-extra">{extra}</div>}
    </div>
  )
}
