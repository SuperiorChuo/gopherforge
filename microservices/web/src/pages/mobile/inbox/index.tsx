import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { CheckOutlined, ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  getNotifyUnreadCount,
  listNotifyMessages,
  markAllNotifyRead,
  markNotifyRead,
  type NotifyMessage,
} from '@/api/notify'
import { message } from '@/utils/feedback'

const TYPE_LABEL: Record<string, string> = {
  notice: '通知',
  ticket: '工单',
  im: 'IM',
  system: '系统',
}

function isHttpLink(link: string) {
  return link.startsWith('http://') || link.startsWith('https://')
}

export default function MobileInboxPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [list, setList] = useState<NotifyMessage[]>([])
  const [unread, setUnread] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [open, setOpen] = useState<NotifyMessage | null>(null)
  const [acting, setActing] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const [res, countRes] = await Promise.all([
        listNotifyMessages({
          page: 1,
          page_size: 30,
          unread: unreadOnly ? '1' : undefined,
        }),
        getNotifyUnreadCount(),
      ])
      setList(res?.list ?? [])
      setUnread(Number(countRes?.count ?? 0))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [unreadOnly])

  useEffect(() => {
    void load()
  }, [load])

  const markReadLocal = (id: number) => {
    setList((prev) =>
      prev.map((row) => (row.id === id && !row.read_at ? { ...row, read_at: new Date().toISOString() } : row)),
    )
    setUnread((n) => Math.max(0, n - 1))
    setOpen((cur) => (cur?.id === id && !cur.read_at ? { ...cur, read_at: new Date().toISOString() } : cur))
  }

  const openItem = async (row: NotifyMessage) => {
    setOpen(row)
    if (row.read_at) return
    try {
      await markNotifyRead(row.id)
      markReadLocal(row.id)
    } catch {
      /* 打开仍展示正文，标已读失败不挡阅读 */
    }
  }

  const handleMarkAll = async () => {
    setActing(true)
    try {
      const res = await markAllNotifyRead()
      message.success(t('已读 {{n}} 条', { n: res?.updated ?? 0 }))
      void load()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setActing(false)
    }
  }

  const openLink = (link: string) => {
    if (isHttpLink(link)) {
      window.open(link, '_blank', 'noopener,noreferrer')
      return
    }
    navigate(link.startsWith('/m') ? link : `/m`)
  }

  return (
    <main className="m-page">
      <div className="m-page-head">
        <div className="m-page-titles">
          <h2>{t('站内信')}</h2>
          <p>{t('未读 {{n}} 条', { n: unread })}</p>
        </div>
        <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void load()}>
          <ReloadOutlined />
        </button>
      </div>

      <div className="m-chip-row">
        <button
          type="button"
          className={`m-filter${unreadOnly ? '' : ' is-on'}`}
          onClick={() => setUnreadOnly(false)}
        >
          {t('全部')}
        </button>
        <button
          type="button"
          className={`m-filter${unreadOnly ? ' is-on' : ''}`}
          onClick={() => setUnreadOnly(true)}
        >
          {t('仅未读')}
        </button>
        <button type="button" className="m-link-btn" disabled={acting || unread === 0} onClick={() => void handleMarkAll()}>
          <CheckOutlined /> {t('全部已读')}
        </button>
      </div>

      {loading ? (
        <div className="m-list">
          <div className="m-row"><span className="m-skel" /></div>
          <div className="m-row"><span className="m-skel" /></div>
        </div>
      ) : error ? (
        <div className="m-empty">
          <p>{t('暂时不可用')}</p>
          <button type="button" className="m-link-btn" onClick={() => void load()}>
            {t('重试')}
          </button>
        </div>
      ) : list.length === 0 ? (
        <div className="m-empty">{t('暂无站内信')}</div>
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <button key={row.id} type="button" className={`m-row${row.read_at ? '' : ' is-unread'}`} onClick={() => void openItem(row)}>
              <div className="m-row-top">
                <span className="m-pill">{t(TYPE_LABEL[row.type] ?? '通知')}</span>
                <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.title || t('无标题')}</strong>
              {row.content ? <p>{row.content}</p> : null}
            </button>
          ))}
        </div>
      )}

      {open && (
        <div className="m-sheet" role="dialog" aria-modal="true" aria-labelledby="m-inbox-title">
          <div className="m-sheet-card">
            <div className="m-row-top">
              <span className="m-pill">{t(TYPE_LABEL[open.type] ?? '通知')}</span>
              <time>{open.created_at ? dayjs(open.created_at).format('MM-DD HH:mm') : '--'}</time>
            </div>
            <h3 id="m-inbox-title">{open.title || t('无标题')}</h3>
            <p className="m-sheet-body">{open.content || t('暂无内容')}</p>
            <div className="m-actions">
              {open.link ? (
                <button type="button" className="m-primary-btn" onClick={() => openLink(open.link!)}>
                  {t('打开')}
                </button>
              ) : null}
              <button type="button" className="m-text-btn" onClick={() => setOpen(null)}>
                {t('关闭')}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  )
}
