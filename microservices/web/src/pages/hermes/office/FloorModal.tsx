// 办公室楼层新建 / 编辑弹窗。
import { useTranslation } from 'react-i18next'
import { Form, Input, InputNumber, Modal, type FormInstance } from 'antd'
import type { FloorFormValues } from './constants'

export interface FloorModalProps {
  open: boolean
  isEdit: boolean
  saving: boolean
  form: FormInstance<FloorFormValues>
  onOk: () => void
  onCancel: () => void
}

export function FloorModal({ open, isEdit, saving, form, onOk, onCancel }: FloorModalProps) {
  const { t } = useTranslation()

  return (
    <Modal
      title={isEdit ? t('编辑楼层') : t('新建楼层')}
      open={open}
      onOk={onOk}
      onCancel={onCancel}
      confirmLoading={saving}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item name="name" label={t('楼层名称')} rules={[{ required: true, max: 80 }]}>
          <Input placeholder={t('如：3 楼研发区')} />
        </Form.Item>
        <Form.Item name="width" label={t('平面宽度（米）')} rules={[{ required: true }]}>
          <InputNumber min={1} max={500} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="depth" label={t('平面深度（米）')} rules={[{ required: true }]}>
          <InputNumber min={1} max={500} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="sort" label={t('排序')}>
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
