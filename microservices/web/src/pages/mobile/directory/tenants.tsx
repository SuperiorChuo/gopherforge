import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { getTenantList } from '@/api/system/tenant'
import type { TenantInfo } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileTenantsPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getTenantList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <SimpleListPage
      title="租户"
      emptyText="暂无租户"
      load={load}
      rowKey={(row: TenantInfo) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.status === 1 ? '' : ' is-bad'}`}>
              {row.status === 1 ? t('启用') : t('停用')}
            </span>
            <span>{row.plan || row.code}</span>
          </div>
          <strong>{row.name}</strong>
          <p>{[row.code, row.max_users ? t('上限 {{n}} 人', { n: row.max_users }) : ''].filter(Boolean).join(' · ')}</p>
        </article>
      )}
    />
  )
}
