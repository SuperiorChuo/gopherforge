import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Form, Input, Modal, Select } from 'antd'
import { message } from '@/utils/feedback'
import { createGeoPrompt, updateGeoPrompt, type VisGeoPrompt } from '@/api/visibility'

type PromptFormValues = {
  category: string
  question: string
}

export interface PromptFormModalProps {
  open: boolean
  editingPrompt: VisGeoPrompt | null
  onClose: () => void
  onSaved: () => Promise<void> | void
}

export default function PromptFormModal({
  open,
  editingPrompt,
  onClose,
  onSaved,
}: PromptFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<PromptFormValues>()

  useEffect(() => {
    if (!open) return
    form.resetFields()
    if (editingPrompt) {
      form.setFieldsValue({ category: editingPrompt.category, question: editingPrompt.question })
    } else {
      form.setFieldsValue({ category: 'category' })
    }
  }, [open, editingPrompt, form])

  async function onSavePrompt() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editingPrompt) {
        await updateGeoPrompt(editingPrompt.id, values)
        message.success(t('问题已更新'))
      } else {
        await createGeoPrompt(values)
        message.success(t('问题已添加'))
      }
      form.resetFields()
      onClose()
      await onSaved()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  return (
    <Modal
      title={editingPrompt ? t('编辑检测问题') : t('添加检测问题')}
      open={open}
      onCancel={onClose}
      onOk={() => void onSavePrompt()}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" initialValues={{ category: 'category' }}>
        <Form.Item
          name="category"
          label={t('问题类型')}
          help={t('品类词：问某类产品（如“有什么好用的后台系统”）；对比题：和竞品比较；场景题：描述使用场景求推荐')}
        >
          <Select
            options={[
              { label: t('品类词（问某类产品）'), value: 'category' },
              { label: t('对比题（和竞品比较）'), value: 'comparison' },
              { label: t('场景题（描述场景求推荐）'), value: 'scenario' },
              { label: t('其他'), value: 'other' },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="question"
          label={t('问题内容')}
          rules={[{ required: true, message: t('请输入问题') }]}
          help={t('写用户真实会问 AI 的话。检测时会拿这个问题问 AI，再分析回答里有没有提到你的品牌')}
        >
          <Input.TextArea rows={3} placeholder={t('例：有没有适合中小团队的开源后台管理系统推荐？')} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
