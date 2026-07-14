import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, Modal, Descriptions, DatePicker,
  InputNumber, Drawer,
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
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import dayjs from 'dayjs'

const { RangePicker } = DatePicker

const methodColors: Record<string, string> = {
  GET: 'blue', POST: 'green', PUT: 'gold', DELETE: 'red', PATCH: 'purple',
}

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

  useEffect(() => {
    getOperationLogStats().then(setStats).catch(() => setStats(null))
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
    } catch {
      message.error('清理失败')
    } finally {
      setClearing(false)
    }
  }

  const columns: ColumnsType<OperationLog> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    {
      title: '方法',
      dataIndex: 'method',
      width: 90,
      render: (v: string) => <Tag color={methodColors[v] ?? 'default'} variant="filled">{v}</Tag>,
    },
    {
      title: '路径',
      dataIndex: 'path',
      ellipsis: true,
      render: (v: string) => <span className="cell-mono cell-dim">{v}</span>,
    },
    { title: '模块', dataIndex: 'module', width: 100 },
    { title: '动作', dataIndex: 'action', width: 100 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <Tag color={statusColor(v)}>{v}</Tag>,
    },
    {
      title: '耗时',
      dataIndex: 'latency',
      width: 90,
      render: (v?: number) =>
        typeof v === 'number' ? (
          <span
            className="cell-mono"
            style={{ color: v > 1000 ? '#f87171' : v > 300 ? '#fbbf24' : 'rgba(148, 163, 184, 0.85)' }}
          >
            {v}ms
          </span>
        ) : (
          '-'
        ),
    },
    { title: '时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => openDetail(record.id)}>
          详情
        </Button>
      ),
    },
  ]

  const topModules = Object.entries(stats?.by_module ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)

  return (
    <div>
      {stats && (
        <Card style={{ marginBottom: 16 }} styles={{ body: { padding: '14px 24px' } }}>
          <div className="log-stats-row">
            <div className="log-stat">
              <span className="log-stat-label">近 7 天操作</span>
              <span className="log-stat-value">{stats.total.toLocaleString()}</span>
            </div>
            <div className="log-stat">
              <span className="log-stat-label">异常请求</span>
              <span className="log-stat-value" style={{ color: stats.error_count > 0 ? '#f87171' : '#34d399' }}>
                {stats.error_count.toLocaleString()}
              </span>
            </div>
            {Object.keys(stats.by_method ?? {}).length > 0 && (
              <>
                <div className="log-stat-divider" />
                <div className="log-stat">
                  <span className="log-stat-label">方法分布</span>
                  <span>
                    {Object.entries(stats.by_method ?? {}).map(([m, n]) => (
                      <Tag key={m} color={methodColors[m] ?? 'default'} variant="filled">
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
                  <span>
                    {topModules.map(([m, n]) => (
                      <Tag key={m}>{m} {n}</Tag>
                    ))}
                  </span>
                </div>
              </>
            )}
          </div>
        </Card>
      )}

      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch} initialValues={params}>
          <Form.Item name="username">
            <Input placeholder="用户名" prefix={<SearchOutlined />} allowClear />
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
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card>
        <TableToolbar
          title="操作日志"
          total={total}
          extra={
            <>
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
            </>
          }
        />
        <Table
          rowKey="id"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Drawer
        title="请求诊断"
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width={720}
        destroyOnHidden
      >
        {detail && (
          <div>
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="用户">{detail.username || '-'}</Descriptions.Item>
              <Descriptions.Item label="时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              <Descriptions.Item label="方法 / 状态">
                <Tag color={methodColors[detail.method] ?? 'default'} variant="filled">{detail.method}</Tag>
                <Tag color={statusColor(detail.status)}>{detail.status}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="耗时">
                {typeof detail.latency === 'number' ? (
                  <span
                    className="cell-mono"
                    style={{ color: detail.latency > 1000 ? '#f87171' : detail.latency > 300 ? '#fbbf24' : undefined }}
                  >
                    {detail.latency}ms
                  </span>
                ) : (
                  '-'
                )}
              </Descriptions.Item>
              <Descriptions.Item label="路径" span={2}>
                <span className="cell-mono">
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
                <span className="cell-mono">{detail.request_id || '-'}</span>
              </Descriptions.Item>
              {detail.user_agent && (
                <Descriptions.Item label="User-Agent" span={2}>
                  <span style={{ fontSize: 12, color: 'rgba(148, 163, 184, 0.8)' }}>{detail.user_agent}</span>
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
