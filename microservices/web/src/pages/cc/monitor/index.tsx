import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Card, Row, Col, Statistic, Input, Space, Tag, Table, Button, Dropdown, Badge, Switch, Alert, Spin,
  Grid,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  RadarChartOutlined, PhoneOutlined, TeamOutlined, ClockCircleOutlined,
  EyeOutlined, AudioOutlined, ApiOutlined, DisconnectOutlined, VideoCameraOutlined,
  DownOutlined, UserOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import dayjs from 'dayjs'
import {
  getCcMonitor, superviseCall, uuidRecord, fsHangup, setCcAgentStatus,
  type CcMonitor, type CcMonitorMember, type CcMonitorAgent, type CcMonitorChannel,
} from '@/api/cc'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { subscribeCcEvents } from '@/utils/ccSocket'

const SUP_EXT_KEY = 'cc_supervisor_ext'

// mm:ss，用于等待时长与通话时长
function fmtDuration(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) sec = 0
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

// FS `created` 与当前时刻的差（秒）；解析失败按 0 容错
function channelDurationSec(created: string): number {
  if (!created) return 0
  const t = dayjs(created)
  if (!t.isValid()) return 0
  const d = dayjs().diff(t, 'second')
  return d > 0 ? d : 0
}

// 坐席状态 → 颜色 + 文案（文案为词典 key，渲染处 t() 翻译）
function agentStatusMeta(a: CcMonitorAgent): { color: string; text: string } {
  // 通话中优先于签入态展示
  if (a.state === 'In a queue call' || a.state === 'Receiving') {
    return { color: 'blue', text: '通话中' }
  }
  switch (a.status) {
    case 'Available':
      return { color: 'green', text: '已签入' }
    case 'On Break':
      return { color: 'orange', text: '示忙' }
    case 'Logged Out':
      return { color: 'default', text: '已签出' }
    default:
      return { color: 'default', text: a.status || '未知' }
  }
}

const CALLSTATE_META: Record<string, { label: string; color: string }> = {
  RINGING: { label: '振铃中', color: 'processing' },
  EARLY: { label: '早期媒体', color: 'processing' },
  ACTIVE: { label: '通话中', color: 'success' },
  HELD: { label: '保持中', color: 'warning' },
  DOWN: { label: '结束中', color: 'default' },
}

function callstateTag(v: string, t: TFunction) {
  const meta = CALLSTATE_META[v]
  if (meta) return <Tag color={meta.color} variant="filled">{t(meta.label)}</Tag>
  return <Tag variant="filled">{v || '—'}</Tag>
}

export default function CcMonitorPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [data, setData] = useState<CcMonitor | null>(null)
  const [supExt, setSupExt] = useState<string>(() => localStorage.getItem(SUP_EXT_KEY) ?? '')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [monitorUnavailable, setMonitorUnavailable] = useState(false)
  // 首次加载 / 手动重试显示 loading；轮询与事件刷新走 silent 不闪烁
  const [loading, setLoading] = useState(true)
  // 手动录音中的通道
  const [recording, setRecording] = useState<Record<string, boolean>>({})
  // 轮询期间静默拉取，避免闪烁
  const silentRef = useRef(false)

  const fetchMonitor = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      const res = await getCcMonitor()
      setData(res)
      setMonitorUnavailable(false)
    } catch {
      setMonitorUnavailable(true)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchMonitor()
  }, [fetchMonitor])

  // 10s 轮询兜底（事件驱动为主）；后台标签页暂停，回前台立即补一次。
  // autoRefresh 走 ref，开关不重建计时器；latest-ref 写入放 effect，
  // 读取点全在轮询/订阅回调里，提交后即生效。
  const autoRefreshRef = useRef(autoRefresh)
  useEffect(() => {
    autoRefreshRef.current = autoRefresh
  })
  useVisibilityInterval(() => {
    if (!autoRefreshRef.current || silentRef.current) return
    silentRef.current = true
    void fetchMonitor().finally(() => { silentRef.current = false })
  }, 10000, false)

  // 事件驱动为主：收到实时话务事件立即静默刷新一次（轮询 10s 兜底）。
  useEffect(() => {
    if (!autoRefresh) return
    const unsub = subscribeCcEvents(() => {
      if (silentRef.current) return
      silentRef.current = true
      fetchMonitor(true).finally(() => { silentRef.current = false })
    })
    return unsub
  }, [autoRefresh, fetchMonitor])

  const onSupExtChange = (v: string) => {
    setSupExt(v)
    localStorage.setItem(SUP_EXT_KEY, v)
  }

  const queues = data?.queues ?? []
  const agents = data?.agents ?? []
  const channels = data?.channels ?? []
  const queuedTotal = queues.reduce((n, q) => n + q.members.length, 0)
  const onlineAgents = agents.filter((a) => a.status === 'Available' || a.state === 'In a queue call' || a.state === 'Receiving').length

  // ---- 一键监听/耳语/强插 ----
  const requireSupExt = (): boolean => {
    if (!/^[0-9*#+]{2,32}$/.test(supExt.trim())) {
      message.warning(t('请先在顶部填写班长分机号'))
      return false
    }
    return true
  }

  const handleSupervise = async (uuid: string, mode: 'listen' | 'whisper' | 'barge') => {
    if (!requireSupExt()) return
    const label = { listen: t('监听'), whisper: t('耳语'), barge: t('强插') }[mode]
    try {
      await superviseCall(supExt.trim(), uuid, mode)
      message.success(t('已发起{{label}}：分机 {{ext}} 将振铃，接起后进入（话机 0静默/1耳语/2对客/3强插 切换）', { label, ext: supExt }))
    } catch (e) {
      message.error((e as Error)?.message || t('发起{{label}}失败', { label }))
    }
  }

  const handleRecordToggle = async (uuid: string) => {
    const active = !!recording[uuid]
    try {
      await uuidRecord(uuid, active ? 'stop' : 'start')
      setRecording((prev) => ({ ...prev, [uuid]: !active }))
      message.success(active ? t('已停止录音') : t('已开始录音（文件带 manual- 前缀）'))
    } catch (e) {
      message.error((e as Error)?.message || t('录音操作失败'))
    }
  }

  const handleHangup = async (uuid: string) => {
    try {
      await fsHangup(uuid)
      message.success(t('已强拆通道'))
      fetchMonitor(true)
    } catch (e) {
      message.error((e as Error)?.message || t('强拆失败'))
    }
  }

  const handleAgentStatus = async (ext: string, status: 'available' | 'on-break' | 'logged-out') => {
    const label = { available: t('签入'), 'on-break': t('示忙'), 'logged-out': t('签出') }[status]
    try {
      await setCcAgentStatus(ext, status)
      message.success(t('坐席 {{ext}} 已{{label}}', { ext, label }))
      fetchMonitor(true)
    } catch (e) {
      message.error((e as Error)?.message || t('{{label}}失败', { label }))
    }
  }

  const agentMenu = (ext: string): MenuProps => ({
    items: [
      { key: 'available', label: t('签入（Available）') },
      { key: 'on-break', label: t('示忙（On Break）') },
      { key: 'logged-out', label: t('签出（Logged Out）') },
    ],
    onClick: ({ key }) => handleAgentStatus(ext, key as 'available' | 'on-break' | 'logged-out'),
  })

  // 排队明细小表列
  const memberColumns: ColumnsType<CcMonitorMember> = [
    {
      title: t('号码'),
      dataIndex: 'cid_number',
      render: (v: string) => <span className="cell-mono">{v || t('匿名')}</span>,
    },
    {
      title: t('等待'),
      dataIndex: 'wait_sec',
      width: 80,
      render: (v: number) => (
        <span style={{ color: v > 60 ? 'var(--c-error)' : undefined, fontVariantNumeric: 'tabular-nums' }}>
          {fmtDuration(v)}
        </span>
      ),
    },
    { title: t('位置'), dataIndex: 'position', width: 60, align: 'center' },
  ]

  // 通话区列
  const channelColumns: ColumnsType<CcMonitorChannel> = [
    {
      title: t('主叫'),
      dataIndex: 'cid_num',
      width: 130,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('被叫'),
      dataIndex: 'dest',
      width: 130,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    { title: t('状态'), dataIndex: 'callstate', width: 110, render: (v: string) => callstateTag(v, t) },
    {
      title: t('时长'),
      dataIndex: 'created',
      width: 90,
      render: (v: string) => (
        <span style={{ fontVariantNumeric: 'tabular-nums' }}>{fmtDuration(channelDurationSec(v))}</span>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 168,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: row.uuid.slice(0, 8) })}
          actions={[
            {
              key: 'listen',
              label: t('监听'),
              icon: <EyeOutlined />,
              onClick: () => handleSupervise(row.uuid, 'listen'),
            },
            {
              key: 'whisper',
              label: t('耳语'),
              icon: <AudioOutlined />,
              onClick: () => handleSupervise(row.uuid, 'whisper'),
            },
            {
              key: 'barge',
              label: t('强插'),
              icon: <ApiOutlined />,
              onClick: () => handleSupervise(row.uuid, 'barge'),
            },
            {
              key: 'record',
              label: recording[row.uuid] ? t('停录') : t('随录'),
              icon: <VideoCameraOutlined />,
              danger: !!recording[row.uuid],
              onClick: () => handleRecordToggle(row.uuid),
            },
            {
              key: 'hangup',
              label: t('强拆'),
              icon: <DisconnectOutlined />,
              danger: true,
              confirm: {
                title: t('确认强拆该通道？'),
                description: t('通话将被立即挂断（MANAGER_REQUEST）'),
              },
              onClick: () => { void handleHangup(row.uuid) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <div className="page-list cc-monitor-page">
      {monitorUnavailable && (
        <Alert
          type="warning"
          showIcon
          message={t('话务监控服务暂不可用')}
          description={t('请检查 FreeSWITCH 控制接口配置或稍后重试。已有监控数据将继续保留。')}
          action={<Button size="small" onClick={() => void fetchMonitor()}>{t('重试')}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {loading && !data ? (
        <div style={{ padding: 64, textAlign: 'center' }}><Spin size="large" /></div>
      ) : (
        <>
      {/* 顶部班长条 + 概览 */}
      <Card className="list-filter-card" bordered={false}>
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} md={8}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <span className="cell-dim" style={{ fontSize: 13 }}>{t('班长分机（监听/耳语/强插用）')}</span>
              <Input
                placeholder={t('班长分机号（话机或网页坐席，如 1000）')}
                prefix={<UserOutlined />}
                value={supExt}
                onChange={(e) => onSupExtChange(e.target.value)}
                allowClear
              />
            </Space>
          </Col>
          <Col xs={8} md={5}>
            <Statistic title={t('进行中通话')} value={channels.length} prefix={<PhoneOutlined />} />
          </Col>
          <Col xs={8} md={5}>
            <Statistic title={t('排队总数')} value={queuedTotal} prefix={<ClockCircleOutlined />}
              valueStyle={queuedTotal > 0 ? { color: 'var(--c-orange)' } : undefined} />
          </Col>
          <Col xs={8} md={4}>
            <Statistic title={t('在线坐席')} value={onlineAgents} prefix={<TeamOutlined />} />
          </Col>
          <Col xs={24} md={2} style={{ textAlign: 'right' }}>
            <Space size={6}>
              <span className="cell-dim" style={{ fontSize: 12 }}>{t('自动')}</span>
              <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 队列区 */}
      <Card className="list-main-card" bordered={false} style={{ marginTop: 16 }}>
        <TableToolbar title="队列盯盘" total={queues.length} />
        {queues.length === 0 ? (
          <GlassEmpty text="暂无队列" compact />
        ) : (
          <Row gutter={[16, 16]}>
            {queues.map((q) => {
              const longest = q.longest_wait_sec
              return (
                <Col key={q.name} xs={24} sm={12} lg={8} xxl={6}>
                  <Card size="small" bordered className="cc-monitor-queue-card">
                    <Space style={{ justifyContent: 'space-between', width: '100%' }} align="start">
                      <Space direction="vertical" size={0}>
                        <strong>{q.name}</strong>
                        <span className="cell-dim" style={{ fontSize: 12 }}>{q.strategy || t('默认')}</span>
                      </Space>
                      <Statistic
                        value={q.members.length}
                        valueStyle={{
                          fontSize: 32,
                          color: q.members.length > 0 ? 'var(--c-orange)' : undefined,
                          lineHeight: 1,
                        }}
                      />
                    </Space>
                    <Space size={16} style={{ marginTop: 8 }} wrap>
                      <span className="cell-dim" style={{ fontSize: 12 }}>
                        {t('最长等待')}{' '}
                        <span style={{ color: longest > 60 ? 'var(--c-error)' : undefined, fontVariantNumeric: 'tabular-nums' }}>
                          {fmtDuration(longest)}
                        </span>
                      </span>
                      <span className="cell-dim" style={{ fontSize: 12 }}>
                        {t('接通')} <Tag color="green">{q.calls_answered}</Tag>
                        {t('放弃')} <Tag color="volcano">{q.calls_abandoned}</Tag>
                      </span>
                    </Space>
                    <Table
                      style={{ marginTop: 8 }}
                      size="small"
                      rowKey={(m) => `${q.name}-${m.cid_number}-${m.position}`}
                      columns={memberColumns}
                      dataSource={q.members}
                      pagination={false}
                      scroll={{ x: 800 }}
                      locale={{ emptyText: <GlassEmpty text="无人排队" compact /> }}
                    />
                  </Card>
                </Col>
              )
            })}
          </Row>
        )}
      </Card>

      {/* 坐席区 */}
      <Card className="list-main-card" bordered={false} style={{ marginTop: 16 }}>
        <TableToolbar title="坐席网格" total={agents.length} />
        {agents.length === 0 ? (
          <GlassEmpty text="暂无坐席" compact />
        ) : (
          <Row gutter={[12, 12]}>
            {agents.map((a) => {
              const meta = agentStatusMeta(a)
              return (
                <Col key={a.ext} xs={12} sm={8} md={6} lg={4}>
                  <Dropdown menu={agentMenu(a.ext)} trigger={['click']}>
                    <Card size="small" hoverable className="cc-monitor-agent-card">
                      <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                        <Space size={6}>
                          <Badge color={meta.color} />
                          <strong className="cell-mono">{a.ext}</strong>
                        </Space>
                        <DownOutlined style={{ fontSize: 10, opacity: 0.45 }} />
                      </Space>
                      <div style={{ marginTop: 4 }}>
                        <Tag color={meta.color} variant="filled">{t(meta.text)}</Tag>
                      </div>
                    </Card>
                  </Dropdown>
                </Col>
              )
            })}
          </Row>
        )}
      </Card>

      {/* 通话区 */}
      <Card className="list-main-card" bordered={false} style={{ marginTop: 16 }}>
        <TableToolbar
          title="进行中通话"
          total={channels.length}
          extra={
            <span className="cell-dim" style={{ fontSize: 12 }}>
              <RadarChartOutlined /> {t('监听需班长分机在线（话机或网页坐席）')}
            </span>
          }
        />
        <Table
          rowKey="uuid"
          scroll={{ x: 800 }}
          className="list-table"
          columns={channelColumns}
          dataSource={channels}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="当前没有进行中的通话" compact /> }}
        />
      </Card>
        </>
      )}
    </div>
  )
}
