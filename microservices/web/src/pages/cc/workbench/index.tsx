import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Button, Card, Col, Empty, Input, List, Row, Space, Statistic, Tag, Tooltip, Typography,
} from 'antd'
import {
  PhoneOutlined, LoginOutlined, PauseCircleOutlined, LogoutOutlined, ReloadOutlined,
  UserOutlined, ContactsOutlined, LinkOutlined, DashboardOutlined,
} from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { message } from '@/utils/feedback'
import { useAppSelector } from '@/hooks/store'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import WebPhone from '../console/WebPhone'
import Customer360Panel from '@/components/customer360/Customer360Panel'
import GlassEmpty from '@/components/common/GlassEmpty'
import {
  listCcAgents, bindCcAgent, setCcAgentStatus, listCcQueues, fsOriginate, listFsChannels,
  lookupCcNumber, listCcCalls, getCcReportOverview,
  type CcAgent, type CcQueue, type CcLookup, type CcCall, type FsChannel,
} from '@/api/cc'

const { Text } = Typography

const AGENT_EXT_KEY = 'cc_agent_ext'

function statusTag(status: string | undefined, t: TFunction) {
  switch (status) {
    case 'Available': return <Tag color="success" variant="filled">{t('签入')}</Tag>
    case 'On Break': return <Tag color="warning" variant="filled">{t('示忙')}</Tag>
    case 'Logged Out': return <Tag variant="filled">{t('签出')}</Tag>
    default: return <Tag variant="filled">{status || t('未知')}</Tag>
  }
}

function dirTag(direction: string, t: TFunction) {
  const out = direction === 'outbound'
  return <Tag color={out ? 'blue' : 'green'} variant="filled">{out ? t('呼出') : t('呼入')}</Tag>
}

export default function CcWorkbenchPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const userId = userInfo?.id ?? 0

  const [agent, setAgent] = useState<CcAgent | null>(null)
  const [extInput, setExtInput] = useState(localStorage.getItem(AGENT_EXT_KEY) || '')
  const [binding, setBinding] = useState(false)
  const [queues, setQueues] = useState<CcQueue[]>([])
  const [statusBusy, setStatusBusy] = useState(false)

  const [dialNumber, setDialNumber] = useState('')
  const [lookupNumber, setLookupNumber] = useState('')
  const [lookup, setLookup] = useState<CcLookup | null>(null)
  const [lookupLoading, setLookupLoading] = useState(false)
  const [calling, setCalling] = useState(false)
  const [panelRefresh, setPanelRefresh] = useState(0)

  const [myAnswered, setMyAnswered] = useState<number | null>(null)
  const [myAvgSec, setMyAvgSec] = useState<number | null>(null)
  const [myCalls, setMyCalls] = useState<CcCall[]>([])

  const myExt = agent?.extension || ''
  const lastRingRef = useRef<string>('')

  const loadAgent = useCallback(async () => {
    try {
      const res = await listCcAgents()
      const mine = (res.list ?? []).find((a) => a.user_id === userId) || null
      setAgent(mine)
      if (mine) {
        setExtInput(mine.extension)
        localStorage.setItem(AGENT_EXT_KEY, mine.extension)
      }
    } catch {
      /* silent */
    }
  }, [userId])

  const loadQueues = useCallback(async () => {
    try {
      const res = await listCcQueues()
      setQueues(res.list ?? [])
    } catch {
      setQueues([])
    }
  }, [])

  useEffect(() => {
    void loadAgent()
    void loadQueues()
  }, [loadAgent, loadQueues])

  const loadMyStats = useCallback(async (ext: string) => {
    if (!ext) { setMyAnswered(null); setMyAvgSec(null); setMyCalls([]); return }
    try {
      const [ov, calls] = await Promise.all([
        getCcReportOverview({}),
        listCcCalls({ page: 1, page_size: 10, agent_ext: ext }),
      ])
      const mine = (ov.agent_leaderboard ?? []).find((a) => a.agent_ext === ext)
      setMyAnswered(mine?.answered ?? 0)
      setMyAvgSec(mine?.avg_talk_sec ?? 0)
      setMyCalls(calls.list ?? [])
    } catch {
      /* silent */
    }
  }, [])

  useEffect(() => {
    void loadMyStats(myExt)
  }, [myExt, loadMyStats])

  const runLookup = useCallback(async (number: string) => {
    const n = number.trim()
    if (!n) return
    setLookupNumber(n)
    setDialNumber(n)
    setLookupLoading(true)
    try {
      setLookup(await lookupCcNumber(n))
      setPanelRefresh((x) => x + 1)
      // 同步 URL，便于弹屏深链
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev)
        next.set('phone', n)
        return next
      }, { replace: true })
    } catch {
      message.error(t('识客失败'))
      setLookup(null)
    } finally {
      setLookupLoading(false)
    }
  }, [setSearchParams, t])

  // 来自弹屏 / 深链 ?phone=
  useEffect(() => {
    const p = (searchParams.get('phone') || '').trim()
    if (p && p !== lookupNumber) {
      void runLookup(p)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 来电振铃探测轮询；后台标签页暂停，回前台立即补拉
  useVisibilityInterval(() => {
    void (async () => {
      try {
        const res = await listFsChannels()
        const inbound = (res.list ?? []).find(
          (ch: FsChannel) => ch.direction === 'inbound'
            && (ch.callstate === 'RINGING' || ch.callstate === 'EARLY')
            && ch.cid_num,
        )
        if (inbound && inbound.cid_num !== lastRingRef.current) {
          lastRingRef.current = inbound.cid_num
          void runLookup(inbound.cid_num)
        }
      } catch {
        /* silent */
      }
    })()
  }, 5000)

  async function onBind() {
    const ext = extInput.trim()
    if (!/^[0-9A-Za-z._-]{2,32}$/.test(ext)) {
      message.error(t('分机号 2-32 位，字母数字与 ._-'))
      return
    }
    if (!userId) { message.error(t('未取到当前用户')); return }
    setBinding(true)
    try {
      const a = await bindCcAgent(userId, ext, userInfo?.nickname || userInfo?.username)
      setAgent(a)
      localStorage.setItem(AGENT_EXT_KEY, ext)
      message.success(t('已绑定分机 {{ext}}', { ext }))
      void loadQueues()
    } catch (e) {
      message.error((e as Error)?.message || t('绑定失败'))
    } finally {
      setBinding(false)
    }
  }

  async function onStatus(status: 'available' | 'on-break' | 'logged-out') {
    if (!myExt) { message.error(t('请先绑定分机')); return }
    setStatusBusy(true)
    try {
      await setCcAgentStatus(myExt, status)
      message.success(t('状态已更新'))
      setTimeout(() => void loadQueues(), 600)
    } catch (e) {
      message.error((e as Error)?.message || t('设置状态失败'))
    } finally {
      setStatusBusy(false)
    }
  }

  async function onDial() {
    const to = dialNumber.replace(/[^0-9*#+]/g, '')
    if (!to) { message.error(t('请输入被叫号码')); return }
    if (!myExt) { message.error(t('请先绑定分机')); return }
    setCalling(true)
    try {
      await fsOriginate(myExt, to)
      message.success(t('分机 {{ext}} 将振铃，接起后呼出 {{to}}', { ext: myExt, to }))
      void runLookup(to)
    } catch (e) {
      message.error((e as Error)?.message || t('外呼失败'))
    } finally {
      setCalling(false)
    }
  }

  const myQueueRows = useMemo(() => {
    if (!myExt) return []
    const rows: { queue: string; status: string; state: string }[] = []
    for (const q of queues) {
      const a = (q.agents ?? []).find((x) => x.ext === myExt)
      if (a) rows.push({ queue: q.name, status: a.status, state: a.state })
    }
    return rows
  }, [queues, myExt])

  const fmtSec = (s?: number | null) => {
    if (s == null) return '—'
    const m = Math.floor(s / 60), sec = s % 60
    return m > 0 ? t('{{m}}分{{s}}秒', { m, s: sec }) : t('{{s}}秒', { s: sec })
  }

  const focusPhone = lookupNumber || dialNumber.trim()

  return (
    <div className="page-list cc-workbench-page">
      <Row gutter={[16, 16]}>
        {/* 左栏：操作台（状态 + 拨号识客） */}
        <Col xs={24} lg={14}>
          <Card title={<Space><UserOutlined />{t('我的状态')}</Space>} bordered={false} styles={{ body: { paddingTop: 12 } }}>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>{t('坐席分机')}</Text>
                <Space.Compact style={{ width: '100%', marginTop: 4 }}>
                  <Input
                    value={extInput}
                    onChange={(e) => setExtInput(e.target.value)}
                    placeholder={t('如 1001')}
                    prefix={<PhoneOutlined />}
                  />
                  <Button type="primary" loading={binding} onClick={() => void onBind()}>
                    {agent ? t('换绑') : t('绑定')}
                  </Button>
                </Space.Compact>
                {agent
                  ? <Text type="success" style={{ fontSize: 12 }}>{t('已绑定：{{ext}}', { ext: agent.extension })}</Text>
                  : <Text type="warning" style={{ fontSize: 12 }}>{t('未绑定分机，签入前先绑定')}</Text>}
              </div>

              <div>
                <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>{t('队列签入')}</Text>
                  <Button type="text" size="small" icon={<ReloadOutlined />} onClick={() => void loadQueues()} />
                </Space>
                {myQueueRows.length === 0
                  ? <div style={{ marginTop: 4 }}><Text type="secondary" style={{ fontSize: 12 }}>{t('未加入队列或未绑定')}</Text></div>
                  : (
                    <List size="small" style={{ marginTop: 4 }} dataSource={myQueueRows}
                      renderItem={(row) => (
                        <List.Item style={{ padding: '4px 0' }}>
                          <Space size={6}><Tag>{row.queue}</Tag>{statusTag(row.status, t)}</Space>
                          <Text type="secondary" style={{ fontSize: 12 }}>{row.state}</Text>
                        </List.Item>
                      )} />
                  )}
              </div>

              <Space wrap>
                <Button size="small" type="primary" ghost icon={<LoginOutlined />} loading={statusBusy}
                  disabled={!myExt} onClick={() => void onStatus('available')}>{t('签入')}</Button>
                <Button size="small" icon={<PauseCircleOutlined />} loading={statusBusy}
                  disabled={!myExt} onClick={() => void onStatus('on-break')}>{t('示忙')}</Button>
                <Button size="small" danger icon={<LogoutOutlined />} loading={statusBusy}
                  disabled={!myExt} onClick={() => void onStatus('logged-out')}>{t('签出')}</Button>
              </Space>

              <div style={{ marginTop: 4 }}>
                <WebPhone />
              </div>

              <Card size="small" title={<Space size={4}><DashboardOutlined />{t('今日')}</Space>} styles={{ body: { paddingTop: 8 } }}>
                {!myExt
                  ? <Text type="secondary" style={{ fontSize: 12 }}>{t('绑定分机后展示')}</Text>
                  : (
                    <Row gutter={8}>
                      <Col span={12}>
                        <Statistic title={t('接通')} value={myAnswered ?? 0} valueStyle={{ fontSize: 18, color: 'var(--c-primary)' }} />
                      </Col>
                      <Col span={12}>
                        <Statistic title={t('均通')} value={fmtSec(myAvgSec)} valueStyle={{ fontSize: 16 }} />
                      </Col>
                    </Row>
                  )}
              </Card>
            </Space>
          </Card>

          <Card title={<Space><ContactsOutlined />{t('当前通话 · 识客')}</Space>} bordered={false} styles={{ body: { paddingTop: 12 } }} style={{ marginTop: 16 }}>
            <Space.Compact style={{ width: '100%' }}>
              <Input
                value={dialNumber}
                onChange={(e) => setDialNumber(e.target.value)}
                onPressEnter={() => void runLookup(dialNumber)}
                placeholder={t('输入号码：回车识客 / 点呼叫外呼')}
                prefix={<PhoneOutlined />}
                allowClear
              />
              <Button onClick={() => void runLookup(dialNumber)} loading={lookupLoading}>{t('识客')}</Button>
              <Button type="primary" icon={<PhoneOutlined />} loading={calling} onClick={() => void onDial()}>{t('呼叫')}</Button>
            </Space.Compact>

            <div style={{ marginTop: 16 }}>
              {!lookup && !lookupLoading && (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('输入号码或等待来电；右侧展示客户 360')} style={{ padding: 24 }} />
              )}
              {lookup && (
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Space wrap>
                    <Text strong style={{ fontSize: 16 }}>{lookupNumber}</Text>
                    {lookup.contact && <Tag color="blue" variant="filled">{t('备注：{{name}}', { name: lookup.contact.name })}</Tag>}
                    {!lookup.has_crm && !lookup.contact && <Tag variant="filled">{t('陌生号码')}</Tag>}
                  </Space>

                  {lookup.crm_customer && (
                    <Card size="small" styles={{ body: { padding: 12 } }} style={{ background: 'rgba(22,119,255,0.06)' }}>
                      <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                        <Space direction="vertical" size={2}>
                          <Space size={6}>
                            <Text strong>{lookup.crm_customer.name}</Text>
                            {lookup.crm_customer.level && <Tag color="gold" variant="filled">{t('{{level}} 级', { level: lookup.crm_customer.level })}</Tag>}
                          </Space>
                          <Text type="secondary" style={{ fontSize: 12 }}>
                            {[lookup.crm_customer.industry, lookup.crm_customer.phone].filter(Boolean).join(' · ') || t('CRM 客户')}
                          </Text>
                        </Space>
                        <Button type="link" icon={<LinkOutlined />}
                          onClick={() => navigate(`/crm/customers?keyword=${encodeURIComponent(lookup.crm_customer!.phone || lookup.crm_customer!.name)}`)}>
                          CRM
                        </Button>
                      </Space>
                    </Card>
                  )}
                  {lookup.crm_lead && !lookup.crm_customer && (
                    <Card size="small" styles={{ body: { padding: 12 } }} style={{ background: 'rgba(250,140,22,0.06)' }}>
                      <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                        <Space direction="vertical" size={2}>
                          <Space size={6}><Text strong>{lookup.crm_lead.name}</Text><Tag color="orange" variant="filled">{t('线索')}</Tag></Space>
                          <Text type="secondary" style={{ fontSize: 12 }}>{lookup.crm_lead.phone || t('未留电话')}</Text>
                        </Space>
                        <Button type="link" icon={<LinkOutlined />}
                          onClick={() => navigate(`/crm/leads?keyword=${encodeURIComponent(lookup.crm_lead!.phone || lookup.crm_lead!.name)}`)}>
                          CRM
                        </Button>
                      </Space>
                    </Card>
                  )}

                  <div>
                    <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                      <Text type="secondary" style={{ fontSize: 12 }}>{t('我的最近话单')}</Text>
                      <Tooltip title={t('刷新')}><Button type="text" size="small" icon={<ReloadOutlined />} onClick={() => void loadMyStats(myExt)} /></Tooltip>
                    </Space>
                    <List size="small" style={{ marginTop: 4, maxHeight: 280, overflow: 'auto' }} dataSource={myCalls}
                      locale={{ emptyText: <GlassEmpty text="暂无话单" compact /> }}
                      renderItem={(call) => (
                        <List.Item style={{ padding: '6px 0', cursor: 'pointer' }}
                          onClick={() => {
                            const n = call.direction === 'outbound' ? call.callee : call.caller
                            void runLookup(n)
                          }}>
                          <Space size={6}>
                            {dirTag(call.direction, t)}
                            <span className="cell-mono" style={{ fontSize: 12 }}>
                              {call.direction === 'outbound' ? call.callee : call.caller}
                            </span>
                          </Space>
                          <Text type="secondary" style={{ fontSize: 12 }}>{fmtSec(call.duration_sec)}</Text>
                        </List.Item>
                      )} />
                  </div>
                </Space>
              )}
            </div>
          </Card>
        </Col>

        {/* 右栏：客户 360（电话模式） */}
        <Col xs={24} lg={10}>
          <Card
            title={<Space><ContactsOutlined />{t('客户 360')}</Space>}
            bordered={false}
            styles={{ body: { padding: 0 } }}
            className="cc-workbench-360-card"
          >
            <Customer360Panel
              phone={focusPhone || null}
              displayName={
                lookup?.crm_customer?.name ||
                lookup?.crm_lead?.name ||
                lookup?.contact?.name
              }
              refreshToken={panelRefresh}
              compact
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
