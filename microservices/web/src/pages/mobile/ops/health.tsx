import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'
import { getServicesHealth, type ServiceHealthRow } from '@/api/monitor'

export default function MobileHealthPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<ServiceHealthRow[]>([])
  const [healthy, setHealthy] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const res = await getServicesHealth()
      setList(res.list ?? [])
      setHealthy(res.healthy ?? res.list?.filter((row) => row.ok).length ?? 0)
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
          <h2>{t('服务健康')}</h2>
          <p>{healthy == null ? t('加载中…') : t('健康 {{n}} / {{m}}', { n: healthy, m: list.length })}</p>
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
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <article key={row.name} className="m-row">
              <div className="m-row-top">
                <span className={`m-pill${row.ok ? '' : ' is-bad'}`}>{row.ok ? t('正常') : t('异常')}</span>
                <time>{row.latency_ms} ms</time>
              </div>
              <strong>{row.name}</strong>
              {row.error ? <p>{row.error}</p> : <p>HTTP {row.http_code}</p>}
            </article>
          ))}
        </div>
      )}
    </main>
  )
}
