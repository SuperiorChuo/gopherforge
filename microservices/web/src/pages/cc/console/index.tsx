import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Card, Input, Form, Badge, Tooltip, Switch,
  Modal, Radio, Checkbox, Select,
  Grid,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PhoneOutlined, DisconnectOutlined, UserOutlined, AudioOutlined,
  EyeOutlined, VideoCameraOutlined, SwapOutlined, PauseCircleOutlined, PlayCircleOutlined,
  CarOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  getCcFsStatus, listFsChannels, fsOriginate, fsHangup, superviseCall, uuidRecord,
  transferCall, holdCall, parkCall, listParked,
  type CcFsStatus, type FsChannel, type ParkedSlot,
} from '@/api/cc'
import WebPhone from './WebPhone'
import ErrorBoundary from '@/components/common/ErrorBoundary'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { formatDateTime } from '@/utils/format'
import { subscribeCcEvents } from '@/utils/ccSocket'

// 停泊车位下拉：6101-6110（6100 段共 100 个，前端只暴露前 10 个够坐席用）
const PARK_SLOTS = Array.from({ length: 10 }, (_, i) => String(6101 + i))

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

function FsBadge({ fs }: { fs: CcFsStatus | null }) {
  const { t } = useTranslation()
  if (!fs) return <Badge status="default" text={t('状态获取中…')} />
  if (!fs.configured) return <Badge status="default" text={t('FreeSWITCH 未接入')} />
  if (!fs.reachable) {
    return (
      <Tooltip title={fs.error}>
        <Badge status="error" text={t('control-api 不可达')} />
      </Tooltip>
    )
  }
  return <Badge status="success" text={t('FreeSWITCH 已连接')} />
}

export default function CcConsolePage() {
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { t } = useTranslation()
  const [fs, setFs] = useState<CcFsStatus | null>(null)
  const [channels, setChannels] = useState<FsChannel[]>([])
  const [parked, setParked] = useState<ParkedSlot[]>([])
  const [channelsLoading, setChannelsLoading] = useState(false)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [calling, setCalling] = useState(false)
  const [callForm] = Form.useForm<{ from_ext: string; to: string }>()
  // 轮询期间静默加载，避免 loading 闪烁
  const silentRef = useRef(false)

  const fetchChannels = useCallback(async (silent = false) => {
    if (!silent) setChannelsLoading(true)
    try {
      // 活动通话与停泊车位合并触发，一次轮询拿两份数据
      const [chRes, parkRes] = await Promise.all([
        listFsChannels(),
        listParked().catch(() => ({ list: [] as ParkedSlot[], total: 0 })),
      ])
      setChannels(chRes.list ?? [])
      setParked(parkRes.list ?? [])
    } catch {
      if (!silent) message.error(t('获取活动通话失败'))
    } finally {
      if (!silent) setChannelsLoading(false)
    }
  }, [t])

  useEffect(() => {
    getCcFsStatus().then(setFs).catch(() => setFs({ configured: false }))
    fetchChannels()
  }, [fetchChannels])

  // 10s 轮询兜底（事件驱动为主）；后台标签页暂停，回前台立即补一次。
  // autoRefresh 走 ref，开关不重建计时器
  const autoRefreshRef = useRef(autoRefresh)
  useEffect(() => {
    autoRefreshRef.current = autoRefresh
  })
  useVisibilityInterval(() => {
    if (!autoRefreshRef.current || silentRef.current) return
    silentRef.current = true
    void fetchChannels(true).finally(() => { silentRef.current = false })
  }, 10000, false)

  // 事件驱动为主：收到实时话务事件立即静默刷新一次（轮询 10s 兜底）。
  useEffect(() => {
    if (!autoRefresh) return
    const unsub = subscribeCcEvents(() => {
      if (silentRef.current) return
      silentRef.current = true
      fetchChannels(true).finally(() => { silentRef.current = false })
    })
    return unsub
  }, [autoRefresh, fetchChannels])

  const handleOriginate = async (values: { from_ext: string; to: string }) => {
    setCalling(true)
    try {
      await fsOriginate(values.from_ext.trim(), values.to.trim())
      message.success(t('已发起呼叫：分机 {{ext}} 将先振铃，接起后呼 {{to}}', { ext: values.from_ext, to: values.to }))
      setTimeout(() => fetchChannels(true), 1200)
    } catch (e) {
      message.error((e as Error)?.message || t('发起呼叫失败'))
    } finally {
      setCalling(false)
    }
  }

  const handleHangup = async (uuid: string) => {
    try {
      await fsHangup(uuid)
      message.success(t('已强拆通道'))
      fetchChannels(true)
    } catch (e) {
      message.error((e as Error)?.message || t('强拆失败'))
    }
  }

  // 监听弹窗：班长分机 + 模式
  const [supervising, setSupervising] = useState<string | null>(null) // 目标通道 uuid
  const [supMode, setSupMode] = useState<'listen' | 'whisper' | 'barge'>('listen')
  const [supExt, setSupExt] = useState('')
  const [supLoading, setSupLoading] = useState(false)
  // 手动录音中的通道集合
  const [recording, setRecording] = useState<Record<string, boolean>>({})

  const handleSupervise = async () => {
    if (!supervising || !/^\d{2,16}$/.test(supExt)) {
      message.error(t('请填写班长分机号'))
      return
    }
    setSupLoading(true)
    try {
      await superviseCall(supExt, supervising, supMode)
      message.success(t('已发起监听：分机 {{ext}} 将振铃，接起后进入（话机按 0静默/1耳语/2对客/3强插 切换）', { ext: supExt }))
      setSupervising(null)
    } catch (e) {
      message.error((e as Error)?.message || t('发起监听失败'))
    } finally {
      setSupLoading(false)
    }
  }

  const handleRecordToggle = async (uuid: string) => {
    const active = !!recording[uuid]
    try {
      await uuidRecord(uuid, active ? 'stop' : 'start')
      setRecording((prev) => ({ ...prev, [uuid]: !active }))
      message.success(active ? t('已停止录音') : t('已开始录音（文件在「通话录音」页，manual- 前缀）'))
    } catch (e) {
      message.error((e as Error)?.message || t('录音操作失败'))
    }
  }

  // 转移弹窗：目标号码 + 双方一起转
  const [transferring, setTransferring] = useState<string | null>(null) // 目标通道 uuid
  const [transferTo, setTransferTo] = useState('')
  const [transferBoth, setTransferBoth] = useState(false)
  const [transferLoading, setTransferLoading] = useState(false)

  const handleTransfer = async () => {
    if (!transferring || !/^[0-9*#+]{2,32}$/.test(transferTo.trim())) {
      message.error(t('请填写合法的转移目标号码'))
      return
    }
    setTransferLoading(true)
    try {
      await transferCall(transferring, transferTo.trim(), transferBoth)
      message.success(transferBoth ? t('已转移（双方一起转）') : t('已转移当前腿到目标号码'))
      setTransferring(null)
      setTransferTo('')
      setTransferBoth(false)
      setTimeout(() => fetchChannels(true), 800)
    } catch (e) {
      message.error((e as Error)?.message || t('转移失败'))
    } finally {
      setTransferLoading(false)
    }
  }

  // 保持/恢复：本地记录哪些通道被我们置为保持
  const [held, setHeld] = useState<Record<string, boolean>>({})

  const handleHoldToggle = async (uuid: string) => {
    const isHeld = !!held[uuid]
    try {
      await holdCall(uuid, 'toggle')
      setHeld((prev) => ({ ...prev, [uuid]: !isHeld }))
      message.success(isHeld ? t('已恢复通话') : t('已保持通话'))
      setTimeout(() => fetchChannels(true), 800)
    } catch (e) {
      message.error((e as Error)?.message || t('保持操作失败'))
    }
  }

  // 停泊弹窗：选车位
  const [parking, setParking] = useState<string | null>(null) // 目标通道 uuid
  const [parkSlot, setParkSlot] = useState<string>(PARK_SLOTS[0])
  const [parkLoading, setParkLoading] = useState(false)

  const handlePark = async () => {
    if (!parking) return
    setParkLoading(true)
    try {
      const res = await parkCall(parking, parkSlot)
      message.success(t('已停泊到车位 {{slot}}，任意分机拨该号码可取回', { slot: res.slot || parkSlot }))
      setParking(null)
      setTimeout(() => fetchChannels(true), 800)
    } catch (e) {
      message.error((e as Error)?.message || t('停泊失败'))
    } finally {
      setParkLoading(false)
    }
  }

  const channelColumns: ColumnsType<FsChannel> = [
    {
      title: t('通道'),
      dataIndex: 'uuid',
      width: 120,
      render: (v: string) => (
        <Tooltip title={v}>
          <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>{v.slice(0, 8)}…</span>
        </Tooltip>
      ),
    },
    {
      title: t('方向'),
      dataIndex: 'direction',
      width: 90,
      render: (v: string) =>
        v === 'outbound'
          ? <Tag color="cyan" variant="filled">{t('呼出')}</Tag>
          : <Tag color="geekblue" variant="filled">{t('呼入')}</Tag>,
    },
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
      title: t('应用'),
      dataIndex: 'application',
      width: 110,
      render: (v: string) => (v ? <Tag variant="filled">{v}</Tag> : <span className="cell-dim">—</span>),
    },
    { title: t('建立时间'), dataIndex: 'created', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 200,
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
              onClick: () => { setSupervising(row.uuid); setSupMode('listen') },
            },
            {
              key: 'transfer',
              label: t('转移'),
              icon: <SwapOutlined />,
              onClick: () => { setTransferring(row.uuid); setTransferTo(''); setTransferBoth(false) },
            },
            {
              key: 'hold',
              label: held[row.uuid] ? t('恢复') : t('保持'),
              icon: held[row.uuid] ? <PlayCircleOutlined /> : <PauseCircleOutlined />,
              onClick: () => handleHoldToggle(row.uuid),
            },
            {
              key: 'park',
              label: t('停泊'),
              icon: <CarOutlined />,
              onClick: () => { setParking(row.uuid); setParkSlot(PARK_SLOTS[0]) },
            },
            {
              key: 'record',
              label: recording[row.uuid] ? t('停录') : t('录音'),
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

  const parkedColumns: ColumnsType<ParkedSlot> = [
    {
      title: t('车位'),
      dataIndex: 'slot',
      width: 120,
      render: (v: string) => <Tag color="gold" variant="filled">{v}</Tag>,
    },
    {
      title: t('主叫'),
      dataIndex: 'caller',
      width: 160,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('取回方式'),
      key: 'tip',
      render: (_, row) => (
        <span className="cell-dim" style={{ fontSize: 13 }}>{t('任意分机拨 {{slot}} 取回', { slot: row.slot })}</span>
      ),
    },
  ]

  return (
    <div className="page-list cc-console-page">
      <ErrorBoundary
        inline
        title={t('WebPhone 电话条初始化异常')}
        description={t('麦克风权限或 SIP 客户端加载异常，请重试')}
      >
        <WebPhone />
      </ErrorBoundary>
      <Card className="list-filter-card" bordered={false}>
        <Space size={24} wrap>
          <FsBadge fs={fs} />
          <Form form={callForm} layout="inline" onFinish={handleOriginate}>
            <Form.Item
              name="from_ext"
              rules={[{ required: true, message: t('坐席分机') }, { pattern: /^[0-9*#+]{2,32}$/, message: t('格式不对') }]}
            >
              <Input placeholder={t('坐席分机（如 1000）')} prefix={<UserOutlined />} style={{ width: 180 }} allowClear />
            </Form.Item>
            <Form.Item
              name="to"
              rules={[{ required: true, message: t('被叫号码') }, { pattern: /^[0-9*#+]{2,32}$/, message: t('格式不对') }]}
            >
              <Input placeholder={t('被叫（分机/5000 队列/9196 回声）')} prefix={<PhoneOutlined />} style={{ width: 220 }} allowClear />
            </Form.Item>
            <Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                icon={<AudioOutlined />}
                loading={calling}
                disabled={!fs?.configured}
              >
                {t('发起呼叫')}
              </Button>
            </Form.Item>
          </Form>
        </Space>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="活动通话"
          total={channels.length}
          extra={
            <Space size={16}>
              <Space size={8}>
                <span className="cell-dim" style={{ fontSize: 13 }}>{t('自动刷新')}</span>
                <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
              </Space>
              <Button icon={<ReloadOutlined />} onClick={() => fetchChannels()}>{t('刷新')}</Button>
            </Space>
          }
        />
        <Table
          rowKey="uuid"
          className="list-table"
          columns={channelColumns}
          dataSource={channels}
          loading={channelsLoading}
          pagination={false}
          scroll={{ x: 2000 }}
          locale={{ emptyText: <GlassEmpty text="当前没有进行中的通话" compact /> }}
        />
      </Card>

      <Card className="list-main-card" bordered={false} style={{ marginTop: 16 }}>
        <TableToolbar title="停泊车位" total={parked.length} />
        <Table
          rowKey="slot"
          className="list-table"
          columns={parkedColumns}
          dataSource={parked}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="当前没有停泊的通话（停泊后任意分机拨车位号取回）" compact /> }}
        />
      </Card>

      <Modal
        title={t('班长监听')}
        open={!!supervising}
        onCancel={() => setSupervising(null)}
        onOk={handleSupervise}
        confirmLoading={supLoading}
        okText={t('发起监听')}
        destroyOnHidden
      >
        <Space direction="vertical" size={14} style={{ width: '100%', marginTop: 8 }}>
          <Input
            placeholder={t('班长分机号（先振铃它，可用网页坐席分机）')}
            prefix={<UserOutlined />}
            value={supExt}
            onChange={(e) => setSupExt(e.target.value)}
          />
          <Radio.Group value={supMode} onChange={(e) => setSupMode(e.target.value)}>
            <Radio.Button value="listen">{t('静默监听')}</Radio.Button>
            <Radio.Button value="whisper">{t('耳语坐席')}</Radio.Button>
            <Radio.Button value="barge">{t('三方强插')}</Radio.Button>
          </Radio.Group>
          <span className="cell-dim" style={{ fontSize: 12 }}>
            {t('接起后话机按键切换：0 静默 / 1 只对坐席说 / 2 只对客户说 / 3 三方通话')}
          </span>
        </Space>
      </Modal>

      <Modal
        title={t('转移通话')}
        open={!!transferring}
        onCancel={() => setTransferring(null)}
        onOk={handleTransfer}
        confirmLoading={transferLoading}
        okText={t('转移')}
        destroyOnHidden
      >
        <Space direction="vertical" size={14} style={{ width: '100%', marginTop: 8 }}>
          <Input
            placeholder={t('目标号码（分机/5000 队列/9+外线）')}
            prefix={<PhoneOutlined />}
            value={transferTo}
            onChange={(e) => setTransferTo(e.target.value)}
            onPressEnter={handleTransfer}
            allowClear
          />
          <Checkbox checked={transferBoth} onChange={(e) => setTransferBoth(e.target.checked)}>
            {t('双方一起转（-both，把整通通话都带到目标）')}
          </Checkbox>
          <span className="cell-dim" style={{ fontSize: 12 }}>
            {t('默认只转当前腿（客户被转走、坐席留下）；勾选后主被叫一起转到目标。')}
          </span>
        </Space>
      </Modal>

      <Modal
        title={t('停泊通话')}
        open={!!parking}
        onCancel={() => setParking(null)}
        onOk={handlePark}
        confirmLoading={parkLoading}
        okText={t('停泊')}
        destroyOnHidden
      >
        <Space direction="vertical" size={14} style={{ width: '100%', marginTop: 8 }}>
          <Select
            value={parkSlot}
            onChange={setParkSlot}
            style={{ width: 220 }}
            options={PARK_SLOTS.map((s) => ({ value: s, label: t('车位 {{slot}}', { slot: s }) }))}
          />
          <span className="cell-dim" style={{ fontSize: 12 }}>
            {t('停泊后通话进入等待，任意分机拨对应车位号即可取回。')}
          </span>
        </Space>
      </Modal>
    </div>
  )
}
