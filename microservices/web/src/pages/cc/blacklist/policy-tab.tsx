import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Checkbox, Form, InputNumber, Select, Space, TimePicker } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, SaveOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import {
  getCcRiskPolicy, saveCcRiskPolicy, ccYuanToFen,
} from '@/api/cc'

const ISO_WEEKDAYS = [
  { value: '1', label: '周一' },
  { value: '2', label: '周二' },
  { value: '3', label: '周三' },
  { value: '4', label: '周四' },
  { value: '5', label: '周五' },
  { value: '6', label: '周六' },
  { value: '7', label: '周日' },
]

type PolicyForm = {
  hours?: [Dayjs, Dayjs] | null
  weekdays?: string[]
  max_calls_per_number_per_hour?: number | null
  max_calls_per_agent_per_hour?: number | null
  max_calls_per_tenant_per_min?: number | null
  blocked_prefixes?: string[]
  max_rate_per_min_yuan?: number | null
}

export default function OutboundPolicyTab() {
  const { t } = useTranslation()
  const [form] = Form.useForm<PolicyForm>()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const p = await getCcRiskPolicy()
      const weekdays = (p.outbound_weekdays || '').split(',').map((s) => s.trim()).filter(Boolean)
      const prefixes = (p.blocked_prefixes || '').split(',').map((s) => s.trim()).filter(Boolean)
      const hours = p.outbound_start && p.outbound_end
        ? [dayjs(p.outbound_start, 'HH:mm'), dayjs(p.outbound_end, 'HH:mm')] as [Dayjs, Dayjs]
        : null
      form.setFieldsValue({
        hours,
        weekdays,
        max_calls_per_number_per_hour: p.max_calls_per_number_per_hour || null,
        max_calls_per_agent_per_hour: p.max_calls_per_agent_per_hour || null,
        max_calls_per_tenant_per_min: p.max_calls_per_tenant_per_min || null,
        blocked_prefixes: prefixes,
        max_rate_per_min_yuan: p.max_rate_per_min ? Number((p.max_rate_per_min / 100).toFixed(2)) : null,
      })
    } catch {
      message.error(t('获取外呼策略失败'))
    } finally {
      setLoading(false)
    }
  }, [form, t])

  useEffect(() => { void load() }, [load])

  const handleSave = async (values: PolicyForm) => {
    setSaving(true)
    try {
      const start = values.hours?.[0]?.format('HH:mm') ?? ''
      const end = values.hours?.[1]?.format('HH:mm') ?? ''
      await saveCcRiskPolicy({
        outbound_start: start,
        outbound_end: end,
        outbound_weekdays: (values.weekdays ?? []).join(','),
        max_calls_per_number_per_hour: values.max_calls_per_number_per_hour || 0,
        max_calls_per_agent_per_hour: values.max_calls_per_agent_per_hour || 0,
        max_calls_per_tenant_per_min: values.max_calls_per_tenant_per_min || 0,
        blocked_prefixes: (values.blocked_prefixes ?? []).join(','),
        max_rate_per_min: ccYuanToFen(values.max_rate_per_min_yuan),
      })
      message.success(t('外呼策略已保存，点击外呼即时生效'))
      void load()
    } catch (e) {
      message.error((e as Error)?.message || t('保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card bordered={false} loading={loading}>
      <div className="cell-dim" style={{ fontSize: 13, marginBottom: 16 }}>
        {t('空着或填 0 表示该条规则关闭。时区固定 Asia/Shanghai。白名单号码不受这些规则限制。')}
      </div>
      <Form form={form} layout="vertical" onFinish={handleSave} style={{ maxWidth: 640 }}>
        <Form.Item name="weekdays" label={t('允许外呼的星期')}>
          <Checkbox.Group options={ISO_WEEKDAYS.map((d) => ({ label: t(d.label), value: d.value }))} />
        </Form.Item>
        <Form.Item
          name="hours"
          label={t('允许外呼的时段')}
          tooltip={t('结束早于开始视为隔夜窗，如 22:00-06:00。留空则不限时段。')}
        >
          <TimePicker.RangePicker format="HH:mm" minuteStep={5} order={false} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="max_calls_per_number_per_hour"
          label={t('同一被叫每小时最多外呼（次）')}
        >
          <InputNumber min={0} max={100000} precision={0} style={{ width: '100%' }} placeholder={t('0 或不填 = 不限')} />
        </Form.Item>
        <Form.Item
          name="max_calls_per_agent_per_hour"
          label={t('本分机每小时最多外呼（次）')}
        >
          <InputNumber min={0} max={100000} precision={0} style={{ width: '100%' }} placeholder={t('0 或不填 = 不限')} />
        </Form.Item>
        <Form.Item
          name="max_calls_per_tenant_per_min"
          label={t('本租户每分钟最多外呼（次）')}
        >
          <InputNumber min={0} max={10000} precision={0} style={{ width: '100%' }} placeholder={t('0 或不填 = 不限')} />
        </Form.Item>
        <Form.Item
          name="blocked_prefixes"
          label={t('禁止外呼的被叫前缀')}
          tooltip={t('如 00 拦国际，166 拦虚拟号段。最长前缀优先。')}
        >
          <Select mode="tags" tokenSeparators={[',', ' ']} placeholder={t('输入前缀后回车，如 00')} />
        </Form.Item>
        <Form.Item
          name="max_rate_per_min_yuan"
          label={t('费率上限（元/分钟）')}
          tooltip={t('命中计费费率且高于此值则拒呼。未配置费率的方向不拦。')}
        >
          <InputNumber min={0} precision={2} style={{ width: '100%' }} placeholder={t('0 或不填 = 不限')} />
        </Form.Item>
        <Space>
          <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>{t('保存')}</Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
        </Space>
      </Form>
    </Card>
  )
}
