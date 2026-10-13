import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, Button, Form, Input, Modal, Select, Spin } from 'antd'
import { useTranslation } from 'react-i18next'
import { getAgentDefinition, updateAgentDefinition, type AgentDefinition } from '@/api/ai/agent'
import { message } from '@/utils/feedback'
import { definitionPayload, definitionTextLength } from './definition-form'

interface DefinitionEditorProps {
  id: number
  canWrite: boolean
  onClose: () => void
  onSaved: (definition: AgentDefinition) => void
}

/** 每次打开重新读取定义，不拿陈旧列表字段覆盖服务端；失败不展示空白可保存表单。 */
export default function DefinitionEditor({ id, canWrite, onClose, onSaved }: DefinitionEditorProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const [definition, setDefinition] = useState<AgentDefinition | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [saving, setSaving] = useState(false)
  const requestID = useRef(0)
  const savingRef = useRef(false)

  const load = useCallback(async () => {
    const ticket = ++requestID.current
    setLoading(true)
    setLoadError(false)
    setDefinition(null)
    form.resetFields()
    try {
      const result = await getAgentDefinition(id)
      if (ticket !== requestID.current) return
      if (result.definition.id !== id) throw new Error('agent-definition-id-mismatch')
      setDefinition(result.definition)
      form.setFieldsValue(definitionPayload({ ...result.definition }))
    } catch {
      // 请求层报告服务错误；这里独立保留可重试错误态，不把失败伪装为可编辑空数据。
      if (ticket === requestID.current) setLoadError(true)
    } finally {
      if (ticket === requestID.current) setLoading(false)
    }
  }, [form, id])

  useEffect(() => {
    void load()
    // 关闭或切换定义后，迟到响应不得污染下一次编辑。
    return () => { requestID.current += 1 }
  }, [load])

  const submit = async () => {
    if (!canWrite || !definition || loading || savingRef.current) return
    savingRef.current = true
    const ticket = requestID.current
    try {
      const values = await form.validateFields().catch(() => null)
      if (!values || ticket !== requestID.current) return
      setSaving(true)
      const next = await updateAgentDefinition(id, definitionPayload(values))
      if (ticket !== requestID.current) return
      message.success(t('更新成功'))
      onSaved(next)
      onClose()
    } catch {
      // PATCH 失败保留用户输入，只有成功响应才能关窗/刷新列表。
    } finally {
      savingRef.current = false
      if (ticket === requestID.current) setSaving(false)
    }
  }

  const textRule = (maximum: number, required = false) => ({
    validator: (_: unknown, value: string | undefined) => {
      const length = definitionTextLength(value ?? '')
      if (required && length === 0) return Promise.reject(new Error(t('请输入名称')))
      return length <= maximum ? Promise.resolve() : Promise.reject(new Error(t('最多 {{n}} 个字符', { n: maximum })))
    },
  })

  return (
    <Modal
      title={t('编辑 Agent')}
      open
      onCancel={() => { if (!savingRef.current) onClose() }}
      onOk={() => void submit()}
      confirmLoading={saving}
      okButtonProps={{ disabled: !canWrite || loading || !definition || loadError }}
      cancelButtonProps={{ disabled: saving }}
      closable={!saving}
      maskClosable={!saving}
      keyboard={!saving}
      width={560}
    >
      {loading && <Spin style={{ display: 'block', margin: '32px auto' }} />}
      {loadError && (
        <Alert type="error" showIcon message={t('Agent 定义加载失败')}
          action={<Button size="small" onClick={() => void load()}>{t('重试')}</Button>} />
      )}
      {/* Form 始终挂载以绑定 useForm；未加载成功时隐藏，避免显示半成品或旧定义。 */}
      <Form form={form} layout="vertical" disabled={!canWrite || loading || saving} style={{ display: definition && !loadError ? undefined : 'none' }}>
        <Form.Item label={t('Agent 标识')} extra={t('创建后不可修改，用于程序引用')}>
          <Input aria-label={t('Agent 标识')} value={definition?.agent_key ?? ''} disabled />
        </Form.Item>
        <Form.Item name="name" label={t('名称')} required rules={[textRule(100, true)]}>
          <Input />
        </Form.Item>
        <Form.Item name="description" label={t('描述')} rules={[textRule(500)]}>
          <Input.TextArea rows={3} />
        </Form.Item>
        <Form.Item name="status" label={t('状态')} rules={[{ required: true }]} extra={t('禁用仅阻止新运行，不会取消已有运行；重新启用后可继续运行。')}>
          <Select options={[{ value: 'active', label: t('启用') }, { value: 'disabled', label: t('禁用') }]} />
        </Form.Item>
        <Alert type="info" showIcon message={t('名称和描述只更新定义，已发布版本保持不变。')} />
      </Form>
    </Modal>
  )
}
