import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, DatePicker, Modal, InputNumber, Tooltip, Segmented, Skeleton,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, ClearOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { LoginLog } from '@/types'
import { getLoginLogList, clearLoginLogs, getLoginStats, getLoginGeoDistribution, type LoginLogStats, type LoginGeoItem } from '@/api/system/log'
import GeoMap, { type GeoMapPoint } from '@/components/GeoMap'
import { resolveGeoPoint, resolveProvinceShort } from '@/utils/chinaGeo'
import TableToolbar from '@/components/TableToolbar'
import CountUpValue from '@/components/CountUpValue'
import StatusPill from '@/components/StatusPill'
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
  const [geoDays, setGeoDays] = useState(7)
  const [geoData, setGeoData] = useState<LoginGeoItem[] | null>(null)
  const [geoLoading, setGeoLoading] = useState(true)
  // 刷新失败保留上一窗口数据，避免整卡连切换器一起消失、用户失去重试入口
  const geoFailedRef = useRef(false)
  const [searchForm] = Form.useForm()
  const [clearForm] = Form.useForm()
  const { hasPerm } = usePermission()

  useEffect(() => {
    getLoginStats().then(setStats).catch(() => setStats(null))
  }, [])

  useEffect(() => {
    let cancelled = false
    setGeoLoading(true)
    getLoginGeoDistribution(geoDays)
      .then((data) => {
        if (cancelled) return
        setGeoData(data)
        geoFailedRef.current = false
      })
      .catch(() => {
        if (cancelled) return
        // 保留旧数据；从未拿到过数据时记失败态供空态文案区分
        geoFailedRef.current = true
      })
      .finally(() => {
        if (!cancelled) setGeoLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [geoDays])

  // 落点合并：同坐标累加（同市不同 ISP、省级兜底锚点与省会同点）；
  // 无法定位的（内网/海外/未知）进榜单不落图
  const geoView = useMemo(() => {
    if (!geoData || geoData.length === 0) return null
    const pointMap = new Map<string, GeoMapPoint>()
    const provinceTotals: Record<string, number> = {}
    const unlocated = new Map<string, { name: string; total: number; failed: number }>()
    for (const item of geoData) {
      const point = resolveGeoPoint(item.province, item.city)
      if (point) {
        const key = `${point.lng},${point.lat}`
        const prev = pointMap.get(key)
        if (prev) {
          prev.total += item.total
          prev.failed += item.failed
        } else {
          pointMap.set(key, { ...point, total: item.total, failed: item.failed })
        }
        if (!point.abroad) {
          const short = resolveProvinceShort(item.province)
          provinceTotals[short] = (provinceTotals[short] ?? 0) + item.total
        }
      } else {
        // 内网/未知按解析后标签合并（新旧记录的原文不同：「内网」/「Private Network」）
        const name = item.province === '内网' || item.province === '未知' ? item.province : item.location || '未知'
        const prev = unlocated.get(name)
        if (prev) {
          prev.total += item.total
          prev.failed += item.failed
        } else {
          unlocated.set(name, { name, total: item.total, failed: item.failed })
        }
      }
    }
    const points = [...pointMap.values()]
    const ranking = [
      ...points.map((p) => ({ name: p.name, total: p.total, failed: p.failed, located: true })),
      ...[...unlocated.values()].map((o) => ({ ...o, located: false })),
    ]
      .sort((a, b) => b.total - a.total)
      .slice(0, 12)
    return { points, ranking, max: ranking[0]?.total ?? 0, provinceTotals }
  }, [geoData])

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

      {hasPerm('system:log:login') && (geoData !== null || geoLoading) && (
        <Card className="list-filter-card" bordered={false}>
          <div className="geo-dist-header">
            <span className="geo-dist-title">登录地域分布</span>
            <Segmented
              size="small"
              options={[
                { label: '近 7 天', value: 7 },
                { label: '近 14 天', value: 14 },
                { label: '近 30 天', value: 30 },
              ]}
              value={geoDays}
              onChange={(v) => setGeoDays(v as number)}
            />
          </div>
          {geoView ? (
            <div className="geo-dist-body">
              <div className="geo-dist-map">
                <GeoMap points={geoView.points} provinceTotals={geoView.provinceTotals} height={400} />
              </div>
              <div className="geo-dist-side">
                {geoView.ranking.map((r) => (
                  <div key={r.name} className="geo-rank-item">
                    <div className="geo-rank-meta">
                      <span className={`geo-rank-name${r.located ? '' : ' geo-rank-name-muted'}`}>{r.name}</span>
                      <span className="geo-rank-count">
                        {r.total}
                        {r.failed > 0 && <em>失败 {r.failed}</em>}
                      </span>
                    </div>
                    <div className="geo-rank-bar">
                      <i
                        className={r.failed / r.total > 0.5 ? 'geo-rank-bar-alarm' : ''}
                        style={{ '--ratio': geoView.max > 0 ? r.total / geoView.max : 0 } as React.CSSProperties}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : geoLoading ? (
            <Skeleton active paragraph={{ rows: 6 }} title={false} style={{ padding: '24px 0' }} />
          ) : (
            <GlassEmpty compact text={geoFailedRef.current ? '分布数据加载失败，可切换天数重试' : '该时间段暂无登录记录'} />
          )}
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
    </div>
  )
}
