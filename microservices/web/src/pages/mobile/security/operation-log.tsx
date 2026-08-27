import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getOperationLogList } from '@/api/system/log'
import type { OperationLog } from '@/types'
import LogListPage from './LogListPage'

export default function MobileOperationLogPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getOperationLogList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <LogListPage
      title="操作日志"
      emptyText="暂无操作记录"
      load={load}
      rowKey={(row: OperationLog) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.status >= 400 ? ' is-bad' : ''}`}>
              {row.method} {row.status}
            </span>
            <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
          </div>
          <strong>{row.path}</strong>
          <p>
            {[row.username, row.module, row.action, row.ip].filter(Boolean).join(' · ') || t('操作日志')}
            {row.error_msg ? ` · ${row.error_msg}` : ''}
          </p>
        </article>
      )}
    />
  )
}
