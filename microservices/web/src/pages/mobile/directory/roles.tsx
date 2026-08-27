import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { getRoleList } from '@/api/system/role'
import type { SystemRole } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileRolesPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getRoleList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <SimpleListPage
      title="角色"
      emptyText="暂无角色"
      load={load}
      rowKey={(row: SystemRole) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className="m-pill">{row.code}</span>
            <span>{row.data_scope || t('角色')}</span>
          </div>
          <strong>{row.name}</strong>
          <p>{row.description || t('查看角色')}</p>
        </article>
      )}
    />
  )
}
