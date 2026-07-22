import { useMemo, useState } from 'react'
import { Button, Form, Input, Modal, Radio, Select, Space } from 'antd'
import {
  CheckOutlined,
  CloseOutlined,
  EditOutlined,
  SwapOutlined,
  UndoOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  approveTask,
  rejectTask,
  returnTask,
  transferTask,
  type BpmTask,
} from '@/api/bpm'
import BpmResubmitModal from '@/components/BpmResubmitModal'
import { useUserNameMap } from '@/hooks/useUserNameMap'

// 任务详情 actions 缺省（后端未返回动作列表）时的基线：M1/M2 常规审批动作。
// 服务端仍是权威校验方，越权动作会被拒绝并由拦截器提示。
const FALLBACK_ACTIONS = ['approve', 'reject', 'transfer', 'return_start']

type ModalMode = 'approve' | 'reject' | 'transfer' | 'return'

/**
 * 审批任务动作条（待办行 / 任务详情动作区 / 实例详情复用同一份，含弹窗）。
 * 按任务详情返回的动作列表动态渲染：approve/reject/transfer/return_start/return_prev/resubmit。
 * - 转办：选人（复用现有用户映射数据源）+ 意见选填
 * - 退回：退回到发起人/上一节点（return_prev 仅当动作列表含它时显示）+ 意见必填
 * - 重新提交：复用 BpmResubmitModal（含撤销流程入口）
 */
interface BpmTaskActionsProps {
  task: BpmTask
  /** 任务详情返回的动作列表；undefined = 未知（按基线渲染，交给后端兜底校验） */
  actions?: string[]
  /** 行内用 link（默认），详情动作区用 default 实体按钮 */
  buttonType?: 'link' | 'default'
  onDone: () => void
}

export default function BpmTaskActions({
  task,
  actions,
  buttonType = 'link',
  onDone,
}: BpmTaskActionsProps) {
  const [modal, setModal] = useState<ModalMode | null>(null)
  const [resubmitOpen, setResubmitOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const userMap = useUserNameMap()

  const acts = actions ?? FALLBACK_ACTIONS
  const canReturnStart = acts.includes('return_start')
  const canReturnPrev = acts.includes('return_prev')

  const userOptions = useMemo(
    () =>
      Object.entries(userMap)
        .map(([id, name]) => ({ value: Number(id), label: name }))
        .filter((o) => o.value !== task.assignee_id),
    [userMap, task.assignee_id],
  )

  const openModal = (mode: ModalMode) => {
    form.resetFields()
    if (mode === 'return') {
      form.setFieldsValue({ to: canReturnStart ? 'start' : 'prev' })
    }
    setModal(mode)
  }

  const onSubmit = async () => {
    if (!modal) return
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (modal === 'approve') {
        const res = await approveTask(task.id, values.comment)
        message.success(res?.instance_status === 'approved' ? '已同意，流程审批通过' : '已同意')
      } else if (modal === 'reject') {
        const res = await rejectTask(task.id, values.comment)
        message.success(res?.instance_status === 'rejected' ? '已拒绝，流程结束' : '已拒绝')
      } else if (modal === 'transfer') {
        await transferTask(task.id, values.target_user_id, values.comment)
        message.success('已转办，新处理人将收到待办通知')
      } else {
        await returnTask(task.id, values.to, values.comment)
        message.success(values.to === 'start' ? '已退回发起人' : '已退回上一节点')
      }
      setModal(null)
      onDone()
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const size = buttonType === 'link' ? 'small' : 'middle'
  const modalTitles: Record<ModalMode, string> = {
    approve: '同意',
    reject: '拒绝',
    transfer: '转办',
    return: '退回',
  }

  return (
    <>
      <Space size={0} wrap className={buttonType === 'link' ? 'table-actions' : undefined}>
        {acts.includes('approve') && (
          <Button type={buttonType} size={size} icon={<CheckOutlined />} onClick={() => openModal('approve')}>
            同意
          </Button>
        )}
        {acts.includes('reject') && (
          <Button type={buttonType} size={size} danger icon={<CloseOutlined />} onClick={() => openModal('reject')}>
            拒绝
          </Button>
        )}
        {acts.includes('transfer') && (
          <Button type={buttonType} size={size} icon={<SwapOutlined />} onClick={() => openModal('transfer')}>
            转办
          </Button>
        )}
        {(canReturnStart || canReturnPrev) && (
          <Button type={buttonType} size={size} icon={<UndoOutlined />} onClick={() => openModal('return')}>
            退回
          </Button>
        )}
        {acts.includes('resubmit') && (
          <Button
            type={buttonType === 'link' ? 'link' : 'primary'}
            size={size}
            icon={<EditOutlined />}
            onClick={() => setResubmitOpen(true)}
          >
            重新提交
          </Button>
        )}
      </Space>

      <Modal
        title={modal ? `${modalTitles[modal]}：${task.instance_title || `实例 #${task.instance_id}`}` : ''}
        open={!!modal}
        onOk={() => void onSubmit()}
        onCancel={() => setModal(null)}
        confirmLoading={submitting}
        okText={modal ? `确认${modalTitles[modal]}` : '确认'}
        okButtonProps={modal === 'reject' ? { danger: true } : undefined}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          {modal === 'transfer' && (
            <Form.Item
              name="target_user_id"
              label="转办给"
              rules={[{ required: true, message: '请选择转办目标用户' }]}
            >
              <Select
                showSearch
                optionFilterProp="label"
                placeholder="选择用户（任务将转由其处理，计数规则不变）"
                options={userOptions}
              />
            </Form.Item>
          )}
          {modal === 'return' && (
            <Form.Item name="to" label="退回到" rules={[{ required: true, message: '请选择退回目标' }]}>
              <Radio.Group
                options={[
                  ...(canReturnStart
                    ? [{ label: '发起人（修改后可重新提交）', value: 'start' }]
                    : []),
                  ...(canReturnPrev ? [{ label: '上一节点（重新审批）', value: 'prev' }] : []),
                ]}
              />
            </Form.Item>
          )}
          <Form.Item
            name="comment"
            label={modal === 'transfer' ? '转办说明' : '审批意见'}
            rules={
              modal === 'reject' || modal === 'return'
                ? [
                    {
                      required: true,
                      message: modal === 'return' ? '退回时必须填写意见' : '拒绝时必须填写审批意见',
                    },
                  ]
                : []
            }
          >
            <Input.TextArea
              rows={3}
              maxLength={512}
              placeholder={
                modal === 'reject'
                  ? '请说明拒绝原因（必填）'
                  : modal === 'return'
                    ? '请说明退回原因（必填）'
                    : '可选'
              }
            />
          </Form.Item>
        </Form>
      </Modal>

      <BpmResubmitModal
        instanceId={task.instance_id}
        open={resubmitOpen}
        onClose={() => setResubmitOpen(false)}
        onDone={onDone}
      />
    </>
  )
}
