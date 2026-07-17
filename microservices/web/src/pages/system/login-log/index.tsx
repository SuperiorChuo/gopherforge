import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, DatePicker, Modal, InputNumber, Tooltip, Segmented, Skeleton,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, ClearOutlined, RobotOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { LoginLog } from '@/types'
import { getLoginLogList, clearLoginLogs, getLoginStats, type LoginLogStats } from '@/api/system/log'
import { getLogsInsight } from '@/api/ai'
import TableToolbar from '@/components/TableToolbar'
import CountUpValue from '@/components/CountUpValue'
import StatusPill from '@/components/StatusPill'
import AiMarkdown from '@/components/AiMarkdown'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import dayjs from 'dayjs'

const { RangePicker } = DatePicker

interface SearchParams {
  username?: string
  ip?: string
  status?: number
  start_time?: string
  end_time?: string
  page: number
  page_size: number
}

const loginTypeLabels: Record<number, string> = { 1: '密码登录', 2: 'GitHub', 3: '微信', 4: 'TOTP' }
const loginTypeColors: Record<number, string> = { 1: 'geekblue', 2: 'purple', 3: 'green', 4: 'cyan' }

export default function LoginLogPage() {
  const [list, setList] = useState<LoginLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [stats, setStats] = useState<LoginLogStats | null>(null)
  // AI 洞察：报告缓存到弹窗关闭，切换天数重新生成
  const [insightOpen, setInsightOpen] = useState(false)
  const [insightDays, setInsightDays] = useState(7)
  const [insightLoading, setInsightLoading] = useState(false)
  const [insightReport, setInsightReport] = useState('')
  const [searchForm] = Form.useForm()
  const [clearForm] = Form.useForm()
  const { hasPerm } = usePermission()

  useEffect(() => {
    getLoginStats().then(setStats).catch(() => setStats(null))
  }, [])

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getLoginLogList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取登录日志失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: {
    username?: string
    ip?: string
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

  const handleClear = async () => {
    const values = await clearForm.validateFields().catch(() => null)
    if (!values) return
    setClearing(true)
    try {
      const res = await clearLoginLogs(values.days)
      message.success(`清理成功，共删除 ${res.deleted_count} 条日志`)
      setClearOpen(false)
      fetchList({ ...params, page: 1 })
    } catch {
      message.error('清理失败')
    } finally {
      setClearing(false)
    }
  }

  const runInsight = async (days: number) => {
    setInsightLoading(true)
    setInsightReport('')
    try {
      const res = await getLogsInsight({ days })
      setInsightReport(res.report)
    } catch {
      // 拦截器已提示，弹窗保留以便重试
    } finally {
      setInsightLoading(false)
    }
  }

  const openInsight = () => {
    setInsightOpen(true)
    runInsight(insightDays)
  }

  const columns: ColumnsType<LoginLog> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    {
      title: 'IP / 位置',
      dataIndex: 'ip',
      width: 200,
      render: (v: string, record) => {
        const text = [v, record.location].filter(Boolean).join(' · ')
        return text ? <span className="cell-mono">{text}</span> : <span className="cell-muted">—</span>
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number, record) =>
        v === 1 ? (
          <StatusPill tone="success" label="成功" pulse={false} />
        ) : (
          <Tooltip title={record.message || undefined}>
            <span style={{ cursor: record.message ? 'help' : undefined }}>
              <StatusPill tone="danger" label="失败" />
            </span>
          </Tooltip>
        ),
    },
    {
      title: '登录类型',
      dataIndex: 'login_type',
      width: 100,
      render: (v: number) => (
        <Tag color={loginTypeColors[v]} variant="filled">{loginTypeLabels[v] ?? v}</Tag>
      ),
    },
    { title: '浏览器', dataIndex: 'browser', ellipsis: true },
    { title: 'OS', dataIndex: 'os', width: 120 },
    { title: '时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
  ]

  return (
    <div className="page-list login-log-page">
      {stats && (
        <Card className="list-filter-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
          <div className="log-stats-row">
            <div className="log-stat">
              <span className="log-stat-label">近 7 天登录</span>
              <span className="log-stat-value"><CountUpValue value={stats.total} /></span>
            </div>
            <div className="log-stat-divider" />
            <div className="log-stat">
              <span className="log-stat-label">成功</span>
              <span className="log-stat-value log-stat-success"><CountUpValue value={stats.success} /></span>
            </div>
            <div className="log-stat">
              <span className="log-stat-label">失败</span>
              <span className={`log-stat-value ${stats.failed > 0 ? 'log-stat-danger' : ''}`}>
                <CountUpValue value={stats.failed} />
              </span>
            </div>
            <div className="log-stat-divider" />
            <div className="log-stat">
              <span className="log-stat-label">今日活跃用户</span>
              <span className="log-stat-value log-stat-accent"><CountUpValue value={stats.today_users} /></span>
            </div>
          </div>
        </Card>
      )}

      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="username">
            <Input placeholder="搜索用户名" prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="ip">
            <Input placeholder="IP" allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>成功</Select.Option>
              <Select.Option value={0}>失败</Select.Option>
            </Select>
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
          title="登录日志"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<RobotOutlined />} onClick={openInsight}>
                AI 分析
              </Button>
              {hasPerm('system:log:login') && (
                <Button
                  danger
                  icon={<ClearOutlined />}
                  onClick={() => { clearForm.resetFields(); setClearOpen(true) }}
                >
                  清理日志
                </Button>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无登录记录" compact /> }}
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

      <Modal
        title="清理登录日志"
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

      <Modal
        title={<span><RobotOutlined style={{ marginRight: 8 }} />登录日志 AI 洞察</span>}
        open={insightOpen}
        onCancel={() => setInsightOpen(false)}
        footer={null}
        width={720}
        destroyOnHidden
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '8px 0 16px' }}>
          <span style={{ fontSize: 13, opacity: 0.75 }}>分析范围</span>
          <Segmented
            options={[
              { label: '近 7 天', value: 7 },
              { label: '近 14 天', value: 14 },
              { label: '近 30 天', value: 30 },
            ]}
            value={insightDays}
            disabled={insightLoading}
            onChange={(v) => {
              const days = v as number
              setInsightDays(days)
              runInsight(days)
            }}
          />
        </div>
        {insightLoading ? (
          <Skeleton active paragraph={{ rows: 8 }} title={false} />
        ) : insightReport ? (
          <div style={{ maxHeight: '60vh', overflowY: 'auto' }}>
            <AiMarkdown content={insightReport} />
          </div>
        ) : (
          <div style={{ padding: '24px 0', textAlign: 'center', opacity: 0.6 }}>
            生成失败，可切换天数重试
          </div>
        )}
      </Modal>
    </div>
  )
}
