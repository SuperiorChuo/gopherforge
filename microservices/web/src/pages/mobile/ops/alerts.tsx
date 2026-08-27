import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { getAlertRules, type MonitorAlertRule } from '@/api/monitor'

export default function MobileAlertsPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<MonitorAlertRule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const res = await getAlertRules({ page: 1, page_size: 30, state: 'firing' })
      setList(res.list ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <main className="m-page">
      <div className="m-page-head">
        <div className="m-page-titles">
          <h2>{t('告警')}</h2>
          <p>{t('正在告警 {{n}} 条', { n: list.length })}</p>
        </div>
        <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void load()}>
          <ReloadOutlined />
        </button>
      </div>
      {loading ? (
        <div className="m-row"><span className="m-skel" /></div>
      ) : error ? (
        <div className="m-empty">
          <p>{t('暂时不可用')}</p>
          <button type="button" className="m-link-btn" onClick={() => void load()}>{t('重试')}</button>
        </div>
      ) : list.length === 0 ? (
        <div className="m-empty">{t('当前无告警')}</div>
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <article key={row.id} className="m-row">
              <div className="m-row-top">
                <span className="m-pill is-bad">{t('告警中')}</span>
                <time>{row.firing_since ? dayjs(row.firing_since).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.name}</strong>
              <p>
                {row.metric} {row.operator} {row.threshold}
                {row.last_value != null ? ` · ${row.last_value}` : ''}
              </p>
            </article>
          ))}
        </div>
      )}
    </main>
  )
}
