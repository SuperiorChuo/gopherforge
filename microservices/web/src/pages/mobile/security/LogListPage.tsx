import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'

type LogListPageProps<T> = {
  title: string
  emptyText: string
  load: () => Promise<{ list: T[]; total: number }>
  render: (row: T) => ReactNode
  rowKey: (row: T) => string | number
}

export default function LogListPage<T>({ title, emptyText, load, render, rowKey }: LogListPageProps<T>) {
  const { t } = useTranslation()
  const [list, setList] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const res = await load()
      setList(res.list)
      setTotal(res.total)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [load])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return (
    <main className="m-page">
      <div className="m-page-head">
        <div className="m-page-titles">
          <h2>{t(title)}</h2>
          <p>{t('共 {{n}} 条', { n: total })}</p>
        </div>
        <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void refresh()}>
          <ReloadOutlined />
        </button>
      </div>
      {loading ? (
        <div className="m-row"><span className="m-skel" /></div>
      ) : error ? (
        <div className="m-empty">
          <p>{t('暂时不可用')}</p>
          <button type="button" className="m-link-btn" onClick={() => void refresh()}>{t('重试')}</button>
        </div>
      ) : list.length === 0 ? (
        <div className="m-empty">{t(emptyText)}</div>
      ) : (
        <div className="m-list">{list.map((row) => <div key={rowKey(row)}>{render(row)}</div>)}</div>
      )}
    </main>
  )
}
