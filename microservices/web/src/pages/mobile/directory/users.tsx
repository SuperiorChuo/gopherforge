import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { getUserList } from '@/api/system/user'
import type { SystemUser } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileUsersPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getUserList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <SimpleListPage
      title="用户"
      emptyText="暂无用户"
      load={load}
      rowKey={(row: SystemUser) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.status === 1 ? '' : ' is-bad'}`}>
              {row.status === 1 ? t('启用') : t('停用')}
            </span>
            <span>{row.roles?.map((r) => r.name).filter(Boolean).join(' · ') || t('无角色')}</span>
          </div>
          <strong>{row.nickname || row.username}</strong>
          <p>{[row.username, row.email, row.phone].filter(Boolean).join(' · ')}</p>
        </article>
      )}
    />
  )
}
