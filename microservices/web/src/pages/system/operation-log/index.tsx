import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, Descriptions, DatePicker,
  InputNumber, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, EyeOutlined, DownloadOutlined, ClearOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OperationLog } from '@/types'
import {
  getOperationLogList, getOperationLogDetail, exportOperationLogs, clearOperationLogs,
  getOperationLogStats, type OperationLogStats,
} from '@/api/system/log'
import EntityDetailDrawer from '@/components/common/EntityDetailDrawer'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import CountUpValue from '@/components/common/CountUpValue'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid, { StatsGridDivider } from '@/components/common/StatsGrid'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import dayjs from 'dayjs'
import './styles.css'

const { RangePicker } = DatePicker

interface SearchParams {
  username?: string
  method?: string
  path?: string
  module?: string
  status?: number
  start_time?: string
  end_time?: string
  page: number
  page_size: number
}

function statusColor(status: number): string {
  if (status >= 500) return 'error'
  if (status >= 400) return 'warning'
  if (status >= 200 && status < 300) return 'success'
  return 'default'
}

function tryPrettyJson(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function latencyClass(ms: number): string {
  return ms > 1000 ? 'latency-high' : ms > 300 ? 'latency-mid' : 'latency-low'
}

function ScanText({ value, mono = false }: { value?: string; mono?: boolean }) {
  const text = value || '—'
  return (
    <Tooltip title={value || undefined}>
      <span className={`operation-scan-text${mono ? ' cell-mono' : ''}`}>{text}</span>
    </Tooltip>
  )
}

export default function OperationLogPage() {
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const detailModal = useCrudModal<OperationLog>()
  const [exporting, setExporting] = useState(false)
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [stats, setStats] = useState<OperationLogStats | null>(null)
  const [searchForm] = Form.useForm()
  const [clearForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

  const loadStats = () => {
    getOperationLogStats().then(setStats).catch(() => setStats(null))
  }

  useEffect(() => {
    loadStats()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const fetchList = useCallback(async (p: SearchParams) => {
    const res = await getOperationLogList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => message.error(t('获取操作日志失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onLoadError,
  })

  const handleSearch = (values: {
    username?: string
    method?: string
    path?: string
    module?: string
    status?: number
    dateRange?: [dayjs.Dayjs, dayjs.Dayjs]
  }) => {
    const { dateRange, ...rest } = values
    setParams({
      ...params,
      page: 1,
      ...rest,
      start_time: dateRange?.[0]?.format('YYYY-MM-DD HH:mm:ss'),
      end_time: dateRange?.[1]?.format('YYYY-MM-DD HH:mm:ss'),
    })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openDetail = async (id: number) => {
    try {
      const res = await getOperationLogDetail(id)
      detailModal.openEdit(res)
    } catch {
      message.error(t('获取详情失败'))
    }
  }

  const handleExport = async () => {
    setExporting(true)
    try {
      const { page: _p, page_size: _ps, ...filters } = params
      await exportOperationLogs(filters)
      message.success(t('导出成功'))
    } catch {
      message.error(t('导出失败'))
    } finally {
      setExporting(false)
    }
  }

  const handleClear = async () => {
    const values = await clearForm.validateFields().catch(() => null)
    if (!values) return
    setClearing(true)
    try {
      const res = await clearOperationLogs(values.days)
      message.success(t('清理成功，共删除 {{n}} 条日志', { n: res.deleted_count }))
      setClearOpen(false)
      setParams({ ...params, page: 1 })
      // 顶部统计与列表同屏，清完必须一起刷新，否则两块数据互相矛盾
      loadStats()
    } catch {
      message.error(t('清理失败'))
    } finally {
      setClearing(false)
    }
  }

  const columns: ColumnsType<OperationLog> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    { title: t('用户名'), dataIndex: 'username', width: 160, ellipsis: true, responsive: ['sm'], render: (v: string) => <ScanText value={v} /> },
    {
      title: t('方法'),
      dataIndex: 'method',
      width: 90,
      responsive: ['md'],
      render: (v: string) => <Tag variant="filled" className="cell-mono operation-method-tag">{v}</Tag>,
    },
    {
      title: t('路径'),
      dataIndex: 'path',
      width: 300,
      ellipsis: true,
      render: (v: string) => <ScanText value={v} mono />,
    },
    { title: t('模块'), dataIndex: 'module', width: 140, ellipsis: true, responsive: ['md'], render: (v: string) => <ScanText value={v} /> },
    { title: t('动作'), dataIndex: 'action', width: 160, ellipsis: true, responsive: ['lg'], render: (v: string) => <ScanText value={v} mono /> },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      responsive: ['sm'],
      render: (v: number) => <Tag color={statusColor(v)} variant="filled" className="cell-mono operation-status-tag">{v}</Tag>,
    },
    {
      title: t('耗时'),
      dataIndex: 'latency',
      width: 90,
      responsive: ['lg'],
      render: (v?: number) =>
        typeof v === 'number' ? (
          <span className={`cell-mono ${latencyClass(v)}`}>{v}ms</span>
        ) : (
          <span className="cell-muted">—</span>
        ),
    },
    { title: t('时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['md'] },
    {
      title: t('操作'),
      width: 64,
      fixed: 'right',
      render: (_, record) => (
        <Tooltip title={t('查看详情')}>
          <Button
            type="text"
            size="small"
            className="operation-row-action"
            aria-label={t('查看操作日志详情')}
            icon={<EyeOutlined />}
            onClick={() => openDetail(record.id)}
          />
        </Tooltip>
      ),
    },
  ]

  const topModules = Object.entries(stats?.by_module ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)

  return (
    <div className="page-list operation-log-page">
      {stats && (
        <Card className="list-filter-card operation-stats-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
          <StatsGrid className="operation-stats-grid">
            <MetricCard label={t('近 7 天操作')} value={<CountUpValue value={stats.total} />} />
            <MetricCard
              label={t('异常请求')}
              value={<CountUpValue value={stats.error_count} />}
              valueClassName={stats.error_count > 0 ? 'log-stat-danger' : 'log-stat-success'}
            />
            {Object.keys(stats.by_method ?? {}).length > 0 && (
              <>
                <StatsGridDivider />
                <MetricCard
                  label={t('方法分布')}
                  value={(
                    <span className="operation-stat-tags">
                      {Object.entries(stats.by_method ?? {}).map(([m, n]) => (
                        <Tag key={m} variant="filled" className="cell-mono operation-method-tag">
                          {m} {n}
                        </Tag>
                      ))}
                    </span>
                  )}
                />
              </>
            )}
            {topModules.length > 0 && (
              <>
                <StatsGridDivider />
                <MetricCard
                  label={t('活跃模块 Top{{n}}', { n: topModules.length })}
                  value={(
                    <span className="operation-stat-tags">
                      {topModules.map(([m, n]) => (
                        <Tooltip key={m} title={m}>
                          <Tag variant="filled" className="operation-module-tag">{m} {n}</Tag>
                        </Tooltip>
                      ))}
                    </span>
                  )}
                />
              </>
            )}
          </StatsGrid>
        </Card>
      )}

      <ListPageShell
        filter={(
          <ListFilterForm
            form={searchForm}
          onFinish={handleSearch}
          initialValues={{
            ...params,
            // URL 里存 start_time/end_time 字符串,表单字段是 dateRange——
            // 不反解的话刷新后时间过滤仍生效但选择器显示为空
            dateRange:
              params.start_time && params.end_time
                ? [dayjs(params.start_time), dayjs(params.end_time)]
                : undefined,
          }}
        >
          <Form.Item name="username">
            <Input placeholder={t('搜索用户名')} prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="method">
            <Select placeholder={t('方法')} style={{ width: 90 }} allowClear>
              {['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].map((m) => (
                <Select.Option key={m} value={m}>{m}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="path">
            <Input placeholder={t('路径')} allowClear style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="module">
            <Input placeholder={t('模块')} allowClear style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="dateRange">
            <RangePicker showTime format="YYYY-MM-DD HH:mm:ss" />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
            title="操作日志"
            total={total}
          extra={
            <Space wrap className="operation-toolbar-actions">
              {hasPerm('system:log:operation:clear') && (
                <Button
                  danger
                  icon={<ClearOutlined />}
                  onClick={() => { clearForm.resetFields(); setClearOpen(true) }}
                >
                  {t('清理日志')}
                </Button>
              )}
              <Button icon={<DownloadOutlined />} onClick={handleExport} loading={exporting}>
                {t('导出 CSV')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
          />
        )}
        >
        <Table
          rowKey="id"
          className="list-table operation-log-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无操作记录" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </ListPageShell>

      <EntityDetailDrawer
        title={t('请求诊断')}
        open={detailModal.open}
        onClose={detailModal.close}
        width="min(720px, 100vw)"
        destroyOnHidden
      >
        {detailModal.record && (
          <div className="operation-detail-content">
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label={t('用户')}>{detailModal.record.username || '-'}</Descriptions.Item>
              <Descriptions.Item label={t('时间')}>{formatDateTime(detailModal.record.created_at)}</Descriptions.Item>
              <Descriptions.Item label={t('方法 / 状态')}>
                <Tag variant="filled" className="cell-mono operation-method-tag">{detailModal.record.method}</Tag>
                <Tag color={statusColor(detailModal.record.status)} variant="filled" className="cell-mono operation-status-tag">{detailModal.record.status}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label={t('耗时')}>
                {typeof detailModal.record.latency === 'number' ? (
                  <span className={`cell-mono ${latencyClass(detailModal.record.latency)}`}>
                    {detailModal.record.latency}ms
                  </span>
                ) : (
                  '-'
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('路径')} span={2}>
                <span className="cell-mono operation-detail-long">
                  {detailModal.record.path}
                  {detailModal.record.query ? `?${detailModal.record.query}` : ''}
                </span>
              </Descriptions.Item>
              <Descriptions.Item label={t('模块 / 动作')}>
                {[detailModal.record.module, detailModal.record.action].filter(Boolean).join(' / ') || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="IP">
                <span className="cell-mono">{detailModal.record.ip || '-'}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('请求ID')} span={2}>
                <span className="cell-mono operation-detail-long">{detailModal.record.request_id || '-'}</span>
              </Descriptions.Item>
              {detailModal.record.user_agent && (
                <Descriptions.Item label="User-Agent" span={2}>
                  <span className="card-extra-note operation-detail-long">{detailModal.record.user_agent}</span>
                </Descriptions.Item>
              )}
            </Descriptions>

            {detailModal.record.error_msg && (
              <div className="log-detail-block log-detail-error">
                <div className="log-detail-block-title">{t('错误信息')}</div>
                <pre>{detailModal.record.error_msg}</pre>
              </div>
            )}
            {detailModal.record.request_body && (
              <div className="log-detail-block">
                <div className="log-detail-block-title">{t('请求体')}</div>
                <pre>{tryPrettyJson(detailModal.record.request_body)}</pre>
              </div>
            )}
            {detailModal.record.response_body && (
              <div className="log-detail-block">
                <div className="log-detail-block-title">{t('响应体')}</div>
                <pre>{tryPrettyJson(detailModal.record.response_body)}</pre>
              </div>
            )}
          </div>
        )}
      </EntityDetailDrawer>

      <EntityFormModal
        title={t('清理操作日志')}
        open={clearOpen}
        form={clearForm}
        onSubmit={handleClear}
        onClose={() => setClearOpen(false)}
        submitting={clearing}
        okButtonProps={{ danger: true }}
        okText={t('确认清理')}
        destroyOnHidden
        formProps={{ style: { marginTop: 16 } }}
      >
          <Form.Item
            name="days"
            label={t('保留最近天数（早于该范围的日志将被删除，不可恢复）')}
            rules={[{ required: true, message: t('请输入保留天数') }]}
            initialValue={30}
          >
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
      </EntityFormModal>
    </div>
  )
}
