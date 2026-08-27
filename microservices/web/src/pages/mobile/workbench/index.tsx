import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { usePermission } from '@/hooks/usePermission'
import { WORKBENCH_GROUPS } from '../nav'
import { openDesktopConsole } from '../open-desktop'

export default function MobileWorkbenchPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { hasPerm } = usePermission()

  return (
    <main className="m-page">
      <p className="m-sub">{t('基础设施入口都在这里。编辑类操作请回电脑。')}</p>
      {WORKBENCH_GROUPS.map((group) => {
        const tiles = group.tiles.filter((tile) => hasPerm(tile.perm))
        if (!tiles.length) return null
        return (
          <section key={group.key} className="m-section">
            <h2 className="m-section-title">{t(group.title)}</h2>
            <div className="m-tile-grid">
              {tiles.map((tile) => (
                <button
                  key={tile.key}
                  type="button"
                  className={`m-tile${tile.status === 'soon' ? ' is-soon' : ''}`}
                  onClick={() => navigate(tile.to)}
                >
                  <strong>{t(tile.label)}</strong>
                  <span>{t(tile.hint)}</span>
                  {tile.status === 'soon' ? <em>{t('即将开通')}</em> : null}
                </button>
              ))}
            </div>
          </section>
        )
      })}
      <button type="button" className="m-link-btn" onClick={() => openDesktopConsole()}>
        {t('打开完整控制台')}
      </button>
    </main>
  )
}
