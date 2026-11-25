import { useTranslation } from 'react-i18next'
import {
  Alert,
  Button,
  Card,
  Dropdown,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  type FormInstance,
  type MenuProps,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import {
  DeleteOutlined,
  EditOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type {
  AlertMetricDefinition,
  AlertRuleListParams,
  AlertRuleState,
  AlertSeverity,
  MonitorAlertRule,
} from '@/api/monitor'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import TableRowActions from '@/components/common/TableRowActions'
import TableToolbar from '@/components/common/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { formatDateTime } from '@/utils/format'
import {
  defaultRuleParams,
  formatMetricValue,
  OPERATOR_LABELS,
  SEVERITY_META,
  STATE_META,
  type AlertRuleFilterValues,
} from './helpers'

interface RulesTabProps {
  filterForm: FormInstance<AlertRuleFilterValues>
  rules: MonitorAlertRule[]
  total: number
  params: AlertRuleListParams
  loading: boolean
  loaded: boolean
  failed: boolean
  metrics: AlertMetricDefinition[]
  metricMap: Map<string, AlertMetricDefinition>
  compactTable: boolean
  fixActionsRight: boolean
  evaluatingID: number | null
  onParamsChange: (next: AlertRuleListParams) => void
  onRefresh: () => void
  onCreate: () => void
  onEdit: (rule: MonitorAlertRule) => void
  onDelete: (rule: MonitorAlertRule) => Promise<void>
  onEvaluate: (rule: MonitorAlertRule) => Promise<void>
}

export default function RulesTab({
  filterForm,
  rules,
  total,
  params,
  loading,
  loaded,
  failed,
  metrics,
  metricMap,
  compactTable,
  fixActionsRight,
  evaluatingID,
  onParamsChange,
  onRefresh,
  onCreate,
  onEdit,
  onDelete,
  onEvaluate,
}: RulesTabProps) {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const [confirmModal, confirmContextHolder] = Modal.useModal()

  const confirmRemoveRule = (rule: MonitorAlertRule) => {
    confirmModal.confirm({
      title: t('删除告警规则'),
      content: t('确认删除“{{name}}”？历史告警事件仍会保留。', { name: rule.name }),
      okText: t('删除'),
      cancelText: t('取消'),
      okButtonProps: { danger: true },
      onOk: () => onDelete(rule),
    })
  }

  const compactRuleActions = (rule: MonitorAlertRule): MenuProps['items'] => [
    hasPerm('system:alert:evaluate') ? {
      key: 'evaluate',
      icon: <ThunderboltOutlined />,
      label: t('立即评估'),
      disabled: !rule.enabled,
    } : null,
    hasPerm('system:alert:update') ? {
      key: 'edit',
      icon: <EditOutlined />,
      label: t('编辑规则'),
    } : null,
    hasPerm('system:alert:delete') ? {
      key: 'delete',
      icon: <DeleteOutlined />,
      label: t('删除规则'),
      danger: true,
    } : null,
  ].filter((item): item is NonNullable<typeof item> => item !== null)

  const runCompactRuleAction = (key: string, rule: MonitorAlertRule) => {
    if (key === 'evaluate') void onEvaluate(rule)
    if (key === 'edit') onEdit(rule)
    if (key === 'delete') confirmRemoveRule(rule)
  }

  const desktopRuleColumns: ColumnsType<MonitorAlertRule> = [
    {
      title: t('规则'),
      dataIndex: 'name',
      width: 180,
      ellipsis: { showTitle: false },
      render: (name: string, rule) => (
        <div className="alert-rule-name">
          <Tooltip title={name} placement="topLeft"><span>{name}</span></Tooltip>
          {!rule.enabled && <Tag>{t('停用')}</Tag>}
        </div>
      ),
    },
    {
      title: t('指标'),
      dataIndex: 'metric',
      width: 190,
      render: (metric: string) => (
        <Tooltip title={metricMap.get(metric)?.description ?? metric}>
          <span className="cell-mono">{metricMap.get(metric)?.title ?? metric}</span>
        </Tooltip>
      ),
    },
    {
      title: t('条件'),
      width: 135,
      render: (_, rule) => (
        <span className="alert-condition">
          {t(OPERATOR_LABELS[rule.operator])} {formatMetricValue(rule.threshold, metricMap.get(rule.metric)?.unit)}
        </span>
      ),
    },
    {
      title: t('持续'),
      dataIndex: 'duration_seconds',
      width: 84,
      responsive: ['xl'],
      render: (seconds: number) => seconds === 0 ? t('立即') : t('{{n}} 秒', { n: seconds }),
    },
    {
      title: t('级别'),
      dataIndex: 'severity',
      width: 80,
      responsive: ['lg'],
      render: (severity: AlertSeverity) => (
        <Tag color={SEVERITY_META[severity].color}>{t(SEVERITY_META[severity].label)}</Tag>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'state',
      width: 140,
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
      title: t('最近值'),
      dataIndex: 'last_value',
      width: 118,
      render: (value: number | null | undefined, rule) =>
        value == null ? '-' : formatMetricValue(value, metricMap.get(rule.metric)?.unit),
    },
    {
      title: t('最近评估'),
      dataIndex: 'last_evaluated_at',
      width: 170,
      className: 'cell-time',
      responsive: ['xl'],
      render: formatDateTime,
    },
    {
      title: t('操作'),
      width: 116,
      fixed: fixActionsRight ? 'right' : undefined,
      align: 'center' as const,
      className: 'alert-rule-actions-cell',
      render: (_, rule) => (
        <TableRowActions
          className="alert-rule-actions"
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: rule.name })}
          actions={[
            {
              key: 'evaluate',
              label: rule.enabled ? t('立即使用真实指标评估') : t('停用规则不能评估'),
              icon: <ThunderboltOutlined />,
              show: hasPerm('system:alert:evaluate'),
              disabled: !rule.enabled,
              loading: evaluatingID === rule.id,
              onClick: () => { void onEvaluate(rule) },
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:alert:update'),
              onClick: () => onEdit(rule),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:alert:delete'),
              confirm: t('删除规则后历史事件仍会保留，确认删除?'),
              onClick: () => { void onDelete(rule) },
            },
          ]}
        />
      ),
    },
  ]

  const compactRuleColumns: ColumnsType<MonitorAlertRule> = [
    {
      title: t('规则'),
      width: 156,
      render: (_, rule) => {
        const metric = metricMap.get(rule.metric)
        return (
          <div className="alert-rule-compact-main">
            <div className="alert-rule-name">
              <Tooltip title={rule.name} placement="topLeft"><span>{rule.name}</span></Tooltip>
              {!rule.enabled && <Tag>{t('停用')}</Tag>}
            </div>
            <Tooltip title={metric?.description ?? rule.metric} placement="topLeft">
              <span className="alert-rule-compact-meta">
                {metric?.title ?? rule.metric} · {t(OPERATOR_LABELS[rule.operator])} {formatMetricValue(rule.threshold, metric?.unit)}
              </span>
            </Tooltip>
          </div>
        )
      },
    },
    {
      title: t('状态'),
      width: 116,
      render: (_, rule) => {
        const metric = metricMap.get(rule.metric)
        return (
          <div className="alert-rule-compact-state">
            <Tooltip title={rule.state === 'error' ? rule.last_error : undefined}>
              <span>
                <StatusPill
                  tone={STATE_META[rule.state].tone}
                  label={rule.state === 'error' && rule.firing_since ? '告警 / 异常' : STATE_META[rule.state].label}
                  pulse={rule.state === 'firing' || (rule.state === 'error' && Boolean(rule.firing_since))}
                />
              </span>
            </Tooltip>
            <span>{t('最近值 {{n}}', { n: rule.last_value == null ? '--' : formatMetricValue(rule.last_value, metric?.unit) })}</span>
            <span>{rule.last_evaluated_at ? dayjs(rule.last_evaluated_at).format('MM-DD HH:mm') : t('尚未评估')}</span>
          </div>
        )
      },
    },
    {
      title: <Tooltip title={t('操作')}><MoreOutlined /></Tooltip>,
      width: 48,
      align: 'center',
      className: 'alert-rule-actions-cell',
      render: (_, rule) => {
        const items = compactRuleActions(rule) ?? []
        return items.length > 0 ? (
          <Dropdown
            trigger={['click']}
            menu={{ items, onClick: ({ key }) => runCompactRuleAction(key, rule) }}
          >
            <Button
              type="text"
              size="small"
              icon={<MoreOutlined />}
              aria-label={t('更多操作：{{name}}', { name: rule.name })}
              loading={evaluatingID === rule.id}
            />
          </Dropdown>
        ) : <span className="cell-muted">--</span>
      },
    },
  ]

  const ruleColumns = compactTable ? compactRuleColumns : desktopRuleColumns

  return (
    <>
      {confirmContextHolder}
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={filterForm}
          layout="inline"
          className="list-filter-form"
          onFinish={(values) => onParamsChange({ ...defaultRuleParams, ...values })}
        >
          <Form.Item name="name">
            <Input placeholder={t('规则名称')} prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="metric">
            <Select
              placeholder={t('指标')}
              allowClear
              showSearch
              optionFilterProp="label"
              options={metrics.map((metric) => ({ value: metric.key, label: metric.title }))}
            />
          </Form.Item>
          <Form.Item name="state">
            <Select
              placeholder={t('状态')}
              allowClear
              options={Object.entries(STATE_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
            />
          </Form.Item>
          <Form.Item name="enabled">
            <Select
              placeholder={t('启用状态')}
              allowClear
              options={[{ value: true, label: t('启用') }, { value: false, label: t('停用') }]}
            />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => {
                  filterForm.resetFields()
                  onParamsChange(defaultRuleParams)
                }}
              >
                {t('重置')}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="告警规则"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={onRefresh}>{t('刷新')}</Button>
              {hasPerm('system:alert:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={onCreate}>{t('新增规则')}</Button>
              )}
            </Space>
          }
        />
        {failed && (
          <Alert
            className="alert-list-state"
            type={loaded ? 'warning' : 'error'}
            showIcon
            message={loaded ? t('规则列表刷新失败，当前显示上次成功数据') : t('告警规则列表暂不可用')}
            action={(
              <Button size="small" icon={<ReloadOutlined />} onClick={onRefresh}>
                {t('重试')}
              </Button>
            )}
          />
        )}
        <Table
          rowKey="id"
          className="list-table"
          columns={ruleColumns}
          dataSource={rules}
          loading={loading}
          tableLayout={compactTable ? 'fixed' : 'auto'}
          scroll={{ x: compactTable ? 320 : 'max-content' }}
          locale={{
            emptyText: <GlassEmpty text={failed && !loaded ? '告警规则暂不可用' : '暂无告警规则'} compact />,
          }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => onParamsChange({ ...params, page, page_size }),
          }}
        />
      </Card>
    </>
  )
}
