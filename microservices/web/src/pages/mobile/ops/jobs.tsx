import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { getTaskRuns, getTaskRunSummary, type OpsTaskRun, type TaskRunSummary } from '@/api/monitor'

export default function MobileJobsPage() {
  const { t } = useTranslation()
  const [summary, setSummary] = useState<TaskRunSummary | null>(null)
  const [list, setList] = useState<OpsTaskRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const [sum, runs] = await Promise.all([
        getTaskRunSummary(24),
        getTaskRuns({ page: 1, page_size: 20, status: 'failed' }),
      ])
      setSummary(sum)
      setList(runs.list ?? [])
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
          <h2>{t('任务中心')}</h2>
          <p>
            {summary
              ? t('近 24 小时失败 {{n}} / 共 {{m}}', { n: summary.failed, m: summary.total })
              : t('加载中…')}
          </p>
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
        <div className="m-empty">{t('近 24 小时无失败')}</div>
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <article key={row.id} className="m-row">
              <div className="m-row-top">
                <span className="m-pill is-bad">{t('失败')}</span>
                <time>{row.started_at ? dayjs(row.started_at).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.task_key || row.description || row.run_id}</strong>
              <p>{row.service}{row.error_message ? ` · ${row.error_message}` : ''}</p>
            </article>
          ))}
        </div>
      )}
    </main>
  )
}
