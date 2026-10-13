import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Space, Timeline } from 'antd'
import { useTranslation } from 'react-i18next'
import { getAgentRun, type AgentRunDetails } from '@/api/ai/agent'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'

type EventPage = Pick<AgentRunDetails, 'events' | 'events_has_more' | 'events_next_cursor'> & { afterCursor: number }

/** 仅保留当前200条；翻页失败保留旧批且可重试，SSE刷新不把正在查看的后续批重置回首批。 */
export default function RunEvents({ details }: { details: AgentRunDetails }) {
  const { t } = useTranslation()
  const [page, setPage] = useState<EventPage | null>(null)
  const [loading, setLoading] = useState(false)
  const [failedCursor, setFailedCursor] = useState<number | null>(null)
  const requestID = useRef(0)
  const busy = useRef(false)
  const current = page ?? details
  useEffect(() => () => { requestID.current += 1 }, [])

  const load = async (cursor: number) => {
    if (busy.current) return
    busy.current = true
    const id = ++requestID.current
    setLoading(true)
    setFailedCursor(null)
    try {
      const next = await getAgentRun(details.run.id, cursor)
      if (id !== requestID.current) return
      // 不能把错误Run或未前进的游标拼进当前账本；不静默吞掉分页契约漂移。
      if (next.run.id !== details.run.id || next.events.length > 200
        || next.events.some(event => event.run_id !== details.run.id || event.id <= cursor)
        || (next.events_has_more && next.events_next_cursor <= cursor)) throw new Error('invalid event page')
      setPage({ events: next.events, events_has_more: next.events_has_more, events_next_cursor: next.events_next_cursor, afterCursor: cursor })
    } catch {
      if (id === requestID.current) setFailedCursor(cursor)
    } finally {
      if (id === requestID.current) { busy.current = false; setLoading(false) }
    }
  }

  return <section aria-label={t('事件分页')}>
    <Space wrap style={{ marginBottom: 16 }}>
      <span className="cell-muted">{t('本批 {{n}} 条事件', { n: current.events.length })}</span>
      {page && <>
        <Button size="small" disabled={loading} onClick={() => { setPage(null); setFailedCursor(null) }}>{t('返回首批')}</Button>
        {/* 尾批has_more=false也允许原位探测后来追加的事件，不需要退回首批重走。 */}
        <Button size="small" disabled={loading} onClick={() => void load(page.afterCursor)}>{t('刷新当前批次')}</Button>
      </>}
      <Button size="small" disabled={!current.events_has_more || loading} loading={loading} onClick={() => void load(current.events_next_cursor)}>{t('下一批事件')}</Button>
    </Space>
    {failedCursor !== null && <Alert type="error" showIcon message={t('事件加载失败，已保留当前批次')} style={{ marginBottom: 16 }}
      action={<Button size="small" disabled={loading} onClick={() => void load(failedCursor)}>{t('重试')}</Button>} />}
    {current.events.length ? <Timeline items={current.events.map(event => ({
      color: event.type.startsWith('run') ? 'blue' : 'green',
      content: <span><span className="cell-mono">{event.type}</span><span className="cell-muted"> · {formatDateTime(event.occurred_at)}</span></span>,
    }))} /> : <GlassEmpty text={t('尚无事件')} compact />}
  </section>
}
