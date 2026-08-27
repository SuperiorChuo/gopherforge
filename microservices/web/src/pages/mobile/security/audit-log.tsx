import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getAuditLogList, type AuditLog } from '@/api/system/audit-log'
import LogListPage from './LogListPage'

export default function MobileAuditLogPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getAuditLogList({ page: 1, page_size: 30 })
    return { list: res.items ?? [], total: res.pagination?.total ?? 0 }
  }, [])

  return (
    <LogListPage
      title="审计日志"
      emptyText="暂无审计记录"
      load={load}
      rowKey={(row: AuditLog) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className="m-pill">{row.action || t('审计')}</span>
            <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
          </div>
          <strong>{row.summary || `${row.target_type || ''} ${row.target_id || ''}`.trim() || t('审计日志')}</strong>
          <p>{[row.actor_type, row.actor_id, row.target_type, row.target_id].filter(Boolean).join(' · ')}</p>
        </article>
      )}
    />
  )
}
