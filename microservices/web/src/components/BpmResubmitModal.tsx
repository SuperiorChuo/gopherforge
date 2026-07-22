import { useEffect, useState } from 'react'
import { Alert, Button, Input, InputNumber, Modal, Popconfirm, Skeleton, Space, Tag, Typography } from 'antd'
import { RollbackOutlined, SendOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  BPM_FORM_FIELD_LABELS,
  cancelInstance,
  getInstance,
  resubmitInstance,
  type BpmInstance,
} from '@/api/bpm'

const { Text } = Typography

/**
 * 被退回实例的「重新提交」弹窗（M2，引擎侧通用能力）。
 * 展示当前 form_snapshot 的键值编辑器：字符串/数字值可改，其余类型只读展示；
 * 不做业务表单渲染。确认后调 resubmit（全链路 round+1 重新流转）；
 * 旁给「撤销流程」按钮（复用既有 cancel）。
 * 复用方：我发起的列表/详情抽屉、待办中心的重提任务动作。
 */
interface BpmResubmitModalProps {
  /** 为空时不加载（配合 open 控制） */
  instanceId?: number | null
  open: boolean
  onClose: () => void
  /** 重新提交或撤销成功后回调（刷新列表等） */
  onDone: () => void
}

type SnapshotEntry = { key: string; value: unknown; editable: 'string' | 'number' | false }

function toEntries(snapshot?: Record<string, unknown>): SnapshotEntry[] {
  return Object.entries(snapshot ?? {}).map(([key, value]) => ({
    key,
    value,
    editable: typeof value === 'string' ? 'string' : typeof value === 'number' ? 'number' : false,
  }))
}

export default function BpmResubmitModal({ instanceId, open, onClose, onDone }: BpmResubmitModalProps) {
  const [loading, setLoading] = useState(false)
  const [instance, setInstance] = useState<BpmInstance | null>(null)
  const [entries, setEntries] = useState<SnapshotEntry[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [canceling, setCanceling] = useState(false)

  useEffect(() => {
    if (!open || !instanceId) return
    let alive = true
    setLoading(true)
    setInstance(null)
    setEntries([])
    getInstance(instanceId)
      .then((inst) => {
        if (!alive) return
        setInstance(inst)
        setEntries(toEntries(inst.form_snapshot))
      })
      .catch(() => {
        // 错误提示由 request 拦截器统一弹出
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [open, instanceId])

  const setValue = (key: string, value: unknown) => {
    setEntries((prev) => prev.map((e) => (e.key === key ? { ...e, value } : e)))
  }

  const onResubmit = async () => {
    if (!instanceId) return
    setSubmitting(true)
    try {
      const snapshot: Record<string, unknown> = {}
      entries.forEach((e) => {
        snapshot[e.key] = e.value
      })
      await resubmitInstance(instanceId, snapshot)
      message.success('已重新提交，流程将重新流转')
      onClose()
      onDone()
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const onCancelFlow = async () => {
    if (!instanceId) return
    setCanceling(true)
    try {
      await cancelInstance(instanceId)
      message.success('流程已撤销')
      onClose()
      onDone()
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setCanceling(false)
    }
  }

  return (
    <Modal
      title={instance ? `重新提交：${instance.title}` : '重新提交'}
      open={open}
      onCancel={onClose}
      destroyOnHidden
      width={520}
      footer={
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Popconfirm
            title="撤销该流程？"
            description="撤销后流程终止，业务对象状态由回调回写"
            onConfirm={() => void onCancelFlow()}
          >
            <Button danger icon={<RollbackOutlined />} loading={canceling}>
              撤销流程
            </Button>
          </Popconfirm>
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button
              type="primary"
              icon={<SendOutlined />}
              loading={submitting}
              disabled={loading || !instance}
              onClick={() => void onResubmit()}
            >
              重新提交
            </Button>
          </Space>
        </div>
      }
    >
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        message="流程被退回到发起人，可修改表单快照后重新提交，审批链将从头重新流转"
      />
      {loading ? (
        <Skeleton active paragraph={{ rows: 4 }} title={false} />
      ) : entries.length ? (
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          {entries.map((e) => (
            <div key={e.key}>
              <Space size={6}>
                <Text type="secondary">{BPM_FORM_FIELD_LABELS[e.key] ?? e.key}</Text>
                <Tag className="cell-mono" style={{ fontSize: 11 }}>
                  {e.key}
                </Tag>
              </Space>
              <div style={{ marginTop: 4 }}>
                {e.editable === 'string' ? (
                  <Input
                    value={e.value as string}
                    maxLength={512}
                    onChange={(ev) => setValue(e.key, ev.target.value)}
                  />
                ) : e.editable === 'number' ? (
                  <InputNumber
                    style={{ width: '100%' }}
                    value={e.value as number}
                    onChange={(v) => setValue(e.key, v ?? 0)}
                  />
                ) : (
                  <Text type="secondary" className="cell-mono">
                    {JSON.stringify(e.value)}（只读）
                  </Text>
                )}
              </div>
            </div>
          ))}
        </Space>
      ) : (
        <Text type="secondary">该实例无表单快照，可直接重新提交</Text>
      )}
    </Modal>
  )
}
