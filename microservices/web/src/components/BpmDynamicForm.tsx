import { DatePicker, Form, Input, InputNumber, Radio, Select, Switch } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import type { BpmFormField, BpmFormSchema } from '@/api/bpm'

// 声明式表单 Schema 的动态渲染器（表单构建器 M1）：通用发起页与重提弹窗
// 共用。渲染在父组件的 <Form> 内（本组件只产出 Form.Item 列表）。
// 约定：amount 表单值为「元」（提交时经 formValuesToSnapshot 转分），
// date 表单值为 dayjs（提交时转 YYYY-MM-DD 字符串）。

/** 快照（分/字符串日期）→ 表单初值（元/dayjs） */
export function snapshotToFormValues(
  schema: BpmFormSchema | null | undefined,
  snapshot?: Record<string, unknown> | null,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema?.fields ?? []) {
    const v = snapshot?.[f.key]
    if (v === undefined || v === null) continue
    if (f.type === 'amount' && typeof v === 'number') out[f.key] = v / 100
    else if (f.type === 'date' && typeof v === 'string' && v) out[f.key] = dayjs(v)
    else out[f.key] = v
  }
  return out
}

/** 表单值（元/dayjs）→ 快照（分/字符串日期）；空值剔除 */
export function formValuesToSnapshot(
  schema: BpmFormSchema | null | undefined,
  values: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema?.fields ?? []) {
    const v = values[f.key]
    if (v === undefined || v === null || v === '') continue
    if (f.type === 'amount' && typeof v === 'number') out[f.key] = Math.round(v * 100)
    else if (f.type === 'date') out[f.key] = (v as Dayjs).format('YYYY-MM-DD')
    else out[f.key] = v
  }
  return out
}

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
          addonAfter="元"
          placeholder={f.placeholder}
        />
      )
    case 'select':
      return (
        <Select
          allowClear={!f.required}
          placeholder={f.placeholder || '请选择'}
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
          rules={f.required ? [{ required: true, message: `请填写${f.label}` }] : undefined}
        >
          {fieldControl(f)}
        </Form.Item>
      ))}
    </>
  )
}
