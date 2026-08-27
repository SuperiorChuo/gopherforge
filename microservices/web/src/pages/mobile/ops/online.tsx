import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { getOnlineUserList, kickUser } from '@/api/system/online-user'
import { message } from '@/utils/feedback'
import type { OnlineUser } from '@/types'

export default function MobileOnlinePage() {
  const { t } = useTranslation()
  const [list, setList] = useState<OnlineUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [kicking, setKicking] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      setList(await getOnlineUserList())
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const handleKick = async (row: OnlineUser) => {
    if (!window.confirm(t('确认踢出 {{name}} 的这个会话？', { name: row.nickname || row.username }))) return
    setKicking(row.token_id)
    try {
      await kickUser(row.token_id)
      message.success(t('已踢出'))
      setList((prev) => prev.filter((item) => item.token_id !== row.token_id))
    } catch {
      message.error(t('操作失败'))
    } finally {
      setKicking(null)
    }
  }

  return (
    <main className="m-page">
      <div className="m-page-head">
        <div className="m-page-titles">
          <h2>{t('在线用户')}</h2>
          <p>{t('共 {{n}} 条', { n: list.length })}</p>
        </div>
        <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void load()}>
          <ReloadOutlined />
        </button>
      </div>
      {loading ? (
        <div className="m-row"><span className="m-skel" /></div>
      ) : error ? (
        <div className="m-empty">
          <p>{t('暂时不可用')}</p>
          <button type="button" className="m-link-btn" onClick={() => void load()}>{t('重试')}</button>
        </div>
      ) : list.length === 0 ? (
        <div className="m-empty">{t('暂无在线会话')}</div>
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <article key={row.token_id} className="m-row">
              <div className="m-row-top">
                <span className="m-pill">{row.os || t('会话')}</span>
                <time>{row.login_time ? dayjs(row.login_time).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.nickname || row.username}</strong>
              <p>
                {[row.ip, row.location, row.browser].filter(Boolean).join(' · ') || t('当前在线会话')}
              </p>
              <div className="m-actions">
                <button
                  type="button"
                  className="m-danger-btn"
                  disabled={kicking === row.token_id}
                  onClick={() => void handleKick(row)}
                >
                  {kicking === row.token_id ? t('加载中…') : t('踢出')}
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </main>
  )
}
