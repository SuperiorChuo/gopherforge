import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getLoginLogList } from '@/api/system/log'
import type { LoginLog } from '@/types'
import LogListPage from './LogListPage'

export default function MobileLoginLogPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getLoginLogList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <LogListPage
      title="登录日志"
      emptyText="暂无登录记录"
      load={load}
      rowKey={(row: LoginLog) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.status === 1 ? '' : ' is-bad'}`}>
              {row.status === 1 ? t('成功') : t('失败')}
            </span>
            <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
          </div>
          <strong>{row.username || t('未知用户')}</strong>
          <p>{[row.ip, row.location, row.browser, row.os].filter(Boolean).join(' · ') || row.message || '--'}</p>
        </article>
      )}
    />
  )
}
