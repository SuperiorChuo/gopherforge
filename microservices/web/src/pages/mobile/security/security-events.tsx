import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getSecurityEventList, type SecurityEvent } from '@/api/system/security-events'
import LogListPage from './LogListPage'

const SEVERITY_LABEL: Record<string, string> = {
  info: '信息',
  warning: '警告',
  critical: '严重',
}

export default function MobileSecurityEventsPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getSecurityEventList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <LogListPage
      title="安全事件"
      emptyText="暂无安全事件"
      load={load}
      rowKey={(row: SecurityEvent) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.severity === 'info' ? '' : ' is-bad'}`}>
              {t(SEVERITY_LABEL[row.severity] ?? row.severity)}
            </span>
            <time>{row.occurred_at ? dayjs(row.occurred_at).format('MM-DD HH:mm') : '--'}</time>
          </div>
          <strong>{row.summary || row.rule}</strong>
          <p>{[row.rule, row.actor_id, row.target].filter(Boolean).join(' · ')}</p>
        </article>
      )}
    />
  )
}
