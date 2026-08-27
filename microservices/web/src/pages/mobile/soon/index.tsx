import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { WORKBENCH_GROUPS } from '../nav'
import { openDesktopConsole } from '../open-desktop'

export default function MobileSoonPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { key } = useParams()
  const fromWorkbench = WORKBENCH_GROUPS.flatMap((group) => group.tiles).find(
    (tile) => tile.to === `/m/soon/${key ?? ''}`,
  )
  const title = fromWorkbench?.label ?? '即将开通'
  const body = t('这一项会做成手机只读页，编辑仍回电脑。先占位，避免点进去 404。')

  return (
    <main className="m-page">
      <article className="m-sheet-card is-static">
        <h3>{t(title)}</h3>
        <p className="m-sheet-body">{t(body)}</p>
        <div className="m-actions">
          <button type="button" className="m-primary-btn" onClick={() => openDesktopConsole()}>
            {t('打开完整控制台')}
          </button>
          <button type="button" className="m-text-btn" onClick={() => navigate('/m/workbench')}>
            {t('返回工作台')}
          </button>
        </div>
      </article>
    </main>
  )
}
