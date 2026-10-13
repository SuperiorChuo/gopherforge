import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, DatePicker, Modal, InputNumber, Tooltip, Segmented, Skeleton,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, ClearOutlined, RobotOutlined, DownloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { LoginLog } from '@/types'
import { getLoginLogList, clearLoginLogs, getLoginStats, getLoginGeoDistribution, exportLoginLogs, type LoginLogStats, type LoginGeoItem } from '@/api/system/log'
import { getLogsInsight } from '@/api/ai'
import GeoMap, { type GeoMapPoint } from '@/components/common/GeoMap'
import { resolveGeoPoint, resolveProvinceShort } from '@/utils/chinaGeo'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import TableToolbar from '@/components/common/TableToolbar'
import CountUpValue from '@/components/common/CountUpValue'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid, { StatsGridDivider } from '@/components/common/StatsGrid'
import StatusPill from '@/components/common/StatusPill'
import AiMarkdown from '@/components/common/AiMarkdown'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import dayjs from 'dayjs'
import './styles.css'

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
  const { t } = useTranslation()
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [stats, setStats] = useState<LoginLogStats | null>(null)
  const [geoDays, setGeoDays] = useState(7)
  const [geoData, setGeoData] = useState<LoginGeoItem[] | null>(null)
  const [geoLoading, setGeoLoading] = useState(true)
  // 刷新失败保留上一窗口数据，避免整卡连切换器一起消失、用户失去重试入口
  const [geoFailed, setGeoFailed] = useState(false)
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

  useEffect(() => {
    let cancelled = false
    setGeoLoading(true)
    getLoginGeoDistribution(geoDays)
      .then((data) => {
        if (cancelled) return
        setGeoData(data)
        setGeoFailed(false)
      })
      .catch(() => {
        if (cancelled) return
        // 保留旧数据；从未拿到过数据时记失败态供空态文案区分
        setGeoFailed(true)
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

  const fetchList = useCallback(async (p: SearchParams) => {
    const res = await getLoginLogList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => message.error(t('获取登录日志失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onLoadError,
  })

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

  // 导出走当前筛选条件（不带分页），与页面所见一致；后端封顶 1 万行
  const handleExport = async () => {
    setExporting(true)
    try {
      const { page: _page, page_size: _pageSize, ...filters } = params
      await exportLoginLogs(filters)
      message.success(t('导出已开始，请查看浏览器下载'))
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
      const res = await clearLoginLogs(values.days)
      message.success(t('清理成功，共删除 {{n}} 条日志', { n: res.deleted_count }))
      setClearOpen(false)
      setParams({ ...params, page: 1 })
    } catch {
      message.error(t('清理失败'))
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
    { title: 'ID', dataIndex: 'id', width: 64, responsive: ['lg'] },
    {
      title: t('用户名'),
      dataIndex: 'username',
      width: 132,
      render: (v: string) => v ? (
        <Tooltip title={v}>
          <span className="login-log-ellipsis login-log-username">{v}</span>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('IP / 位置'),
      dataIndex: 'ip',
      width: 220,
      responsive: ['sm'],
      render: (v: string, record) => {
        const text = [v, record.location].filter(Boolean).join(' · ')
        return text ? (
          <Tooltip title={text}>
            <span className="cell-mono login-log-ellipsis">{text}</span>
          </Tooltip>
        ) : <span className="cell-muted">—</span>
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 84,
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
      title: t('失败原因'),
      dataIndex: 'message',
      width: 220,
      ellipsis: true,
      responsive: ['md'],
      render: (v: string | undefined, record) => record.status !== 1 && v ? (
        <Tooltip title={v}>
          <span className="login-log-ellipsis login-log-reason">{v}</span>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('登录类型'),
      dataIndex: 'login_type',
      width: 112,
      responsive: ['md'],
      render: (v: number) => (
        <Tag color={loginTypeColors[v]} variant="filled">{t(loginTypeLabels[v] ?? '')}</Tag>
      ),
    },
    { title: '浏览器', dataIndex: 'browser', width: 160, ellipsis: true, responsive: ['lg'] },
    { title: 'OS', dataIndex: 'os', width: 140, ellipsis: true, responsive: ['lg'] },
    {
      title: t('设备指纹'),
      dataIndex: 'device_id',
      width: 130,
      responsive: ['xl'],
      render: (v?: string) => v ? (
        <Tooltip title={v}>
          <span className="cell-mono login-log-ellipsis">{v.slice(0, 8)}…</span>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      width: 164,
      className: 'cell-time',
      render: (v?: string) => <span className="login-log-time">{formatDateTime(v)}</span>,
    },
  ]

  return (
    <div className="page-list login-log-page">
      {stats && (
        <Card className="list-filter-card login-log-stats-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
          <StatsGrid className="login-log-stats-grid">
            <MetricCard label={t('近 7 天登录')} value={<CountUpValue value={stats.total} />} />
            <StatsGridDivider />
            <MetricCard
              label={t('成功')}
              value={<CountUpValue value={stats.success} />}
              valueClassName="log-stat-success"
            />
            <MetricCard
              label={t('失败')}
              value={<CountUpValue value={stats.failed} />}
              valueClassName={stats.failed > 0 ? 'log-stat-danger' : undefined}
            />
            <StatsGridDivider />
            <MetricCard
              label={t('今日活跃用户')}
              value={<CountUpValue value={stats.today_users} />}
              valueClassName="log-stat-accent"
            />
          </StatsGrid>
        </Card>
      )}

      {hasPerm('system:log:login') && (geoData !== null || geoLoading) && (
        <Card className="list-filter-card" bordered={false}>
          <div className="geo-dist-header">
            <span className="geo-dist-title">{t('登录地域分布')}</span>
            <Segmented
              size="small"
              options={[
                { label: t('近 7 天'), value: 7 },
                { label: t('近 14 天'), value: 14 },
                { label: t('近 30 天'), value: 30 },
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
                        {r.failed > 0 && <em>{t('失败')} {r.failed}</em>}
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
            <GlassEmpty compact text={geoFailed ? '分布数据加载失败，可切换天数重试' : '该时间段暂无登录记录'} />
          )}
        </Card>
      )}

      <Card className="list-filter-card" bordered={false}>
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
          <Form.Item name="ip">
            <Input placeholder="IP" allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
              <Select.Option value={1}>{t('成功')}</Select.Option>
              <Select.Option value={0}>{t('失败')}</Select.Option>
            </Select>
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
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="登录日志"
          total={total}
          extra={
            <Space wrap className="login-log-toolbar-actions">
              <Tooltip title={t('AI 分析')}>
                <Button aria-label={t('AI 分析登录日志')} icon={<RobotOutlined />} onClick={openInsight}>
                  {t('AI 分析')}
                </Button>
              </Tooltip>
              {hasPerm('system:log:login') && (
                <Tooltip title={t('导出 CSV')}>
                  <Button aria-label={t('导出登录日志 CSV')} icon={<DownloadOutlined />} loading={exporting} onClick={handleExport}>
                    {t('导出 CSV')}
                  </Button>
                </Tooltip>
              )}
              {hasPerm('system:log:login') && (
                <Tooltip title={t('清理日志')}>
                  <Button
                    danger
                    aria-label={t('清理登录日志')}
                    icon={<ClearOutlined />}
                    onClick={() => { clearForm.resetFields(); setClearOpen(true) }}
                  >
                    {t('清理日志')}
                  </Button>
                </Tooltip>
              )}
              <Tooltip title={t('刷新')}>
                <Button aria-label={t('刷新登录日志')} icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
              </Tooltip>
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table login-log-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无登录记录" compact /> }}
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
      </Card>

      <EntityFormModal
        title={t('清理登录日志')}
        open={clearOpen}
        form={clearForm}
        onSubmit={handleClear}
        onClose={() => setClearOpen(false)}
        submitting={clearing}
        okButtonProps={{ danger: true }}
        okText={t('确认清理')}
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

      <Modal
        title={<span><RobotOutlined style={{ marginRight: 8 }} />{t('登录日志 AI 洞察')}</span>}
        open={insightOpen}
        onCancel={() => setInsightOpen(false)}
        footer={null}
        width={720}
        destroyOnHidden
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '8px 0 16px' }}>
          <span style={{ fontSize: 13, opacity: 0.75 }}>{t('分析范围')}</span>
          <Segmented
            options={[
              { label: t('近 7 天'), value: 7 },
              { label: t('近 14 天'), value: 14 },
              { label: t('近 30 天'), value: 30 },
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
            {t('生成失败，可切换天数重试')}
          </div>
        )}
      </Modal>
    </div>
  )
}
