import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Input, Select, Form, Badge, Tooltip, Modal, Progress,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  SearchOutlined, ReloadOutlined, PhoneOutlined, ApiOutlined, ThunderboltOutlined,
  ArrowDownOutlined, ArrowUpOutlined, PlayCircleOutlined, UserOutlined, AudioOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listCcCalls, getCcFsStatus, testCcWebhook, fetchFsRecordingBlob, upsertCcContact, transcribeCall,
  getTranscribeJob,
  type CcCall, type CcFsStatus, type QcTranscribeResult, type QcHit, HANGUP_CLASS_LABEL,
} from '@/api/cc'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  status?: string
  caller?: string
  agent_ext?: string
  page: number
  page_size: number
}

const STATUS_META: Record<string, { label: string; color: string }> = {
  ringing: { label: '振铃中', color: 'processing' },
  answered: { label: '通话中', color: 'success' },
  hangup: { label: '已挂断', color: 'default' },
  recording_ready: { label: '录音就绪', color: 'purple' },
}

function statusTag(v: string, t: TFunction) {
  const meta = STATUS_META[v]
  if (meta) return <Tag color={meta.color} variant="filled">{t(meta.label)}</Tag>
  return <Tag variant="filled">{v || '—'}</Tag>
}

function formatDuration(sec: number, t: TFunction) {
  if (!sec) return '—'
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m > 0 ? t('{{m}}分{{s}}秒', { m, s }) : t('{{s}}秒', { s })
}

// 质检评分色：≥80 绿 / 60-79 橙 / <60 红
function scoreColor(score: number) {
  if (score >= 80) return 'var(--c-success)'
  if (score >= 60) return 'var(--c-orange)'
  return 'var(--c-error-strong)'
}

// 转写全文里高亮命中词：禁词红底、必说词（命中的）绿底
function highlightText(text: string, hits: QcHit[]) {
  const marks = hits.filter((h) => h.count > 0)
  if (!marks.length) return <span>{text}</span>
  // 按词长倒序，避免短词先切断长词
  const words = marks.slice().sort((a, b) => b.word.length - a.word.length)
  // 半透明语义底色同时适配深浅主题，文字保持足够对比度。
  const styleOf = (w: string): React.CSSProperties => {
    const h = marks.find((m) => m.word === w)
    return h?.type === 'forbidden'
      ? { background: 'rgba(255, 77, 79, 0.14)', border: '1px solid rgba(255, 77, 79, 0.32)', color: 'var(--c-error)' }
      : { background: 'rgba(82, 196, 26, 0.14)', border: '1px solid rgba(82, 196, 26, 0.32)', color: 'var(--c-success-strong)' }
  }
  // 用分隔符切分：转义正则特殊字符
  const escaped = words.map((h) => h.word.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  const re = new RegExp(`(${escaped.join('|')})`, 'g')
  const parts = text.split(re)
  return (
    <span>
      {parts.map((seg, i) => (words.some((h) => h.word === seg) ? (
        <mark
          key={i}
          style={{ ...styleOf(seg), borderRadius: 3, padding: '0 2px' }}
        >
          {seg}
        </mark>
      ) : (
        <span key={i}>{seg}</span>
      )))}
    </span>
  )
}

// FS 接入状态徽标：未配置 / 已配置不可达 / 可达
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

export default function CcCallsPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<CcCall[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [fs, setFs] = useState<CcFsStatus | null>(null)
  const [testing, setTesting] = useState(false)
  const [searchForm] = Form.useForm()
  // 录音播放弹窗：blob URL 关闭时释放
  const [playing, setPlaying] = useState<{ name: string; url: string } | null>(null)
  // 备注姓名弹窗（来显归属：号码 → 客户名，upsert cc_contacts）
  const [contactModal, setContactModal] = useState<{ number: string } | null>(null)
  const [contactSaving, setContactSaving] = useState(false)
  const [contactForm] = Form.useForm<{ name: string; remark?: string }>()
  // 转写质检弹窗
  const [transcribing, setTranscribing] = useState<string | null>(null) // 正在转写的 call_id
  const [qc, setQc] = useState<(QcTranscribeResult & { callId: string }) | null>(null)
  const transcribePollRef = useRef<object | null>(null) // 当前生效的轮询令牌，卸载/换通话即失效

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listCcCalls(p)
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取话单失败'))
    } finally {
      setLoading(false)
    }
  }

  // 列表页惯用式：params 引用变化即重拉（含搜索/分页），fetchList 身份刻意排除。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    fetchList(params)
  }, [params])

  useEffect(() => {
    getCcFsStatus().then(setFs).catch(() => setFs({ configured: false }))
  }, [])

  // 卸载时中止转写轮询，避免卸载后继续请求与 setState
  useEffect(() => () => { transcribePollRef.current = null }, [])

  const handleWebhookTest = async () => {
    setTesting(true)
    try {
      await testCcWebhook()
      message.success(t('测试事件已发出，稍后可在话单留档中查看'))
    } catch {
      message.error(t('webhook 测试失败，请检查 control-api 配置'))
    } finally {
      setTesting(false)
    }
  }

  const handleSearch = (values: { status?: string; caller?: string; agent_ext?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  // recording 存 FS 容器内路径，播放取 basename 走流式代理
  const handlePlayRecording = async (recordingPath: string) => {
    const name = recordingPath.split('/').pop() || ''
    try {
      const blob = await fetchFsRecordingBlob(name)
      setPlaying({ name, url: URL.createObjectURL(blob) })
    } catch {
      message.error(t('拉取录音失败（文件可能已清理）'))
    }
  }

  const closePlayer = () => {
    if (playing) URL.revokeObjectURL(playing.url)
    setPlaying(null)
  }

  // 备注姓名：把主叫号 → 客户名写入联系人库，下次该号来电弹屏/话机显示名字
  const openContact = (number: string) => {
    contactForm.resetFields()
    setContactModal({ number })
  }

  const handleContactSave = async (values: { name: string; remark?: string }) => {
    if (!contactModal) return
    setContactSaving(true)
    try {
      await upsertCcContact({ number: contactModal.number, name: values.name.trim(), remark: values.remark?.trim() })
      message.success(t('已为 {{number}} 备注「{{name}}」，下次来电即显示', { number: contactModal.number, name: values.name.trim() }))
      setContactModal(null)
    } catch (e) {
      message.error((e as Error)?.message || t('备注失败'))
    } finally {
      setContactSaving(false)
    }
  }

  // 转写质检：入队后后台跑（录音不再进请求内存），前端轮询任务状态。
  // 轮询 2s 一次、上限 5 分钟；组件卸载或换通话时由 transcribePollRef 中止。
  const handleTranscribe = async (callId: string) => {
    setTranscribing(callId)
    const token = {}
    transcribePollRef.current = token
    try {
      const accepted = await transcribeCall(callId)
      if (!accepted.created) message.info(t('该通话已有转写任务在进行，正在等待结果'))

      const deadline = Date.now() + 5 * 60 * 1000
      for (;;) {
        if (transcribePollRef.current !== token) return // 已被后续操作取代
        const res = await getTranscribeJob(accepted.job_id)
        if (res.job.status === 'done') {
          setQc({ ...res, callId })
          return
        }
        if (res.job.status === 'failed') {
          message.error(res.job.error || t('转写失败（ASR 未配置或录音不可用）'))
          return
        }
        if (Date.now() > deadline) {
          message.warning(t('转写仍在进行，稍后可重新点击查看结果'))
          return
        }
        await new Promise((r) => setTimeout(r, 2000))
      }
    } catch (e) {
      message.error((e as Error)?.message || t('转写失败（ASR 未配置或录音不可用）'))
    } finally {
      if (transcribePollRef.current === token) {
        transcribePollRef.current = null
        setTranscribing(null)
      }
    }
  }

  const columns: ColumnsType<CcCall> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: t('呼叫'),
      dataIndex: 'direction',
      width: 90,
      render: (v: string) =>
        v === 'outbound' ? (
          <Tag icon={<ArrowUpOutlined />} color="cyan" variant="filled">{t('呼出')}</Tag>
        ) : (
          <Tag icon={<ArrowDownOutlined />} color="geekblue" variant="filled">{t('呼入')}</Tag>
        ),
    },
    {
      title: t('主叫'),
      dataIndex: 'caller',
      width: 140,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('被叫'),
      dataIndex: 'callee',
      width: 140,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('坐席分机'),
      dataIndex: 'agent_ext',
      width: 100,
      render: (v: string) => (v ? <span className="cell-mono">{v}</span> : <span className="cell-dim">—</span>),
    },
    {
      title: t('队列'),
      dataIndex: 'queue',
      width: 110,
      render: (v: string) => (v ? <Tag variant="filled">{v}</Tag> : <span className="cell-dim">—</span>),
    },
    { title: t('状态'), dataIndex: 'status', width: 110, render: (v: string) => statusTag(v, t) },
    {
      title: t('收号'),
      dataIndex: 'collected_digits',
      width: 90,
      render: (v: string) =>
        v ? <Tag color="cyan" variant="filled" className="cell-mono">{v}</Tag> : <span className="cell-dim">—</span>,
    },
    {
      title: t('时长'),
      dataIndex: 'duration_sec',
      width: 90,
      render: (v: number) => formatDuration(v, t),
    },
    {
      title: t('挂断原因'),
      dataIndex: 'hangup_class',
      width: 120,
      render: (cls: string, row: CcCall) => {
        const label = cls ? (HANGUP_CLASS_LABEL[cls] || cls) : ''
        if (!label && !row.hangup_cause) return '—'
        return (
          <Tooltip title={row.hangup_cause || undefined}>
            <span>{label || <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>{row.hangup_cause}</span>}</span>
          </Tooltip>
        )
      },
    },
    { title: t('开始时间'), dataIndex: 'started_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('录音'),
      dataIndex: 'recording',
      width: 90,
      render: (v: string) =>
        v ? (
          <Button size="small" icon={<PlayCircleOutlined />} onClick={() => handlePlayRecording(v)}>{t('播放')}</Button>
        ) : (
          <span className="cell-dim">—</span>
        ),
    },
    {
      title: t('转写质检'),
      key: 'transcribe',
      width: 120,
      render: (_, row) =>
        row.recording ? (
          <Button
            size="small"
            icon={<AudioOutlined />}
            loading={transcribing === row.call_id}
            onClick={() => handleTranscribe(row.call_id)}
          >
            {t('转写质检')}
          </Button>
        ) : (
          <span className="cell-dim">—</span>
        ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 48,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, row) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'note',
              label: t('备注姓名'),
              icon: <UserOutlined />,
              disabled: !row.caller,
              onClick: () => openContact(row.caller),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-calls-page"
      filter={
        <ListFilterForm
          form={searchForm}
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="caller">
            <Input placeholder={t('主叫号码')} prefix={<PhoneOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="agent_ext">
            <Input placeholder={t('坐席分机')} allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 140 }} allowClear>
              {Object.entries(STATUS_META).map(([value, meta]) => (
                <Select.Option key={value} value={value}>{t(meta.label)}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      }
      toolbar={
        <TableToolbar
          title="呼叫话单"
          total={total}
          extra={
            <Space size={16} wrap>
              <FsBadge fs={fs} />
              <Button
                icon={<ThunderboltOutlined />}
                loading={testing}
                disabled={!fs?.configured}
                onClick={handleWebhookTest}
              >
                {t('Webhook 测试')}
              </Button>
              <Button icon={<ApiOutlined />} onClick={() => getCcFsStatus().then(setFs)}>
                {t('检测连接')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>{t('刷新')}</Button>
            </Space>
          }
        />}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 1600 }}
          locale={{ emptyText: <GlassEmpty text="暂无话单记录" compact /> }}
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

      <Modal title={playing?.name} open={!!playing} onCancel={closePlayer} footer={null} destroyOnHidden>
        {playing && (
          // eslint-disable-next-line jsx-a11y/media-has-caption
          <audio controls autoPlay src={playing.url} style={{ width: '100%' }} />
        )}
      </Modal>

      <Modal
        title={t('备注姓名：{{number}}', { number: contactModal?.number ?? '' })}
        open={!!contactModal}
        onCancel={() => setContactModal(null)}
        onOk={() => contactForm.submit()}
        confirmLoading={contactSaving}
        okText={t('保存')}
        destroyOnHidden
      >
        <Form form={contactForm} layout="vertical" onFinish={handleContactSave} style={{ marginTop: 8 }}>
          <Form.Item
            name="name"
            label={t('客户名')}
            rules={[{ required: true, message: t('请输入客户名') }, { max: 128, message: t('最多 128 字') }]}
          >
            <Input placeholder={t('下次该号来电时话机/弹屏显示此名')} prefix={<UserOutlined />} maxLength={128} />
          </Form.Item>
          <Form.Item name="remark" label={t('备注（可选）')} rules={[{ max: 256, message: t('最多 256 字') }]}>
            <Input.TextArea placeholder={t('内部备注，仅记录用')} rows={2} maxLength={256} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Space><AudioOutlined />{t('录音转写质检')}</Space>}
        open={!!qc}
        onCancel={() => setQc(null)}
        footer={null}
        destroyOnHidden
        width={720}
      >
        {qc && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Space size={24} align="center">
              <Progress
                type="circle"
                percent={qc.score ?? 0}
                size={90}
                format={(p) => <span style={{ color: scoreColor(qc.score ?? 0) }}>{p}</span>}
                strokeColor={scoreColor(qc.score ?? 0)}
              />
              <div>
                <div style={{ fontSize: 13, marginBottom: 8 }}>{t('命中关键词')}</div>
                <Space size={6} wrap>
                  {qc.hits?.length ? qc.hits.map((h) => (
                    <Tag
                      key={h.word}
                      color={h.type === 'forbidden' ? 'red' : h.count > 0 ? 'green' : 'default'}
                    >
                      {h.type === 'forbidden' ? t('禁') : t('必')} {h.word}
                      {h.type === 'forbidden' ? ` ×${h.count}` : h.count > 0 ? ' ✓' : t('缺失')}
                    </Tag>
                  )) : <span className="cell-dim">{t('无命中')}</span>}
                </Space>
              </div>
            </Space>
            <div>
              <div style={{ fontSize: 13, marginBottom: 8 }}>{t('转写全文')}</div>
              <div
                style={{
                  maxHeight: 320,
                  overflow: 'auto',
                  padding: 12,
                  borderRadius: 8,
                  background: 'rgba(128,128,160,0.08)',
                  lineHeight: 1.9,
                  whiteSpace: 'pre-wrap',
                }}
              >
                {highlightText(qc.text ?? '', qc.hits ?? [])}
              </div>
            </div>
          </Space>
        )}
      </Modal>
    </ListPageShell>
  )
}
