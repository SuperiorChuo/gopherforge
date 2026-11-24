import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Segmented,
  Select,
  Switch,
} from 'antd'
import dayjs from 'dayjs'
import {
  ALERT_CHANNELS,
  createAlertRule,
  updateAlertRule,
  type AlertMetricDefinition,
  type AlertRulePayload,
  type MonitorAlertRule,
} from '@/api/monitor'
import { message } from '@/utils/feedback'
import {
  metricUnitLabel,
  OPERATOR_LABELS,
  SEVERITY_META,
  type AlertRuleFormValues,
} from './helpers'

interface RuleFormModalProps {
  open: boolean
  editingRule: MonitorAlertRule | null
  metrics: AlertMetricDefinition[]
  metricMap: Map<string, AlertMetricDefinition>
  onCancel: () => void
  onSaved: () => Promise<void> | void
}

export default function RuleFormModal({
  open,
  editingRule,
  metrics,
  metricMap,
  onCancel,
  onSaved,
}: RuleFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<AlertRuleFormValues>()
  const [submitting, setSubmitting] = useState(false)
  const defaultMetricRef = useRef<string | undefined>(metrics[0]?.key)
  const selectedMetricKey = Form.useWatch('metric', form)
  const selectedMetric = selectedMetricKey ? metricMap.get(selectedMetricKey) : undefined

  useEffect(() => {
    defaultMetricRef.current = metrics[0]?.key
  }, [metrics])

  useEffect(() => {
    if (!open) return
    form.resetFields()
    if (editingRule) {
      form.setFieldsValue({
        name: editingRule.name,
        metric: editingRule.metric,
        operator: editingRule.operator,
        threshold: editingRule.threshold,
        duration_seconds: editingRule.duration_seconds,
        severity: editingRule.severity,
        enabled: editingRule.enabled,
        notify_on_resolve: editingRule.notify_on_resolve,
        notify_channels: editingRule.notify_channels ?? [],
        silence_until: editingRule.silence_until ? dayjs(editingRule.silence_until) : null,
      })
      return
    }
    form.setFieldsValue({
      name: '',
      metric: defaultMetricRef.current,
      operator: 'gt',
      threshold: 80,
      duration_seconds: 60,
      severity: 'warning',
      enabled: true,
      notify_on_resolve: true,
      notify_channels: [],
      silence_until: null,
    })
  }, [editingRule, form, open])

  const submitRule = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    const payload: AlertRulePayload = {
      ...values,
      silence_until: values.silence_until ? values.silence_until.toISOString() : null,
    }
    setSubmitting(true)
    try {
      if (editingRule) {
        await updateAlertRule(editingRule.id, payload)
        message.success(t('告警规则已更新'))
      } else {
        await createAlertRule(payload)
        message.success(t('告警规则已创建'))
      }
      await onSaved()
    } catch {
      message.error(editingRule ? t('更新告警规则失败') : t('创建告警规则失败'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={editingRule ? t('编辑告警规则') : t('新增告警规则')}
      open={open}
      onOk={submitRule}
      onCancel={onCancel}
      confirmLoading={submitting}
      destroyOnHidden
      width={620}
    >
      <Form form={form} layout="vertical" className="alert-rule-form">
        <Form.Item name="name" label={t('规则名称')} rules={[{ required: true, message: t('请输入规则名称') }, { max: 100 }]}>
          <Input maxLength={100} />
        </Form.Item>
        <div className="alert-form-grid">
          <Form.Item name="metric" label={t('指标')} rules={[{ required: true, message: t('请选择指标') }]}>
            <Select
              showSearch
              optionFilterProp="label"
              options={metrics.map((metric) => ({ value: metric.key, label: metric.title }))}
              onChange={(value) => {
                const metric = metricMap.get(value)
                const operator = form.getFieldValue('operator')
                if (metric && !metric.operators.includes(operator)) form.setFieldValue('operator', metric.operators[0])
              }}
            />
          </Form.Item>
          <Form.Item name="operator" label={t('判断条件')} rules={[{ required: true, message: t('请选择判断条件') }]}>
            <Select
              options={(selectedMetric?.operators ?? ['gt', 'gte', 'lt', 'lte']).map((operator) => ({
                value: operator,
                label: t(OPERATOR_LABELS[operator]),
              }))}
            />
          </Form.Item>
          <Form.Item name="threshold" label={t('阈值')} rules={[{ required: true, message: t('请输入阈值') }]}>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              max={selectedMetric?.unit === 'percent' ? 100 : undefined}
              precision={selectedMetric?.unit === 'count' || selectedMetric?.unit === 'bytes' ? 0 : 2}
              addonAfter={metricUnitLabel(selectedMetric?.unit, t)}
            />
          </Form.Item>
          <Form.Item name="duration_seconds" label={t('持续时间')} rules={[{ required: true, message: t('请输入持续时间') }]}>
            <InputNumber style={{ width: '100%' }} min={0} max={604800} precision={0} addonAfter={t('秒')} />
          </Form.Item>
        </div>
        <Form.Item name="severity" label={t('告警级别')} rules={[{ required: true }]}>
          <Segmented
            block
            options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
          />
        </Form.Item>
        <div className="alert-switch-row">
          <Form.Item name="enabled" label={t('启用规则')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="notify_on_resolve" label={t('恢复时通知')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </div>
        <div className="alert-form-grid">
          <Form.Item name="notify_channels" label={t('通知渠道')} extra={t('不选则使用所有已配置渠道')}>
            <Select
              mode="multiple"
              allowClear
              placeholder={t('全部已配置渠道')}
              options={ALERT_CHANNELS}
            />
          </Form.Item>
          <Form.Item name="silence_until" label={t('静默至')} extra={t('维护窗口内不评估、不通知')}>
            <DatePicker showTime style={{ width: '100%' }} placeholder={t('不静默')} />
          </Form.Item>
        </div>
      </Form>
    </Modal>
  )
}
