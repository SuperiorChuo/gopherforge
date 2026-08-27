import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getMyLoginLogs } from '@/api/system/log'
import type { LoginLog } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileMyLoginsPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getMyLoginLogs({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <SimpleListPage
      title="最近登录"
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
          <strong>{[row.ip, row.location].filter(Boolean).join(' · ') || t('未知来源')}</strong>
          <p>{[row.browser, row.os, row.message].filter(Boolean).join(' · ') || '--'}</p>
        </article>
      )}
    />
  )
}
