import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Descriptions, Skeleton, Space, Tag, Timeline, Typography } from 'antd'
import { ClockCircleOutlined } from '@ant-design/icons'
import {
  BPM_ACTION_META,
  BPM_INSTANCE_STATUS_META,
  collectNodeNames,
  getInstance,
  getInstanceDiagram,
  getInstanceTimeline,
  type BpmDiagram,
  type BpmInstance,
  type BpmTimelineItem,
} from '@/api/bpm'
import { displayUserName, useUserNameMap } from '@/hooks/useUserNameMap'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

// 表单快照的已知字段中文名（通用示例字段；业务方按自己的 biz_type 扩展）
const FORM_FIELD_LABELS: Record<string, string> = {
  amount_cents: '金额',
  reason: '事由',
  applicant: '申请人',
  title: '标题',
  no: '编号',
}

function formatFormValue(key: string, value: unknown): string {
  if (value === null || value === undefined || value === '') return '-'
  if (key === 'amount_cents' && typeof value === 'number') {
    return `¥${(value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
  }
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

interface BpmInstanceTimelineProps {
  /** 已知实例 id 时直接传（bpm 自有页面） */
  instanceId?: number
  /** 显示实例标题/状态/发起人头部（独立使用时开） */
  showHeader?: boolean
  /** 显示发起表单快照（实例详情页用；业务侧已有自己的表单展示则不开） */
  showForm?: boolean
  /** 实例解析成功回调（业务侧可据此联动展示） */
  onLoaded?: (instance: BpmInstance) => void
  /** BPM 服务不可用 / 查无实例时回调 —— 业务侧据此隐藏审批区块，不破坏现有页面 */
  onUnavailable?: () => void
}

/**
 * 审批实例流转时间线（可复用组件）。
 * 数据源：GET /instances/:id/timeline（流转日志）+ GET /instances/:id/diagram（当前节点标注）。
 * 全部请求 silent，任何失败走 onUnavailable 优雅降级，组件自身渲染为空。
 */
export default function BpmInstanceTimeline({
  instanceId,
  showHeader,
  showForm,
  onLoaded,
  onUnavailable,
}: BpmInstanceTimelineProps) {
  const [loading, setLoading] = useState(true)
  const [instance, setInstance] = useState<BpmInstance | null>(null)
  const [items, setItems] = useState<BpmTimelineItem[]>([])
  const [diagram, setDiagram] = useState<BpmDiagram | null>(null)
  const [unavailable, setUnavailable] = useState(false)
  const userMap = useUserNameMap()

  // 回调经 ref 透传，避免父组件内联函数触发重复加载
  const onLoadedRef = useRef(onLoaded)
  const onUnavailableRef = useRef(onUnavailable)
  onLoadedRef.current = onLoaded
  onUnavailableRef.current = onUnavailable

  useEffect(() => {
    let alive = true
    async function run() {
      setLoading(true)
      setUnavailable(false)
      try {
        const inst: BpmInstance | null = instanceId ? await getInstance(instanceId, true) : null
        if (!alive) return
        if (!inst) {
          setUnavailable(true)
          onUnavailableRef.current?.()
          return
        }
        setInstance(inst)
        onLoadedRef.current?.(inst)
        const [tl, dg] = await Promise.all([
          getInstanceTimeline(inst.id, true).catch(() => [] as BpmTimelineItem[]),
          getInstanceDiagram(inst.id, true).catch(() => null),
        ])
        if (!alive) return
        setItems(tl)
        setDiagram(dg)
      } catch {
        if (alive) {
          setUnavailable(true)
          onUnavailableRef.current?.()
        }
      } finally {
        if (alive) setLoading(false)
      }
    }
    void run()
    return () => {
      alive = false
    }
  }, [instanceId])

  if (unavailable) return null
  if (loading) return <Skeleton active paragraph={{ rows: 3 }} title={false} />
  if (!instance) return null

  const nodeNames = collectNodeNames(diagram?.node_tree)
  const statusMeta = BPM_INSTANCE_STATUS_META[instance.status] ?? {
    label: instance.status,
    color: 'default',
  }

  const timelineItems: { color?: string; children: ReactNode; dot?: ReactNode }[] = items.map((log) => {
    const meta = BPM_ACTION_META[log.action] ?? { label: log.action, color: 'blue' }
    const nodeName = log.node_name || (log.node_id ? nodeNames[log.node_id] : '')
    const comment =
      log.detail && typeof log.detail.comment === 'string' ? log.detail.comment : ''
    return {
      color: meta.color,
      children: (
        <div>
          <Space size={6} wrap>
            <Text strong>{meta.label}</Text>
            {nodeName ? <Tag>{nodeName}</Tag> : null}
            <Text type="secondary" style={{ fontSize: 12 }}>
              {log.operator_name || displayUserName(userMap, log.operator_id)} ·{' '}
              {formatDateTime(log.created_at)}
            </Text>
          </Space>
          {comment ? (
            <div style={{ marginTop: 2 }}>
              <Text type="secondary">意见：{comment}</Text>
            </div>
          ) : null}
        </div>
      ),
    }
  })

  // 审批中：追加“当前进行节点”高亮项（数据来自 diagram 的 doing 标注）
  if (instance.status === 'running') {
    const doingEntry = Object.entries(diagram?.nodes ?? {}).find(([, rt]) => rt.state === 'doing')
    const doingNodeId = doingEntry?.[0] ?? instance.current_node_id
    const doingName =
      (doingNodeId ? nodeNames[doingNodeId] : '') || instance.current_node_name || '当前节点'
    const pendingNames = (doingEntry?.[1].tasks ?? [])
      .filter((t) => t.status === 'pending')
      .map((t) => t.assignee_name || displayUserName(userMap, t.assignee_id))
    timelineItems.push({
      color: 'blue',
      dot: <ClockCircleOutlined style={{ fontSize: 14 }} />,
      children: (
        <div className="bpm-timeline-doing">
          <Space size={6} wrap>
            <Text strong style={{ color: '#1677ff' }}>
              等待审批
            </Text>
            <Tag color="processing">{doingName}</Tag>
            {pendingNames.length > 0 && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                待 {pendingNames.join('、')} 处理
              </Text>
            )}
          </Space>
        </div>
      ),
    })
  }

  return (
    <div className="bpm-instance-timeline">
      {showHeader && (
        <div style={{ marginBottom: 12 }}>
          <Space size={8} wrap>
            <Text strong>{instance.title}</Text>
            <Tag color={statusMeta.color} variant="filled">
              {statusMeta.label}
            </Tag>
          </Space>
          <div style={{ marginTop: 2 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {instance.initiator_name || displayUserName(userMap, instance.initiator_id)} 发起于{' '}
              {formatDateTime(instance.created_at)}
              {instance.finished_at ? ` · 完成于 ${formatDateTime(instance.finished_at)}` : ''}
            </Text>
          </div>
        </div>
      )}
      {showForm && instance.form_snapshot && Object.keys(instance.form_snapshot).length > 0 && (
        <Descriptions
          size="small"
          column={2}
          bordered
          style={{ marginBottom: 16 }}
          items={Object.entries(instance.form_snapshot).map(([key, value]) => ({
            key,
            label: FORM_FIELD_LABELS[key] ?? key,
            children: formatFormValue(key, value),
          }))}
        />
      )}
      {timelineItems.length > 0 ? (
        <Timeline items={timelineItems} />
      ) : (
        <Text type="secondary">暂无流转记录</Text>
      )}
    </div>
  )
}
