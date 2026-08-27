import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { LeftOutlined, ReloadOutlined } from '@ant-design/icons'
import { Input } from 'antd'
import dayjs from 'dayjs'
import {
  approveTask,
  getTask,
  listTodoTasks,
  rejectTask,
  type BpmTask,
  type BpmTaskDetail,
} from '@/api/bpm'
import { message } from '@/utils/feedback'

export default function MobileTasksPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams()
  const taskId = id ? Number(id) : NaN
  const detailMode = Number.isFinite(taskId) && taskId > 0

  const [list, setList] = useState<BpmTask[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [detail, setDetail] = useState<BpmTaskDetail | null>(null)
  const [detailError, setDetailError] = useState(false)
  const [comment, setComment] = useState('')
  const [acting, setActing] = useState<'approve' | 'reject' | null>(null)

  const loadList = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const res = await listTodoTasks({ page: 1, page_size: 30 }, true)
      setList(res?.list ?? [])
      setTotal(Number(res?.total ?? 0))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadDetail = useCallback(async () => {
    if (!detailMode) return
    setDetailError(false)
    setLoading(true)
    try {
      const res = await getTask(taskId, true)
      setDetail(res)
    } catch {
      setDetail(null)
      setDetailError(true)
    } finally {
      setLoading(false)
    }
  }, [detailMode, taskId])

  useEffect(() => {
    if (detailMode) void loadDetail()
    else void loadList()
  }, [detailMode, loadDetail, loadList])

  const canApprove = !detail?.actions || detail.actions.includes('approve')
  const canReject = !detail?.actions || detail.actions.includes('reject')

  const handleApprove = async () => {
    if (!detail) return
    setActing('approve')
    try {
      const res = await approveTask(detail.task.id, comment.trim() || undefined)
      message.success(res?.instance_status === 'approved' ? t('已同意，流程审批通过') : t('已同意'))
      navigate('/m/tasks', { replace: true })
    } catch {
      message.error(t('操作失败'))
    } finally {
      setActing(null)
    }
  }

  const handleReject = async () => {
    if (!detail) return
    const reason = comment.trim()
    if (!reason) {
      message.error(t('拒绝须填写意见'))
      return
    }
    setActing('reject')
    try {
      const res = await rejectTask(detail.task.id, reason)
      message.success(res?.instance_status === 'rejected' ? t('已拒绝，流程结束') : t('已拒绝'))
      navigate('/m/tasks', { replace: true })
    } catch {
      message.error(t('操作失败'))
    } finally {
      setActing(null)
    }
  }

  if (detailMode) {
    const task = detail?.task
    const instance = detail?.instance
    return (
      <main className="m-page">
        <div className="m-page-head">
          <button type="button" className="m-icon-btn" aria-label={t('返回')} onClick={() => navigate('/m/tasks')}>
            <LeftOutlined />
          </button>
          <div className="m-page-titles">
            <h2>{t('审批详情')}</h2>
            <p>{task?.node_name || t('待办审批')}</p>
          </div>
          <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void loadDetail()}>
            <ReloadOutlined />
          </button>
        </div>

        {loading && !detail ? (
          <div className="m-row"><span className="m-skel" /></div>
        ) : detailError || !task ? (
          <div className="m-empty">
            <p>{t('暂时不可用')}</p>
            <button type="button" className="m-link-btn" onClick={() => void loadDetail()}>
              {t('重试')}
            </button>
          </div>
        ) : (
          <>
            <article className="m-sheet-card is-static">
              <h3>{task.instance_title || instance?.title || t('未命名流程')}</h3>
              <p className="m-meta">
                {t('节点')} · {task.node_name}
                {task.initiator_name ? ` · ${t('发起人')} ${task.initiator_name}` : ''}
              </p>
              <p className="m-meta">
                {task.created_at ? dayjs(task.created_at).format('YYYY-MM-DD HH:mm') : '--'}
              </p>
              {instance?.form_snapshot && Object.keys(instance.form_snapshot).length > 0 ? (
                <dl className="m-kv">
                  {Object.entries(instance.form_snapshot).slice(0, 12).map(([key, value]) => (
                    <div key={key}>
                      <dt>{key}</dt>
                      <dd>{value == null || value === '' ? '--' : String(value)}</dd>
                    </div>
                  ))}
                </dl>
              ) : (
                <p className="m-sub">{t('暂无表单快照')}</p>
              )}
            </article>

            {(canApprove || canReject) && (
              <section className="m-sheet-card is-static">
                <label className="m-card-label" htmlFor="m-task-comment">
                  {t('审批意见')}
                </label>
                <Input.TextArea
                  id="m-task-comment"
                  rows={3}
                  maxLength={200}
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder={t('拒绝须填写意见')}
                />
                <div className="m-actions">
                  {canApprove ? (
                    <button
                      type="button"
                      className="m-primary-btn"
                      disabled={acting !== null}
                      onClick={() => void handleApprove()}
                    >
                      {acting === 'approve' ? t('加载中…') : t('同意')}
                    </button>
                  ) : null}
                  {canReject ? (
                    <button
                      type="button"
                      className="m-danger-btn"
                      disabled={acting !== null}
                      onClick={() => void handleReject()}
                    >
                      {acting === 'reject' ? t('加载中…') : t('拒绝')}
                    </button>
                  ) : null}
                </div>
              </section>
            )}
          </>
        )}
      </main>
    )
  }

  return (
    <main className="m-page">
      <div className="m-page-head">
        <div className="m-page-titles">
          <h2>{t('待办审批')}</h2>
          <p>{t('共 {{n}} 条', { n: total })}</p>
        </div>
        <button type="button" className="m-icon-btn" aria-label={t('刷新')} onClick={() => void loadList()}>
          <ReloadOutlined />
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
          <button type="button" className="m-link-btn" onClick={() => void loadList()}>
            {t('重试')}
          </button>
        </div>
      ) : list.length === 0 ? (
        <div className="m-empty">{t('没有待办')}</div>
      ) : (
        <div className="m-list">
          {list.map((row) => (
            <button
              key={row.id}
              type="button"
              className="m-row"
              onClick={() => navigate(`/m/tasks/${row.id}`)}
            >
              <div className="m-row-top">
                <span className="m-pill">{row.node_name || t('待办审批')}</span>
                <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
              </div>
              <strong>{row.instance_title || t('未命名流程')}</strong>
              <p>
                {row.initiator_name ? `${t('发起人')} ${row.initiator_name}` : t('待你处理')}
              </p>
            </button>
          ))}
        </div>
      )}
    </main>
  )
}
