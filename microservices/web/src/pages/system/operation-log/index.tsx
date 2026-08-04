import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, Modal, Descriptions, DatePicker,
  InputNumber, Drawer, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, EyeOutlined, DownloadOutlined, ClearOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OperationLog } from '@/types'
import {
  getOperationLogList, getOperationLogDetail, exportOperationLogs, clearOperationLogs,
  getOperationLogStats, type OperationLogStats,
} from '@/api/system/log'
import TableToolbar from '@/components/TableToolbar'
import CountUpValue from '@/components/CountUpValue'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
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
  const [list, setList] = useState<OperationLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [detailOpen, setDetailOpen] = useState(false)
  const [detail, setDetail] = useState<OperationLog | null>(null)
  const [exporting, setExporting] = useState(false)
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [stats, setStats] = useState<OperationLogStats | null>(null)
  const [searchForm] = Form.useForm()
  const [clearForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const loadStats = () => {
    getOperationLogStats().then(setStats).catch(() => setStats(null))
  }

  useEffect(() => {
    loadStats()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getOperationLogList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取操作日志失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

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
      setDetail(res)
      setDetailOpen(true)
    } catch {
      message.error('获取详情失败')
    }
  }

  const handleExport = async () => {
    setExporting(true)
    try {
      const { page: _p, page_size: _ps, ...filters } = params
      await exportOperationLogs(filters)
      message.success('导出成功')
    } catch {
      message.error('导出失败')
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
      message.success(`清理成功，共删除 ${res.deleted_count} 条日志`)
      setClearOpen(false)
      fetchList({ ...params, page: 1 })
      // 顶部统计与列表同屏，清完必须一起刷新，否则两块数据互相矛盾
      loadStats()
    } catch {
      message.error('清理失败')
    } finally {
      setClearing(false)
    }
  }

  const columns: ColumnsType<OperationLog> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    { title: '用户名', dataIndex: 'username', width: 160, ellipsis: true, responsive: ['sm'], render: (v: string) => <ScanText value={v} /> },
    {
      title: '方法',
      dataIndex: 'method',
      width: 90,
      responsive: ['md'],
      render: (v: string) => <Tag variant="filled" className="cell-mono operation-method-tag">{v}</Tag>,
    },
    {
      title: '路径',
      dataIndex: 'path',
      width: 300,
      ellipsis: true,
      render: (v: string) => <ScanText value={v} mono />,
    },
    { title: '模块', dataIndex: 'module', width: 140, ellipsis: true, responsive: ['md'], render: (v: string) => <ScanText value={v} /> },
    { title: '动作', dataIndex: 'action', width: 160, ellipsis: true, responsive: ['lg'], render: (v: string) => <ScanText value={v} mono /> },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      responsive: ['sm'],
      render: (v: number) => <Tag color={statusColor(v)} variant="filled" className="cell-mono operation-status-tag">{v}</Tag>,
    },
    {
      title: '耗时',
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
    { title: '时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['md'] },
    {
      title: '操作',
      width: 64,
      fixed: 'right',
      render: (_, record) => (
        <Tooltip title="查看详情">
          <Button
            type="text"
            size="small"
            className="operation-row-action"
            aria-label="查看操作日志详情"
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
          <div className="log-stats-row">
            <div className="log-stat">
              <span className="log-stat-label">近 7 天操作</span>
              <span className="log-stat-value"><CountUpValue value={stats.total} /></span>
            </div>
            <div className="log-stat">
              <span className="log-stat-label">异常请求</span>
              <span className={`log-stat-value ${stats.error_count > 0 ? 'log-stat-danger' : 'log-stat-success'}`}>
                <CountUpValue value={stats.error_count} />
              </span>
            </div>
            {Object.keys(stats.by_method ?? {}).length > 0 && (
              <>
                <div className="log-stat-divider" />
                <div className="log-stat">
                  <span className="log-stat-label">方法分布</span>
                  <span className="operation-stat-tags">
                    {Object.entries(stats.by_method ?? {}).map(([m, n]) => (
                      <Tag key={m} variant="filled" className="cell-mono operation-method-tag">
                        {m} {n}
                      </Tag>
                    ))}
                  </span>
                </div>
              </>
            )}
            {topModules.length > 0 && (
              <>
                <div className="log-stat-divider" />
                <div className="log-stat">
                  <span className="log-stat-label">活跃模块 Top{topModules.length}</span>
                  <span className="operation-stat-tags">
                    {topModules.map(([m, n]) => (
                      <Tooltip key={m} title={m}>
                        <Tag variant="filled" className="operation-module-tag">{m} {n}</Tag>
                      </Tooltip>
                    ))}
                  </span>
                </div>
              </>
            )}
          </div>
        </Card>
      )}

      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
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
            <Input placeholder="搜索用户名" prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="method">
            <Select placeholder="方法" style={{ width: 90 }} allowClear>
              {['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].map((m) => (
                <Select.Option key={m} value={m}>{m}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="path">
            <Input placeholder="路径" allowClear style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="module">
            <Input placeholder="模块" allowClear style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="dateRange">
            <RangePicker showTime format="YYYY-MM-DD HH:mm:ss" />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
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
                  清理日志
                </Button>
              )}
              <Button icon={<DownloadOutlined />} onClick={handleExport} loading={exporting}>
                导出 CSV
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
            </Space>
          }
        />
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
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Drawer
        title="请求诊断"
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width="min(720px, 100vw)"
        destroyOnHidden
      >
        {detail && (
          <div className="operation-detail-content">
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label="用户">{detail.username || '-'}</Descriptions.Item>
              <Descriptions.Item label="时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              <Descriptions.Item label="方法 / 状态">
                <Tag variant="filled" className="cell-mono operation-method-tag">{detail.method}</Tag>
                <Tag color={statusColor(detail.status)} variant="filled" className="cell-mono operation-status-tag">{detail.status}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="耗时">
                {typeof detail.latency === 'number' ? (
                  <span className={`cell-mono ${latencyClass(detail.latency)}`}>
                    {detail.latency}ms
                  </span>
                ) : (
                  '-'
                )}
              </Descriptions.Item>
              <Descriptions.Item label="路径" span={2}>
                <span className="cell-mono operation-detail-long">
                  {detail.path}
                  {detail.query ? `?${detail.query}` : ''}
                </span>
              </Descriptions.Item>
              <Descriptions.Item label="模块 / 动作">
                {[detail.module, detail.action].filter(Boolean).join(' / ') || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="IP">
                <span className="cell-mono">{detail.ip || '-'}</span>
              </Descriptions.Item>
              <Descriptions.Item label="请求ID" span={2}>
                <span className="cell-mono operation-detail-long">{detail.request_id || '-'}</span>
              </Descriptions.Item>
              {detail.user_agent && (
                <Descriptions.Item label="User-Agent" span={2}>
                  <span className="card-extra-note operation-detail-long">{detail.user_agent}</span>
                </Descriptions.Item>
              )}
            </Descriptions>

            {detail.error_msg && (
              <div className="log-detail-block log-detail-error">
                <div className="log-detail-block-title">错误信息</div>
                <pre>{detail.error_msg}</pre>
              </div>
            )}
            {detail.request_body && (
              <div className="log-detail-block">
                <div className="log-detail-block-title">请求体</div>
                <pre>{tryPrettyJson(detail.request_body)}</pre>
              </div>
            )}
            {detail.response_body && (
              <div className="log-detail-block">
                <div className="log-detail-block-title">响应体</div>
                <pre>{tryPrettyJson(detail.response_body)}</pre>
              </div>
            )}
          </div>
        )}
      </Drawer>

      <Modal
        title="清理操作日志"
        open={clearOpen}
        onOk={handleClear}
        onCancel={() => setClearOpen(false)}
        confirmLoading={clearing}
        okButtonProps={{ danger: true }}
        okText="确认清理"
        destroyOnHidden
      >
        <Form form={clearForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="days"
            label="保留最近天数（早于该范围的日志将被删除，不可恢复）"
            rules={[{ required: true, message: '请输入保留天数' }]}
            initialValue={30}
          >
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
