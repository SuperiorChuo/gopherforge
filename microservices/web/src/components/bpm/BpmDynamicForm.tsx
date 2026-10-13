import { DatePicker, Form, Input, InputNumber, Radio, Select, Switch } from 'antd'
import i18n from '@/i18n/init'
import type { BpmFormField, BpmFormSchema } from '@/api/bpm'

// 声明式表单 Schema 的动态渲染器（表单构建器 M1）：通用发起页与重提弹窗
// 共用。渲染在父组件的 <Form> 内（本组件只产出 Form.Item 列表）。
// 约定：amount 表单值为「元」（提交时经 formValuesToSnapshot 转分），
// date 表单值为 dayjs（提交时转 YYYY-MM-DD 字符串）。

function fieldControl(f: BpmFormField) {
  switch (f.type) {
    case 'textarea':
      return <Input.TextArea rows={f.rows || 3} placeholder={f.placeholder} maxLength={2000} />
    case 'number':
      return <InputNumber style={{ width: '100%' }} min={f.min} max={f.max} placeholder={f.placeholder} />
    case 'amount':
      return (
        <InputNumber
          style={{ width: '100%' }}
          min={f.min !== undefined ? f.min / 100 : 0}
          max={f.max !== undefined ? f.max / 100 : undefined}
          precision={2}
          addonAfter={i18n.t('元')}
          placeholder={f.placeholder}
        />
      )
    case 'select':
      return (
        <Select
          allowClear={!f.required}
          placeholder={f.placeholder || i18n.t('请选择')}
          options={(f.options ?? []).map((o) => ({ value: o, label: o }))}
        />
      )
    case 'radio':
      return <Radio.Group options={(f.options ?? []).map((o) => ({ value: o, label: o }))} />
    case 'date':
      return <DatePicker style={{ width: '100%' }} placeholder={f.placeholder} />
    case 'switch':
      return <Switch />
    default:
      return <Input placeholder={f.placeholder} maxLength={2000} />
  }
}

export default function BpmDynamicForm({ schema }: { schema?: BpmFormSchema | null }) {
  return (
    <>
      {(schema?.fields ?? []).map((f) => (
        <Form.Item
          key={f.key}
          name={f.key}
          label={f.label}
          valuePropName={f.type === 'switch' ? 'checked' : 'value'}
          rules={f.required ? [{ required: true, message: i18n.t(`请填写${f.label}`) }] : undefined}
        >
          {fieldControl(f)}
        </Form.Item>
      ))}
    </>
  )
}
