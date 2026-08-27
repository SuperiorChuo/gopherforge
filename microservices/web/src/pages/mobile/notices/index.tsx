import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getNoticeList } from '@/api/system/notice'
import type { Notice } from '@/types'
import SimpleListPage from '../SimpleListPage'

const TYPE_LABEL: Record<number, string> = { 1: '通知', 2: '公告' }

function lifecycleLabel(row: Notice): string {
  if (row.status !== 1) return '已停用'
  const now = dayjs()
  const start = row.start_time ? dayjs(row.start_time) : null
  const end = row.end_time ? dayjs(row.end_time) : null
  if (start?.isValid() && start.isAfter(now)) return '待生效'
  if (end?.isValid() && end.isBefore(now)) return '已结束'
  if (!start && !end) return '长期有效'
  return '生效中'
}

export default function MobileNoticesPage() {
  const { t } = useTranslation()
  const [open, setOpen] = useState<Notice | null>(null)

  const load = useCallback(async () => {
    const res = await getNoticeList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  return (
    <>
      <SimpleListPage
        title="公告"
        emptyText="暂无公告"
        load={load}
        rowKey={(row: Notice) => row.id}
        render={(row) => {
          const life = lifecycleLabel(row)
          return (
            <button type="button" className="m-row" onClick={() => setOpen(row)}>
              <div className="m-row-top">
                <span className={`m-pill${life === '已停用' || life === '已结束' ? ' is-bad' : ''}`}>
                  {t(life)}
                </span>
                <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.title || t('无标题')}</strong>
              <p>{t(TYPE_LABEL[row.type] ?? '公告')}</p>
            </button>
          )
        }}
      />
      {open && (
        <div className="m-sheet" role="dialog" aria-modal="true" aria-labelledby="m-notice-title">
          <div className="m-sheet-card">
            <div className="m-row-top">
              <span className="m-pill">{t(TYPE_LABEL[open.type] ?? '公告')}</span>
              <span>{t(lifecycleLabel(open))}</span>
            </div>
            <h3 id="m-notice-title">{open.title || t('无标题')}</h3>
            <p className="m-sheet-body">{open.content || t('暂无正文')}</p>
            <p className="m-meta">
              {[
                open.start_time ? `${t('开始')} ${dayjs(open.start_time).format('MM-DD HH:mm')}` : '',
                open.end_time ? `${t('结束')} ${dayjs(open.end_time).format('MM-DD HH:mm')}` : '',
              ].filter(Boolean).join(' · ') || t('长期有效')}
            </p>
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
