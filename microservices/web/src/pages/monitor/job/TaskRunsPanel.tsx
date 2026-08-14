import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Card, DatePicker, Descriptions, Drawer, Form, Input,
  Result, Select, Space, Table, Tag, Tooltip, Typography,
} from 'antd'
import { EyeOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'
import {
  getTaskRun, getTaskRuns, getTaskRunSummary,
  type OpsTaskRun, type TaskRunListParams, type TaskRunSource,
  type TaskRunStatus, type TaskRunSummary,
} from '@/api/monitor'
import GlassEmpty from '@/components/GlassEmpty'
import ListFilterForm from '@/components/ListFilterForm'
import MetricCard from '@/components/MetricCard'
import StatsGrid from '@/components/StatsGrid'
import TableRowActions from '@/components/TableRowActions'
import TableToolbar from '@/components/TableToolbar'
import { useTableQuery } from '@/hooks/useTableQuery'
import { message } from '@/utils/feedback'
import { formatDateTime } from '@/utils/format'

const STATUS_META: Record<TaskRunStatus, { label: string; color: string }> = {
  running: { label: '运行中', color: 'processing' },
  succeeded: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
  cancelled: { label: '已取消', color: 'default' },
}

const SOURCE_LABELS: Record<TaskRunSource, string> = {
  worker: '服务任务',
  scheduler: '定时调度',
  'ops-cron': '主机任务',
}

const TRIGGER_LABELS: Record<string, string> = {
  scheduled: '定时触发', manual: '手动触发', shell: '主机 Cron',
  legacy: '历史回填', snapshot: '升级快照',
}

type FilterValues = {
  keyword?: string
  service?: string
  status?: TaskRunStatus
  source?: TaskRunSource
  time_range?: [Dayjs, Dayjs]
}

const initialParams: TaskRunListParams = { page: 1, page_size: 10 }

function formatDuration(value: number) {
  if (value < 1000) return `${value} ms`
  if (value < 60_000) return `${(value / 1000).toFixed(1)} s`
  return `${(value / 60_000).toFixed(1)} min`
}

export default function TaskRunsPanel() {
  const { t } = useTranslation()
  const [params, setParams] = useState<TaskRunListParams>(initialParams)
  const [summary, setSummary] = useState<TaskRunSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summaryError, setSummaryError] = useState(false)
  const [detail, setDetail] = useState<OpsTaskRun | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [form] = Form.useForm<FilterValues>()

  const fetchList = useCallback(async (query: TaskRunListParams) => {
    const result = await getTaskRuns(query)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list, total, loading, error, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: () => message.error(t('获取统一执行记录失败')),
  })

  const loadSummary = useCallback(async () => {
    setSummaryLoading(true)
    try {
      setSummary(await getTaskRunSummary(24))
      setSummaryError(false)
    } catch {
      setSummaryError(true)
    } finally {
      setSummaryLoading(false)
    }
  }, [])

  useEffect(() => { void loadSummary() }, [loadSummary])

  const reloadAll = () => {
    void reload()
    void loadSummary()
  }

  const submitFilter = (values: FilterValues) => {
    setParams({
      page: 1,
      page_size: params.page_size,
      keyword: values.keyword?.trim() || undefined,
      service: values.service?.trim() || undefined,
      status: values.status,
      source: values.source,
      start_time: values.time_range?.[0]?.toISOString(),
      end_time: values.time_range?.[1]?.toISOString(),
    })
  }

  const resetFilter = () => {
    form.resetFields()
    setParams(initialParams)
  }

  const openDetail = async (row: OpsTaskRun) => {
    setDetail(row)
    setDetailLoading(true)
    try {
      setDetail(await getTaskRun(row.id))
    } catch {
      message.error(t('获取执行详情失败'))
    } finally {
      setDetailLoading(false)
    }
  }

  const columns: ColumnsType<OpsTaskRun> = [
    {
      title: t('任务'),
      dataIndex: 'task_key',
      width: 250,
      render: (value: string, row) => (
        <div className="task-run-name">
          <Tooltip title={value}><span className="cell-mono job-cell-ellipsis">{value}</span></Tooltip>
          <Tooltip title={row.description}>
            <span className="cell-dim job-cell-ellipsis">{row.description || '—'}</span>
          </Tooltip>
        </div>
      ),
    },
    {
      title: t('来源'),
      key: 'source',
      width: 155,
      render: (_, row) => (
        <div className="task-run-source">
          <Tag variant="filled">{t(SOURCE_LABELS[row.source] ?? row.source)}</Tag>
          <span className="cell-dim">{t(TRIGGER_LABELS[row.trigger_type] ?? row.trigger_type)}</span>
        </div>
      ),
    },
    {
      title: t('服务'),
      dataIndex: 'service',
      width: 150,
      ellipsis: { showTitle: false },
      render: (value: string) => <Tooltip title={value}><span>{value}</span></Tooltip>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: TaskRunStatus) => {
        const meta = STATUS_META[value]
        return <Tag color={meta?.color}>{t(meta?.label ?? value)}</Tag>
      },
    },
    {
      title: t('耗时'),
      dataIndex: 'duration_ms',
      width: 110,
      align: 'right',
      render: (value: number, row) => row.status === 'running' ? '—' : <span className="cell-mono">{formatDuration(value)}</span>,
    },
    {
      title: t('开始时间'),
      dataIndex: 'started_at',
      width: 180,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: t('结果摘要'),
      key: 'result',
      width: 260,
      ellipsis: { showTitle: false },
      render: (_, row) => {
        const text = row.error_message || row.message
        return text ? <Tooltip title={text}><span className={row.error_message ? 'task-run-error job-cell-ellipsis' : 'job-cell-ellipsis'}>{text}</span></Tooltip> : <span className="cell-dim">—</span>
      },
    },
    {
      title: t('操作'),
      key: 'actions',
      width: 60,
      fixed: 'right',
      align: 'center',
      render: (_, row) => (
        <TableRowActions
          menuOnly
          ariaLabel={t('更多操作：{{name}}', { name: row.task_key })}
          actions={[{ key: 'detail', label: t('详情'), icon: <EyeOutlined />, onClick: () => { void openDetail(row) } }]}
        />
      ),
    },
  ]

  const emptyText = error && list.length === 0 ? (
    <Result
      status="error"
      title={t('执行记录加载失败')}
      subTitle={t('已有任务不会受影响，请稍后重试')}
      extra={<Button onClick={() => void reload()}>{t('重试')}</Button>}
    />
  ) : <GlassEmpty text="暂无统一执行记录" compact />

  return (
    <section className="task-runs-panel" aria-label={t('统一执行记录')}>
      {summaryError && (
        <Alert type="warning" showIcon message={t('任务汇总暂时不可用，执行记录仍可继续查看')} closable />
      )}
      <Card className="list-filter-card task-summary-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
        <StatsGrid className={summaryLoading ? 'task-summary-loading' : undefined}>
          <MetricCard label={t('近 24 小时执行')} value={summary?.total ?? '—'} />
          <MetricCard label={t('运行中')} value={summary?.running ?? '—'} />
          <MetricCard label={t('成功率')} value={summary ? `${summary.success_rate.toFixed(1)}%` : '—'} valueClassName="log-stat-success" />
          <MetricCard label={t('失败')} value={summary?.failed ?? '—'} valueClassName={summary?.failed ? 'log-stat-danger' : undefined} />
          <MetricCard label={t('来源服务')} value={summary?.services ?? '—'} />
        </StatsGrid>
      </Card>

      <ListFilterForm form={form} onFinish={submitFilter}>
        <Form.Item name="keyword">
          <Input allowClear prefix={<SearchOutlined />} placeholder={t('搜索任务键或说明')} style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="service">
          <Input allowClear placeholder={t('服务名')} style={{ width: 150 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select
            allowClear placeholder={t('状态')} style={{ width: 120 }}
            options={Object.entries(STATUS_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
          />
        </Form.Item>
        <Form.Item name="source">
          <Select
            allowClear placeholder={t('来源')} style={{ width: 130 }}
            options={Object.entries(SOURCE_LABELS).map(([value, label]) => ({ value, label: t(label) }))}
          />
        </Form.Item>
        <Form.Item name="time_range">
          <DatePicker.RangePicker showTime allowEmpty={[true, true]} />
        </Form.Item>
        <Form.Item className="list-filter-actions">
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
            <Button onClick={resetFilter}>{t('重置')}</Button>
          </Space>
        </Form.Item>
      </ListFilterForm>

      <Card className="list-main-card" bordered={false}>
        {error && list.length > 0 && <Alert type="warning" showIcon message={t('刷新失败，当前保留上次成功数据')} />}
        <TableToolbar
          title="统一执行记录"
          total={total}
          description={t('离散任务逐次留痕；高频轮询 worker 的活性见下方服务心跳')}
          extra={<Button icon={<ReloadOutlined />} loading={loading} onClick={reloadAll}>{t('刷新')}</Button>}
        />
        <Table
          rowKey="id"
          className="list-table task-runs-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (value) => t('共 {{n}} 条', { n: value }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Drawer
        title={detail ? t('执行详情 · {{name}}', { name: detail.task_key }) : t('执行详情')}
        open={!!detail}
        onClose={() => setDetail(null)}
        loading={detailLoading}
        size="min(720px, 100vw)"
        rootClassName="task-run-drawer"
        destroyOnHidden
      >
        {detail && (
          <Descriptions column={1} bordered size="small" items={[
            { key: 'status', label: t('状态'), children: <Tag color={STATUS_META[detail.status]?.color}>{t(STATUS_META[detail.status]?.label ?? detail.status)}</Tag> },
            { key: 'run_id', label: t('运行 ID'), children: <Typography.Text className="cell-mono" copyable>{detail.run_id}</Typography.Text> },
            { key: 'task_key', label: t('任务键'), children: <Typography.Text className="cell-mono" copyable>{detail.task_key}</Typography.Text> },
            { key: 'service', label: t('服务'), children: detail.service },
            { key: 'source', label: t('来源'), children: `${t(SOURCE_LABELS[detail.source] ?? detail.source)} · ${t(TRIGGER_LABELS[detail.trigger_type] ?? detail.trigger_type)}` },
            { key: 'attempt', label: t('尝试次数'), children: detail.attempt },
            { key: 'correlation', label: t('关联 ID'), children: detail.correlation_id ? <Typography.Text copyable>{detail.correlation_id}</Typography.Text> : '—' },
            { key: 'started', label: t('开始时间'), children: formatDateTime(detail.started_at) },
            { key: 'finished', label: t('结束时间'), children: detail.finished_at ? formatDateTime(detail.finished_at) : '—' },
            { key: 'duration', label: t('耗时'), children: detail.status === 'running' ? '—' : formatDuration(detail.duration_ms) },
            { key: 'message', label: t('输出'), children: <Typography.Text className="task-run-detail-text" copyable={!!detail.message}>{detail.message || '—'}</Typography.Text> },
            { key: 'error', label: t('错误摘要'), children: <Typography.Text type={detail.error_message ? 'danger' : undefined} className="task-run-detail-text" copyable={!!detail.error_message}>{detail.error_message || '—'}</Typography.Text> },
          ]} />
        )}
      </Drawer>
    </section>
  )
}
