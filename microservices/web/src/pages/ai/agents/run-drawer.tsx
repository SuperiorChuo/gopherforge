import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Descriptions, Drawer, Space, Spin, Table, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined, StopOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  cancelAgentRun,
  getAgentRun,
  watchAgentRunEvents,
  type AgentRunDetails,
  type AgentStep,
} from '@/api/ai/agent'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { canCancelRun, runStatusMeta, stepStatusMeta } from './_shared'
import RunEvents from './run-events'

const ACTIVE_STATUSES = new Set(['queued', 'running', 'cancelling'])

/** 事件流驱动的详情刷新最小间隔：后端单轮可推 200 条事件，逐帧刷新会变成请求风暴。 */
const SSE_REFRESH_MIN_INTERVAL_MS = 1000

interface RunDrawerProps {
  runId: number | null
  open: boolean
  onClose: () => void
  onChanged?: () => void
}

/**
 * Run 详情抽屉：基本信息 + 步骤表 + 事件时间线。
 * 活跃 Run 挂一条 SSE 长流（服务端 500ms 轮询账本，done 后关流）；
 * 断线不自动重连，回落到手动刷新，避免前端重连风暴。
 */
export default function RunDrawer({ runId, open, onClose, onChanged }: RunDrawerProps) {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canRun = hasPerm('ai:agent:run')

  const [details, setDetails] = useState<AgentRunDetails | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const refreshIDRef = useRef(0)
  const [live, setLive] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const streamRef = useRef<AbortController | null>(null)
  // 事件流刷新节流的计时器与上次刷新时刻（声明在使用它们的 stopStream 之前，避免阅读时误判先后）。
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastRefreshAtRef = useRef(0)

  const stopStream = useCallback(() => {
    streamRef.current?.abort()
    streamRef.current = null
    if (refreshTimerRef.current !== null) {
      clearTimeout(refreshTimerRef.current)
      refreshTimerRef.current = null
    }
    setLive(false)
  }, [])

  const refresh = useCallback(async (id: number, silent = false) => {
    const requestID = ++refreshIDRef.current
    if (!silent) setLoading(true)
    try {
      const next = await getAgentRun(id)
      if (next.run.id !== id) throw new Error('invalid run details')
      if (requestID === refreshIDRef.current) {
        setDetails(next)
        setLoadError(false)
      }
      // 迟到详情不能为已关闭或已切换的Run重新打开SSE。
      return requestID === refreshIDRef.current ? next : null
    } catch {
      // Request layer already reports the server message.
      if (requestID === refreshIDRef.current && !silent) setLoadError(true)
      return null
    } finally {
      if (requestID === refreshIDRef.current) setLoading(false)
    }
  }, [])

  // 事件流的刷新节流（前沿+尾沿）。后端单轮可一次推最多 200 条事件，逐帧各拉一次全量详情
  // 会瞬间产生 200 个并发请求且只有最后一个有用；节流后至多每 SSE_REFRESH_MIN_INTERVAL_MS 一次，
  // 尾沿保证最后一条仍会落到界面上。
  const scheduleStreamRefresh = useCallback((id: number) => {
    const elapsed = Date.now() - lastRefreshAtRef.current
    if (elapsed >= SSE_REFRESH_MIN_INTERVAL_MS) {
      lastRefreshAtRef.current = Date.now()
      void refresh(id, true)
      return
    }
    if (refreshTimerRef.current !== null) return
    refreshTimerRef.current = setTimeout(() => {
      refreshTimerRef.current = null
      lastRefreshAtRef.current = Date.now()
      void refresh(id, true)
    }, SSE_REFRESH_MIN_INTERVAL_MS - elapsed)
  }, [refresh])

  const ensureStream = useCallback(
    (runDetails: AgentRunDetails) => {
      if (streamRef.current) return
      if (!ACTIVE_STATUSES.has(runDetails.run.status)) return
      const controller = new AbortController()
      streamRef.current = controller
      setLive(true)
      const lastEventId = runDetails.events.length
        ? Math.max(...runDetails.events.map((event) => event.id))
        : undefined
      void watchAgentRunEvents(runDetails.run.id, lastEventId, controller.signal, (frame) => {
        if (streamRef.current !== controller) return
        if (frame.event === 'error') {
          message.warning(t('事件流中断，可手动刷新查看最新状态'))
          stopStream()
          return
        }
        if (frame.event === 'done') {
          void refresh(runDetails.run.id, true).finally(() => {
            // 用户可能在刷新完成前切换到另一个 Run；不能误停新流。
            if (streamRef.current !== controller) return
            stopStream()
            onChanged?.()
          })
          return
        }
        scheduleStreamRefresh(runDetails.run.id)
      }).catch(() => {
        // 断线（网络/网关超时）不当作错误弹窗，保留已加载数据并停止 live。
      }).finally(() => {
        // 正常 EOF 也必须收尾：网关/服务端可能在**未发 done 帧**的情况下干净收流，
        // 只挂 .catch 会让界面永远停在"实时跟踪中"，且 ensureStream 不会再重连。
        if (streamRef.current === controller) stopStream()
      })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh, stopStream, onChanged],
  )

  useEffect(() => {
    if (!open || runId === null) {
      stopStream()
      setDetails(null)
      setLoadError(false)
      refreshIDRef.current += 1
      return
    }
    stopStream()
    setDetails(null)
    setLoadError(false)
    void refresh(runId).then((next) => {
      if (next) ensureStream(next)
    })
    return () => { refreshIDRef.current += 1; stopStream() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, runId])

  const handleCancel = async () => {
    if (runId === null) return
    setCancelling(true)
    try {
      await cancelAgentRun(runId)
      message.success(t('已请求取消'))
      await refresh(runId, true)
      onChanged?.()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setCancelling(false)
    }
  }

  const stepColumns: ColumnsType<AgentStep> = [
    { title: '#', dataIndex: 'ordinal', width: 48 },
    {
      title: t('类型'),
      dataIndex: 'kind',
      width: 80,
      render: (kind: string) => <Tag color={kind === 'model' ? 'geekblue' : 'cyan'}>{kind === 'model' ? t('模型') : t('工具')}</Tag>,
    },
    { title: t('名称'), dataIndex: 'name', ellipsis: true },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 96,
      render: (status: AgentStep['status']) => {
        const meta = stepStatusMeta[status]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} /> : status
      },
    },
    {
      title: 'Tokens',
      width: 110,
      render: (_, record) => (
        <span className="cell-mono cell-muted">
          {record.input_tokens}/{record.output_tokens}
        </span>
      ),
    },
    { title: t('开始'), dataIndex: 'started_at', width: 160, render: (v: string) => formatDateTime(v) },
    {
      title: t('错误'),
      dataIndex: 'error_message',
      ellipsis: true,
      render: (v: string) => (v ? <span style={{ color: '#cf1322' }}>{v}</span> : '—'),
    },
  ]

  const run = details?.run
  const statusMeta = run ? runStatusMeta[run.status as keyof typeof runStatusMeta] : undefined

  return (
    <Drawer
      title={run ? t('Run #{{id}}', { id: run.id }) : t('Run 详情')}
      open={open}
      onClose={onClose}
      width={820}
      extra={
        <Space>
          {run && ACTIVE_STATUSES.has(run.status) && (
            <span className="cell-muted">{live ? t('实时跟踪中…') : t('未连接实时流')}</span>
          )}
          <Button icon={<ReloadOutlined />} onClick={() => runId !== null && void refresh(runId)} loading={loading} />
          {run && canCancelRun(run.status) && canRun && (
            <Tooltip title={t('请求取消该 Run')}>
              <Button danger icon={<StopOutlined />} loading={cancelling} onClick={() => void handleCancel()}>
                {t('取消')}
              </Button>
            </Tooltip>
          )}
        </Space>
      }
    >
      {loading && !details && <Spin style={{ display: 'block', margin: '48px auto' }} />}
      {loadError && !details && (
        <Alert
          type="error"
          showIcon
          message={t('运行详情加载失败')}
          action={<Button size="small" onClick={() => runId !== null && void refresh(runId)}>{t('重试')}</Button>}
        />
      )}
      {loadError && details && (
        <Alert type="warning" showIcon message={t('已保留旧数据，最新状态刷新失败')} style={{ marginBottom: 16 }} />
      )}
      {run && details && (
        <>
          <Space size={12} wrap style={{ marginBottom: 16 }}>
            {statusMeta && <StatusPill tone={statusMeta.tone} label={statusMeta.label} pulse={ACTIVE_STATUSES.has(run.status)} />}
            <Tag>{t('版本 v{{v}}', { v: details.version.version })}</Tag>
            {run.attempt > 1 && <Tag color="orange">{t('第 {{n}} 次尝试', { n: run.attempt })}</Tag>}
            {run.error_code && <Tag color="red">{run.error_code}</Tag>}
          </Space>
          <Descriptions
            size="small"
            column={2}
            style={{ marginBottom: 16 }}
            items={[
              { key: 'agent', label: t('Agent'), children: details.definition.name },
              { key: 'created', label: t('创建时间'), children: formatDateTime(run.created_at) },
              { key: 'started', label: t('开始'), children: run.started_at ? formatDateTime(run.started_at) : '—' },
              { key: 'finished', label: t('结束'), children: run.finished_at ? formatDateTime(run.finished_at) : '—' },
            ]}
          />
          <Typography.Paragraph copyable style={{ whiteSpace: 'pre-wrap', marginBottom: 16 }} className="cell-muted">
            {run.input}
          </Typography.Paragraph>
          {(run.output || run.error_message) && (
            <Alert
              type={run.status === 'failed' ? 'error' : 'success'}
              showIcon
              message={run.status === 'failed' ? t('失败') : t('输出')}
              description={<div style={{ whiteSpace: 'pre-wrap' }}>{run.output || run.error_message}</div>}
              style={{ marginBottom: 16 }}
            />
          )}

          <Typography.Title level={5}>{t('步骤')}</Typography.Title>
          <Table
            rowKey="id"
            size="small"
            columns={stepColumns}
            dataSource={details.steps}
            pagination={false}
            scroll={{ x: 720 }}
            locale={{ emptyText: <GlassEmpty text={t('尚无步骤')} compact /> }}
            style={{ marginBottom: 16 }}
          />

          <Typography.Title level={5}>{t('事件')}</Typography.Title>
          <RunEvents key={run.id} details={details} />
        </>
      )}
    </Drawer>
  )
}
