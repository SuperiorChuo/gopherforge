import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Segmented,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
} from 'antd'
import dayjs from 'dayjs'
import type { ColumnsType } from 'antd/es/table'
import {
  BellOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import {
  ALERT_CHANNELS,
  createAlertRule,
  deleteAlertRule,
  evaluateAlertRule,
  getAlertEvents,
  getAlertMetrics,
  getAlertRules,
  getAlertSummary,
  updateAlertRule,
  type AlertEventListParams,
  type AlertMetricDefinition,
  type AlertRuleListParams,
  type AlertRulePayload,
  type AlertRuleState,
  type AlertSeverity,
  type MonitorAlertEvent,
  type MonitorAlertRule,
  type MonitorAlertSummary,
} from '@/api/monitor'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill, { type StatusTone } from '@/components/StatusPill'
import TableToolbar from '@/components/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import './style.css'

type ViewMode = 'rules' | 'events'

// 表单里 silence_until 由 DatePicker 持有为 dayjs，提交时再转 ISO 字符串
type AlertRuleFormValues = Omit<AlertRulePayload, 'silence_until'> & { silence_until: dayjs.Dayjs | null }

const OPERATOR_LABELS: Record<string, string> = {
  gt: '大于',
  gte: '大于等于',
  lt: '小于',
  lte: '小于等于',
}

const STATE_META: Record<AlertRuleState, { label: string; tone: StatusTone }> = {
  ok: { label: '正常', tone: 'success' },
  pending: { label: '等待确认', tone: 'warning' },
  firing: { label: '告警中', tone: 'danger' },
  error: { label: '采集异常', tone: 'danger' },
}

const SEVERITY_META: Record<AlertSeverity, { label: string; color: string }> = {
  info: { label: '提示', color: 'blue' },
  warning: { label: '警告', color: 'gold' },
  critical: { label: '严重', color: 'red' },
}

const NOTIFY_META: Record<string, { label: string; color?: string }> = {
  pending: { label: '待发送' },
  sent: { label: '已发送', color: 'green' },
  skipped: { label: '已跳过' },
  failed: { label: '发送失败', color: 'red' },
}

const defaultRuleParams: AlertRuleListParams = { page: 1, page_size: 10 }
const defaultEventParams: AlertEventListParams = { page: 1, page_size: 10 }

export default function AlertRulesPage() {
  const [view, setView] = useState<ViewMode>('rules')
  const [metrics, setMetrics] = useState<AlertMetricDefinition[]>([])
  const [rules, setRules] = useState<MonitorAlertRule[]>([])
  const [summary, setSummary] = useState<MonitorAlertSummary>({
    total: 0,
    enabled: 0,
    firing: 0,
    pending: 0,
    error: 0,
    checked_at: '',
  })
  const [ruleTotal, setRuleTotal] = useState(0)
  const [ruleParams, setRuleParams] = useState<AlertRuleListParams>(defaultRuleParams)
  const [ruleLoading, setRuleLoading] = useState(false)
  const [events, setEvents] = useState<MonitorAlertEvent[]>([])
  const [eventTotal, setEventTotal] = useState(0)
  const [eventParams, setEventParams] = useState<AlertEventListParams>(defaultEventParams)
  const [eventLoading, setEventLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<MonitorAlertRule | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [evaluatingID, setEvaluatingID] = useState<number | null>(null)
  const [form] = Form.useForm<AlertRuleFormValues>()
  const [ruleFilterForm] = Form.useForm()
  const [eventFilterForm] = Form.useForm()
  const selectedMetricKey = Form.useWatch('metric', form)
  const { hasPerm } = usePermission()

  const metricMap = useMemo(
    () => new Map(metrics.map((metric) => [metric.key, metric])),
    [metrics],
  )
  const selectedMetric = selectedMetricKey ? metricMap.get(selectedMetricKey) : undefined

  const fetchRules = useCallback(async (params: AlertRuleListParams, quiet = false) => {
    if (!quiet) setRuleLoading(true)
    try {
      const result = await getAlertRules(params)
      setRules(result.list ?? [])
      setRuleTotal(result.total ?? 0)
      getAlertSummary().then(setSummary).catch(() => undefined)
    } catch {
      if (!quiet) message.error('获取告警规则失败')
    } finally {
      if (!quiet) setRuleLoading(false)
    }
  }, [])

  const fetchEvents = useCallback(async (params: AlertEventListParams, quiet = false) => {
    if (!quiet) setEventLoading(true)
    try {
      const result = await getAlertEvents(params)
      setEvents(result.list ?? [])
      setEventTotal(result.total ?? 0)
    } catch {
      if (!quiet) message.error('获取告警事件失败')
    } finally {
      if (!quiet) setEventLoading(false)
    }
  }, [])

  useEffect(() => {
    getAlertMetrics()
      .then((result) => setMetrics(result.list ?? []))
      .catch(() => message.error('获取指标目录失败'))
  }, [])

  useEffect(() => {
    void fetchRules(ruleParams)
  }, [fetchRules, ruleParams])

  useEffect(() => {
    void fetchEvents(eventParams)
  }, [eventParams, fetchEvents])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchRules(ruleParams, true)
      if (view === 'events') void fetchEvents(eventParams, true)
    }, 30_000)
    return () => window.clearInterval(timer)
  }, [eventParams, fetchEvents, fetchRules, ruleParams, view])

  const openCreate = () => {
    setEditingRule(null)
    form.setFieldsValue({
      name: '',
      metric: metrics[0]?.key,
      operator: 'gt',
      threshold: 80,
      duration_seconds: 60,
      severity: 'warning',
      enabled: true,
      notify_on_resolve: true,
      notify_channels: [],
      silence_until: null,
    })
    setModalOpen(true)
  }

  const openEdit = (rule: MonitorAlertRule) => {
    setEditingRule(rule)
    form.setFieldsValue({
      name: rule.name,
      metric: rule.metric,
      operator: rule.operator,
      threshold: rule.threshold,
      duration_seconds: rule.duration_seconds,
      severity: rule.severity,
      enabled: rule.enabled,
      notify_on_resolve: rule.notify_on_resolve,
      notify_channels: rule.notify_channels ?? [],
      silence_until: rule.silence_until ? dayjs(rule.silence_until) : null,
    })
    setModalOpen(true)
  }

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
        message.success('告警规则已更新')
      } else {
        await createAlertRule(payload)
        message.success('告警规则已创建')
      }
      setModalOpen(false)
      await fetchRules(ruleParams)
    } catch {
      message.error(editingRule ? '更新告警规则失败' : '创建告警规则失败')
    } finally {
      setSubmitting(false)
    }
  }

  const removeRule = async (rule: MonitorAlertRule) => {
    try {
      await deleteAlertRule(rule.id)
      message.success('告警规则已删除')
      const nextPage = rules.length === 1 && ruleParams.page > 1 ? ruleParams.page - 1 : ruleParams.page
      setRuleParams({ ...ruleParams, page: nextPage })
      void fetchEvents(eventParams, true)
    } catch {
      message.error('删除告警规则失败')
    }
  }

  const evaluateRule = async (rule: MonitorAlertRule) => {
    setEvaluatingID(rule.id)
    try {
      const result = await evaluateAlertRule(rule.id)
      if (result.event?.status === 'firing') {
        message.warning('评估完成，规则进入告警状态')
      } else if (result.event?.status === 'resolved') {
        message.success('评估完成，告警已恢复')
      } else {
        message.success('评估完成')
      }
      await Promise.all([fetchRules(ruleParams, true), fetchEvents(eventParams, true)])
    } catch {
      message.error('评估失败，规则状态已记录采集错误')
      void fetchRules(ruleParams, true)
    } finally {
      setEvaluatingID(null)
    }
  }

  const ruleColumns: ColumnsType<MonitorAlertRule> = [
    {
      title: '规则',
      dataIndex: 'name',
      width: 180,
      render: (name: string, rule) => (
        <div className="alert-rule-name">
          <span>{name}</span>
          {!rule.enabled && <Tag>停用</Tag>}
        </div>
      ),
    },
    {
      title: '指标',
      dataIndex: 'metric',
      width: 220,
      render: (metric: string) => (
        <Tooltip title={metricMap.get(metric)?.description ?? metric}>
          <span className="cell-mono">{metricMap.get(metric)?.title ?? metric}</span>
        </Tooltip>
      ),
    },
    {
      title: '条件',
      width: 150,
      render: (_, rule) => (
        <span className="alert-condition">
          {OPERATOR_LABELS[rule.operator]} {formatMetricValue(rule.threshold, metricMap.get(rule.metric)?.unit)}
        </span>
      ),
    },
    {
      title: '持续',
      dataIndex: 'duration_seconds',
      width: 90,
      render: (seconds: number) => seconds === 0 ? '立即' : `${seconds} 秒`,
    },
    {
      title: '级别',
      dataIndex: 'severity',
      width: 90,
      render: (severity: AlertSeverity) => (
        <Tag color={SEVERITY_META[severity].color}>{SEVERITY_META[severity].label}</Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'state',
      width: 120,
      render: (state: AlertRuleState, rule) => (
        <Tooltip title={state === 'error' ? rule.last_error : undefined}>
          <span>
            <StatusPill
              tone={STATE_META[state].tone}
              label={state === 'error' && rule.firing_since ? '告警中 / 采集异常' : STATE_META[state].label}
              pulse={state === 'firing' || (state === 'error' && Boolean(rule.firing_since))}
            />
          </span>
        </Tooltip>
      ),
    },
    {
      title: '最近值',
      dataIndex: 'last_value',
      width: 120,
      render: (value: number | null | undefined, rule) =>
        value == null ? '-' : formatMetricValue(value, metricMap.get(rule.metric)?.unit),
    },
    {
      title: '最近评估',
      dataIndex: 'last_evaluated_at',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: '操作',
      width: 250,
      fixed: 'right',
      render: (_, rule) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:alert:evaluate') && (
            <Tooltip title={rule.enabled ? '立即使用真实指标评估' : '停用规则不能评估'}>
              <Button
                type="link"
                size="small"
                icon={<ThunderboltOutlined />}
                disabled={!rule.enabled}
                loading={evaluatingID === rule.id}
                onClick={() => evaluateRule(rule)}
              >
                评估
              </Button>
            </Tooltip>
          )}
          {hasPerm('system:alert:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(rule)}>
              编辑
            </Button>
          )}
          {hasPerm('system:alert:delete') && (
            <Popconfirm title="删除规则后历史事件仍会保留，确认删除?" onConfirm={() => removeRule(rule)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  const eventColumns: ColumnsType<MonitorAlertEvent> = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    { title: '规则', dataIndex: 'rule_name', width: 180 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status: MonitorAlertEvent['status']) => status === 'firing'
        ? <StatusPill tone="danger" label="触发" pulse />
        : <StatusPill tone="success" label="恢复" pulse={false} />,
    },
    {
      title: '级别',
      dataIndex: 'severity',
      width: 90,
      render: (severity: AlertSeverity) => (
        <Tag color={SEVERITY_META[severity].color}>{SEVERITY_META[severity].label}</Tag>
      ),
    },
    {
      title: '指标值',
      width: 230,
      render: (_, event) => (
        <span className="cell-mono">
          {metricMap.get(event.metric)?.title ?? event.metric}: {formatMetricValue(event.value, metricMap.get(event.metric)?.unit)}
        </span>
      ),
    },
    {
      title: '通知',
      dataIndex: 'notify_status',
      width: 110,
      render: (status: string, event) => (
        <Tooltip title={event.notify_error || undefined}>
          <Tag color={NOTIFY_META[status]?.color}>{NOTIFY_META[status]?.label ?? status}</Tag>
        </Tooltip>
      ),
    },
    { title: '事件内容', dataIndex: 'message', ellipsis: true },
  ]

  return (
    <div className="page-list alert-rules-page">
      <div className="alert-overview-strip" aria-label="告警状态摘要">
        <div><span>启用规则</span><strong>{summary.enabled}</strong></div>
        <div data-tone="danger"><span>告警中</span><strong>{summary.firing}</strong></div>
        <div data-tone="warning"><span>等待确认</span><strong>{summary.pending}</strong></div>
        <div data-tone="error"><span>采集异常</span><strong>{summary.error}</strong></div>
      </div>

      <div className="alert-view-switch">
        <Segmented
          value={view}
          onChange={(value) => setView(value as ViewMode)}
          options={[
            { value: 'rules', label: '规则', icon: <BellOutlined /> },
            { value: 'events', label: '事件', icon: <HistoryOutlined /> },
          ]}
        />
      </div>

      {view === 'rules' ? (
        <>
          <Card className="list-filter-card" bordered={false}>
            <Form
              form={ruleFilterForm}
              layout="inline"
              className="list-filter-form"
              onFinish={(values) => setRuleParams({ ...defaultRuleParams, ...values })}
            >
              <Form.Item name="name">
                <Input placeholder="规则名称" prefix={<SearchOutlined />} allowClear />
              </Form.Item>
              <Form.Item name="metric">
                <Select
                  placeholder="指标"
                  allowClear
                  showSearch
                  optionFilterProp="label"
                  options={metrics.map((metric) => ({ value: metric.key, label: metric.title }))}
                />
              </Form.Item>
              <Form.Item name="state">
                <Select
                  placeholder="状态"
                  allowClear
                  options={Object.entries(STATE_META).map(([value, meta]) => ({ value, label: meta.label }))}
                />
              </Form.Item>
              <Form.Item name="enabled">
                <Select
                  placeholder="启用状态"
                  allowClear
                  options={[{ value: true, label: '启用' }, { value: false, label: '停用' }]}
                />
              </Form.Item>
              <Form.Item className="list-filter-actions">
                <Space>
                  <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
                  <Button
                    icon={<ReloadOutlined />}
                    onClick={() => {
                      ruleFilterForm.resetFields()
                      setRuleParams(defaultRuleParams)
                    }}
                  >
                    重置
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </Card>

          <Card className="list-main-card" bordered={false}>
            <TableToolbar
              title="告警规则"
              total={ruleTotal}
              extra={
                <Space>
                  <Button icon={<ReloadOutlined />} onClick={() => fetchRules(ruleParams)}>刷新</Button>
                  {hasPerm('system:alert:create') && (
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增规则</Button>
                  )}
                </Space>
              }
            />
            <Table
              rowKey="id"
              className="list-table"
              columns={ruleColumns}
              dataSource={rules}
              loading={ruleLoading}
              scroll={{ x: 1450 }}
              locale={{ emptyText: <GlassEmpty text="暂无告警规则" compact /> }}
              pagination={{
                total: ruleTotal,
                current: ruleParams.page,
                pageSize: ruleParams.page_size,
                showSizeChanger: true,
                showTotal: (total) => `共 ${total} 条`,
                onChange: (page, page_size) => setRuleParams({ ...ruleParams, page, page_size }),
              }}
            />
          </Card>
        </>
      ) : (
        <>
          <Card className="list-filter-card" bordered={false}>
            <Form
              form={eventFilterForm}
              layout="inline"
              className="list-filter-form"
              onFinish={(values) => setEventParams({ ...defaultEventParams, ...values })}
            >
              <Form.Item name="rule_name">
                <Input placeholder="规则名称" prefix={<SearchOutlined />} allowClear />
              </Form.Item>
              <Form.Item name="status">
                <Select placeholder="事件状态" allowClear options={[{ value: 'firing', label: '触发' }, { value: 'resolved', label: '恢复' }]} />
              </Form.Item>
              <Form.Item name="severity">
                <Select
                  placeholder="级别"
                  allowClear
                  options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: meta.label }))}
                />
              </Form.Item>
              <Form.Item name="notify_status">
                <Select
                  placeholder="通知结果"
                  allowClear
                  options={Object.entries(NOTIFY_META).map(([value, meta]) => ({ value, label: meta.label }))}
                />
              </Form.Item>
              <Form.Item className="list-filter-actions">
                <Space>
                  <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
                  <Button
                    icon={<ReloadOutlined />}
                    onClick={() => {
                      eventFilterForm.resetFields()
                      setEventParams(defaultEventParams)
                    }}
                  >
                    重置
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </Card>

          <Card className="list-main-card" bordered={false}>
            <TableToolbar
              title="告警事件"
              total={eventTotal}
              extra={<Button icon={<ReloadOutlined />} onClick={() => fetchEvents(eventParams)}>刷新</Button>}
            />
            <Table
              rowKey="id"
              className="list-table"
              columns={eventColumns}
              dataSource={events}
              loading={eventLoading}
              scroll={{ x: 1100 }}
              locale={{ emptyText: <GlassEmpty text="暂无告警事件" compact /> }}
              pagination={{
                total: eventTotal,
                current: eventParams.page,
                pageSize: eventParams.page_size,
                showSizeChanger: true,
                showTotal: (total) => `共 ${total} 条`,
                onChange: (page, page_size) => setEventParams({ ...eventParams, page, page_size }),
              }}
            />
          </Card>
        </>
      )}

      <Modal
        title={editingRule ? '编辑告警规则' : '新增告警规则'}
        open={modalOpen}
        onOk={submitRule}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={620}
      >
        <Form form={form} layout="vertical" className="alert-rule-form">
          <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名称' }, { max: 100 }]}>
            <Input maxLength={100} />
          </Form.Item>
          <div className="alert-form-grid">
            <Form.Item name="metric" label="指标" rules={[{ required: true, message: '请选择指标' }]}>
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
            <Form.Item name="operator" label="判断条件" rules={[{ required: true, message: '请选择判断条件' }]}>
              <Select
                options={(selectedMetric?.operators ?? ['gt', 'gte', 'lt', 'lte']).map((operator) => ({
                  value: operator,
                  label: OPERATOR_LABELS[operator],
                }))}
              />
            </Form.Item>
            <Form.Item name="threshold" label="阈值" rules={[{ required: true, message: '请输入阈值' }]}>
              <InputNumber
                style={{ width: '100%' }}
                min={0}
                max={selectedMetric?.unit === 'percent' ? 100 : undefined}
                precision={selectedMetric?.unit === 'count' || selectedMetric?.unit === 'bytes' ? 0 : 2}
                addonAfter={metricUnitLabel(selectedMetric?.unit)}
              />
            </Form.Item>
            <Form.Item name="duration_seconds" label="持续时间" rules={[{ required: true, message: '请输入持续时间' }]}>
              <InputNumber style={{ width: '100%' }} min={0} max={604800} precision={0} addonAfter="秒" />
            </Form.Item>
          </div>
          <Form.Item name="severity" label="告警级别" rules={[{ required: true }]}>
            <Segmented
              block
              options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: meta.label }))}
            />
          </Form.Item>
          <div className="alert-switch-row">
            <Form.Item name="enabled" label="启用规则" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="notify_on_resolve" label="恢复时通知" valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          <div className="alert-form-grid">
            <Form.Item name="notify_channels" label="通知渠道" extra="不选则使用所有已配置渠道">
              <Select
                mode="multiple"
                allowClear
                placeholder="全部已配置渠道"
                options={ALERT_CHANNELS}
              />
            </Form.Item>
            <Form.Item name="silence_until" label="静默至" extra="维护窗口内不评估、不通知">
              <DatePicker showTime style={{ width: '100%' }} placeholder="不静默" />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  )
}

function metricUnitLabel(unit?: string) {
  if (unit === 'percent') return '%'
  if (unit === 'bytes') return 'Bytes'
  if (unit === 'count') return '个'
  return unit || '-'
}

function formatMetricValue(value: number, unit?: string) {
  if (unit === 'percent') return `${value.toFixed(2)}%`
  if (unit === 'bytes') return formatBytes(value)
  if (unit === 'count') return Math.round(value).toLocaleString('zh-CN')
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 1024) return `${Math.round(value)} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let unit = -1
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size.toFixed(2)} ${units[unit]}`
}
