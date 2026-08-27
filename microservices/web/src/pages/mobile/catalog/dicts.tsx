import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getDictItemList, getDictTypeList } from '@/api/system/dict'
import type { DictItem, DictType } from '@/types'
import SimpleListPage from '../SimpleListPage'

export default function MobileDictsPage() {
  const { t } = useTranslation()
  const [open, setOpen] = useState<DictType | null>(null)
  const [items, setItems] = useState<DictItem[]>([])
  const [itemState, setItemState] = useState<'idle' | 'loading' | 'error'>('idle')

  const load = useCallback(async () => {
    const res = await getDictTypeList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  const openType = async (row: DictType) => {
    setOpen(row)
    setItemState('loading')
    try {
      const res = await getDictItemList(row.id, { page: 1, page_size: 50 })
      setItems(res.list ?? [])
      setItemState('idle')
    } catch {
      setItems([])
      setItemState('error')
    }
  }

  return (
    <>
      <SimpleListPage
        title="字典"
        emptyText="暂无字典"
        load={load}
        rowKey={(row: DictType) => row.id}
        render={(row) => (
          <button type="button" className="m-row" onClick={() => void openType(row)}>
            <div className="m-row-top">
              <span className={`m-pill${row.status === 1 ? '' : ' is-bad'}`}>
                {row.status === 1 ? t('启用') : t('停用')}
              </span>
              <span>{row.code}</span>
            </div>
            <strong>{row.name}</strong>
            <p>{t('点开查看字典项，编辑回电脑')}</p>
          </button>
        )}
      />
      {open && (
        <div className="m-sheet" role="dialog" aria-modal="true" aria-labelledby="m-dict-title">
          <div className="m-sheet-card">
            <div className="m-row-top">
              <span className="m-pill">{open.code}</span>
              <span>{open.status === 1 ? t('启用') : t('停用')}</span>
            </div>
            <h3 id="m-dict-title">{open.name}</h3>
            {itemState === 'loading' ? (
              <p className="m-sheet-body">{t('加载中…')}</p>
            ) : itemState === 'error' ? (
              <p className="m-sheet-body">{t('暂时不可用')}</p>
            ) : items.length === 0 ? (
              <p className="m-sheet-body">{t('暂无字典项')}</p>
            ) : (
              <div className="m-list">
                {items.map((item) => (
                  <article key={item.id} className="m-row is-static">
                    <div className="m-row-top">
                      <span className={`m-pill${item.status === 1 ? '' : ' is-bad'}`}>{item.value}</span>
                      <span>{item.status === 1 ? t('启用') : t('停用')}</span>
                    </div>
                    <strong>{item.label}</strong>
                  </article>
                ))}
              </div>
            )}
            <div className="m-actions">
              <button type="button" className="m-text-btn" onClick={() => setOpen(null)}>
                {t('关闭')}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
