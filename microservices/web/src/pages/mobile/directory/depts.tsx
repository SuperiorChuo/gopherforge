import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { getDepartmentList } from '@/api/system/department'
import type { Department } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileDeptsPage() {
  const { t } = useTranslation()
  const load = useCallback(async () => {
    const res = await getDepartmentList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <SimpleListPage
      title="部门"
      emptyText="暂无部门"
      load={load}
      rowKey={(row: Department) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className={`m-pill${row.status === 1 ? '' : ' is-bad'}`}>
              {row.status === 1 ? t('启用') : t('停用')}
            </span>
            <span>{row.code}</span>
          </div>
          <strong>{row.name}</strong>
          <p>{[row.leader, row.phone, row.email].filter(Boolean).join(' · ') || t('查看组织')}</p>
        </article>
      )}
    />
  )
}
