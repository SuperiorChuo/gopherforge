import { useTranslation } from 'react-i18next'
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  type FormInstance,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type {
  AlertEventListParams,
  AlertMetricDefinition,
  AlertSeverity,
  MonitorAlertEvent,
} from '@/api/monitor'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import { formatDateTime } from '@/utils/format'
import {
  defaultEventParams,
  formatMetricValue,
  NOTIFY_META,
  SEVERITY_META,
  type AlertEventFilterValues,
} from './helpers'

interface EventsTabProps {
  filterForm: FormInstance<AlertEventFilterValues>
  events: MonitorAlertEvent[]
  total: number
  params: AlertEventListParams
  loading: boolean
  loaded: boolean
  failed: boolean
  metricMap: Map<string, AlertMetricDefinition>
  compactTable: boolean
  onParamsChange: (next: AlertEventListParams) => void
  onRefresh: () => void
}

export default function EventsTab({
  filterForm,
  events,
  total,
  params,
  loading,
  loaded,
  failed,
  metricMap,
  compactTable,
  onParamsChange,
  onRefresh,
}: EventsTabProps) {
  const { t } = useTranslation()

  const desktopEventColumns: ColumnsType<MonitorAlertEvent> = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: t('规则'),
      dataIndex: 'rule_name',
      width: 180,
      ellipsis: { showTitle: false },
      render: (name: string) => <Tooltip title={name} placement="topLeft"><span>{name}</span></Tooltip>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (status: MonitorAlertEvent['status']) => status === 'firing'
        ? <StatusPill tone="danger" label="触发" pulse />
        : <StatusPill tone="success" label="恢复" pulse={false} />,
    },
    {
      title: t('级别'),
      dataIndex: 'severity',
      width: 80,
      responsive: ['sm'],
      render: (severity: AlertSeverity) => (
        <Tag color={SEVERITY_META[severity].color}>{t(SEVERITY_META[severity].label)}</Tag>
      ),
    },
    {
      title: t('指标值'),
      width: 210,
      render: (_, event) => (
        <span className="cell-mono">
          {metricMap.get(event.metric)?.title ?? event.metric}: {formatMetricValue(event.value, metricMap.get(event.metric)?.unit)}
        </span>
      ),
    },
    {
      title: t('通知'),
      dataIndex: 'notify_status',
      width: 100,
      responsive: ['md'],
      render: (status: string, event) => (
        <Tooltip title={event.notify_error || undefined}>
          <Tag color={NOTIFY_META[status]?.color}>{NOTIFY_META[status] ? t(NOTIFY_META[status].label) : status}</Tag>
        </Tooltip>
      ),
    },
    {
      title: t('事件内容'),
      dataIndex: 'message',
      width: 300,
      ellipsis: { showTitle: false },
      responsive: ['lg'],
      render: (value: string) => <Tooltip title={value} placement="topLeft"><span>{value}</span></Tooltip>,
    },
  ]

  const compactEventColumns: ColumnsType<MonitorAlertEvent> = [
    {
      title: t('事件'),
      width: 202,
      render: (_, event) => {
        const metric = metricMap.get(event.metric)
        return (
          <div className="alert-event-compact-main">
            <Tooltip title={event.rule_name} placement="topLeft">
              <strong>{event.rule_name}</strong>
            </Tooltip>
            <span>{dayjs(event.created_at).format('MM-DD HH:mm:ss')}</span>
            <span className="cell-mono">
              {metric?.title ?? event.metric}: {formatMetricValue(event.value, metric?.unit)}
            </span>
            <Tooltip title={event.message} placement="topLeft">
              <span className="alert-event-compact-message">{event.message}</span>
            </Tooltip>
          </div>
        )
      },
    },
    {
      title: t('状态'),
      width: 108,
      render: (_, event) => (
        <div className="alert-event-compact-state">
          {event.status === 'firing'
            ? <StatusPill tone="danger" label="触发" pulse />
            : <StatusPill tone="success" label="恢复" pulse={false} />}
          <Tag color={SEVERITY_META[event.severity].color}>{t(SEVERITY_META[event.severity].label)}</Tag>
          <Tooltip title={event.notify_error || undefined}>
            <Tag color={NOTIFY_META[event.notify_status]?.color}>
              {NOTIFY_META[event.notify_status] ? t(NOTIFY_META[event.notify_status].label) : event.notify_status}
            </Tag>
          </Tooltip>
        </div>
      ),
    },
  ]

  const eventColumns = compactTable ? compactEventColumns : desktopEventColumns

  return (
    <>
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={filterForm}
          layout="inline"
          className="list-filter-form"
          onFinish={(values) => onParamsChange({ ...defaultEventParams, ...values })}
        >
          <Form.Item name="rule_name">
            <Input placeholder={t('规则名称')} prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('事件状态')} allowClear options={[{ value: 'firing', label: t('触发') }, { value: 'resolved', label: t('恢复') }]} />
          </Form.Item>
          <Form.Item name="severity">
            <Select
              placeholder={t('级别')}
              allowClear
              options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
            />
          </Form.Item>
          <Form.Item name="notify_status">
            <Select
              placeholder={t('通知结果')}
              allowClear
              options={Object.entries(NOTIFY_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
            />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => {
                  filterForm.resetFields()
                  onParamsChange(defaultEventParams)
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
          title="告警事件"
          total={total}
          extra={<Button icon={<ReloadOutlined />} onClick={onRefresh}>{t('刷新')}</Button>}
        />
        {failed && (
          <Alert
            className="alert-list-state"
            type={loaded ? 'warning' : 'error'}
            showIcon
            message={loaded ? t('事件列表刷新失败，当前显示上次成功数据') : t('告警事件列表暂不可用')}
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
          columns={eventColumns}
          dataSource={events}
          loading={loading}
          tableLayout={compactTable ? 'fixed' : 'auto'}
          scroll={{ x: compactTable ? 310 : 'max-content' }}
          locale={{
            emptyText: <GlassEmpty text={failed && !loaded ? '告警事件暂不可用' : '暂无告警事件'} compact />,
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
