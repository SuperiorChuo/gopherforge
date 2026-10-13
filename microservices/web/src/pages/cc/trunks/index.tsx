import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Input, InputNumber, Form, Badge, Tooltip, Modal, Switch, Radio, Collapse, Select, Empty, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined, ApiOutlined, GlobalOutlined,
  NodeIndexOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listFsGateways, createFsGateway, updateFsGateway, deleteFsGateway, previewLcr,
  type FsGateway, type FsGatewayPayload, type LcrPreviewRow,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

// 费率分↔元互转（DB 存分/分钟，界面用元/分钟）
const centsToYuan = (cents?: number) => (cents && cents > 0 ? cents / 100 : undefined)
const yuanToCents = (yuan?: number) => (yuan && yuan > 0 ? Math.round(yuan * 100) : 0)

// 对接模式：注册中继（REGISTER+账密）/ IP 点对点（免注册免鉴权，对端白名单）/
// 免注册鉴权（不发 REGISTER，INVITE 带账密——部分运营商这种）
type TrunkMode = 'reg' | 'peer' | 'auth'

const MODE_META: Record<TrunkMode, { label: string; hint: string }> = {
  reg: { label: '注册中继', hint: '向运营商发 REGISTER，需账号密码' },
  peer: { label: 'IP 点对点', hint: '不注册不鉴权，双方按 IP 白名单互信（落地网关常用）' },
  auth: { label: '免注册鉴权', hint: '不发 REGISTER，但呼叫时按账密应答鉴权质询' },
}

function modeOf(g: FsGateway): TrunkMode {
  if (g.register) return 'reg'
  return g.username ? 'auth' : 'peer'
}

function stateBadge(state: string, t: TFunction) {
  switch (state) {
    case 'REGED':
      return <Badge status="success" text={t('已注册')} />
    case 'NOREG':
      return <Badge status="processing" text={t('在线(免注册)')} />
    case 'TRYING':
      return <Badge status="warning" text={t('注册中')} />
    case 'UNREGED':
    case 'UNREGISTER':
      return <Badge status="default" text={t('未注册')} />
    case 'FAIL_WAIT':
    case 'FAILED':
      return <Badge status="error" text={t('注册失败')} />
    case 'DOWN':
      return <Badge status="error" text={t('探测离线')} />
    case 'NOT_LOADED':
      return <Badge status="error" text={t('未加载')} />
    default:
      return <Badge status="default" text={state || t('未知')} />
  }
}

type FormValues = {
  mode: TrunkMode
  name: string
  realm: string
  proxy?: string
  username?: string
  password?: string
  extension?: string
  dial_prefix?: string
  prepend?: string
  caller_number?: string
  register_transport?: string
  expire_seconds?: number
  retry_seconds?: number
  ping_seconds?: number
  caller_id_in_from?: boolean
  from_user?: string
  from_domain?: string
  max_concurrent?: number
  // LCR：费率（元/分，界面单位）+ 同费率排序权重
  rate_yuan?: number
  sort_weight?: number
  // 线路归属租户（转售计费）：出局通话按该租户入账
  tenant_id?: number
}

export default function CcTrunksPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const hostRule = { pattern: /^[A-Za-z0-9.-]{2,64}(:\d{2,5})?$/, message: t('域名或 IP[:端口]') }
  const [modalOpen, setModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  // 非 null 表示编辑态（名称锁定，PUT）
  const [editing, setEditing] = useState<FsGateway | null>(null)
  const [form] = Form.useForm<FormValues>()

  // 选路预览
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewNumber, setPreviewNumber] = useState('')
  const [previewRows, setPreviewRows] = useState<LcrPreviewRow[]>([])
  const [previewMatched, setPreviewMatched] = useState<boolean | null>(null)
  const [previewing, setPreviewing] = useState(false)

  const fetcher = useCallback(async () => {
    const res = await listFsGateways()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取线路列表失败')), [t]),
  })

  const runPreview = async () => {
    const num = previewNumber.trim()
    if (!/^[0-9*#]{3,32}$/.test(num)) {
      message.warning(t('请输入 3-32 位号码（含字冠）'))
      return
    }
    setPreviewing(true)
    try {
      const res = await previewLcr(num)
      setPreviewRows(res.list ?? [])
      setPreviewMatched(res.matched)
    } catch (e) {
      message.error((e as Error)?.message || t('选路预览失败'))
    } finally {
      setPreviewing(false)
    }
  }

  const openPreview = () => {
    setPreviewNumber('')
    setPreviewRows([])
    setPreviewMatched(null)
    setPreviewOpen(true)
  }

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ mode: 'reg', caller_id_in_from: false, ping_seconds: 25, max_concurrent: 0 })
    setModalOpen(true)
  }

  const openEdit = (g: FsGateway) => {
    setEditing(g)
    form.setFieldsValue({
      mode: modeOf(g),
      name: g.name,
      realm: g.realm,
      proxy: g.proxy || undefined,
      username: g.username || undefined,
      password: g.password || undefined,
      extension: g.extension || undefined,
      dial_prefix: g.dial_prefix || undefined,
      prepend: g.prepend || undefined,
      caller_number: g.caller_number || undefined,
      register_transport: g.register_transport || undefined,
      expire_seconds: g.expire_seconds || undefined,
      retry_seconds: g.retry_seconds || undefined,
      ping_seconds: g.ping_seconds || 0,
      caller_id_in_from: g.caller_id_in_from,
      from_user: g.from_user || undefined,
      from_domain: g.from_domain || undefined,
      max_concurrent: g.max_concurrent || 0,
      rate_yuan: centsToYuan(g.rate_per_min),
      sort_weight: g.sort_weight || undefined,
      tenant_id: g.tenant_id || undefined,
    })
    setModalOpen(true)
  }

  const handleSave = async (values: FormValues) => {
    const payload: FsGatewayPayload = {
      name: values.name.trim(),
      realm: values.realm.trim(),
      proxy: values.proxy?.trim(),
      register: values.mode === 'reg',
      username: values.mode === 'peer' ? '' : values.username?.trim(),
      password: values.mode === 'peer' ? '' : values.password?.trim(),
      extension: values.extension?.trim(),
      dial_prefix: values.dial_prefix?.trim(),
      prepend: values.prepend?.trim(),
      caller_number: values.caller_number?.trim(),
      register_transport: values.register_transport,
      expire_seconds: values.expire_seconds || 0,
      retry_seconds: values.retry_seconds || 0,
      ping_seconds: values.ping_seconds || 0,
      caller_id_in_from: values.caller_id_in_from ?? false,
      from_user: values.from_user?.trim(),
      from_domain: values.from_domain?.trim(),
      max_concurrent: values.max_concurrent || 0,
      rate_per_min: yuanToCents(values.rate_yuan),
      sort_weight: values.sort_weight || 0,
      tenant_id: values.tenant_id || 0,
    }
    setSaving(true)
    try {
      if (editing) {
        await updateFsGateway(editing.name, payload)
        message.success(t('线路 {{name}} 已更新并重载', { name: editing.name }))
      } else {
        await createFsGateway(payload)
        message.success(t('线路 {{name}} 已接入，external profile 已重载', { name: payload.name }))
      }
      setModalOpen(false)
      void reload()
      // 注册需要几秒，稍后再刷一次状态
      setTimeout(() => { void reload() }, 4000)
    } catch (e) {
      message.error((e as Error)?.message || t('保存线路失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteFsGateway(name)
      message.success(t('线路 {{name}} 已删除', { name }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除线路失败'))
    }
  }

  const columns: ColumnsType<FsGateway> = [
    {
      title: t('线路'),
      dataIndex: 'name',
      width: 140,
      render: (v: string) => (
        <Space size={6}><ApiOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'state',
      width: 130,
      responsive: ['sm'],
      render: (v: string, row) =>
        row.disabled ? (
          <Tooltip title={t('欠费冻结联动停用：注册已断、字冠直拨失效；到计费管理解冻租户后恢复')}>
            <Tag color="error" variant="filled">{t('已停用')}</Tag>
          </Tooltip>
        ) : (
          stateBadge(v, t)
        ),
    },
    {
      title: t('模式'),
      key: 'mode',
      width: 110,
      responsive: ['md'],
      render: (_, row) => {
        const m = modeOf(row)
        return (
          <Tooltip title={t(MODE_META[m].hint)}>
            <Tag color={m === 'reg' ? 'geekblue' : m === 'auth' ? 'purple' : 'default'} variant="filled">
              {t(MODE_META[m].label)}
            </Tag>
          </Tooltip>
        )
      },
    },
    {
      title: t('服务器'),
      key: 'server',
      responsive: ['sm'],
      render: (_, row) => (
        <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>
          {row.username ? `${row.username}@` : ''}{row.realm}
          {row.proxy && row.proxy !== row.realm ? ` (proxy ${row.proxy})` : ''}
          {row.register_transport ? ` /${row.register_transport.toUpperCase()}` : ''}
        </span>
      ),
    },
    {
      title: t('出局'),
      key: 'outbound',
      width: 150,
      responsive: ['md'],
      render: (_, row) => {
        if (!row.dial_prefix) return <span className="cell-dim">—</span>
        const pre = row.prepend ? t(' → 加前缀 {{prepend}}', { prepend: row.prepend }) : ''
        const caller = row.caller_number ? t('，显号 {{caller}}', { caller: row.caller_number }) : ''
        return (
          <Tooltip title={t('拨 {{dial}} + 号码 → 去冠{{pre}} → 送线路{{caller}}', { dial: row.dial_prefix, pre, caller })}>
            <Tag color="cyan" variant="filled">{t('{{dial}} + 号码{{pre}}…', { dial: row.dial_prefix, pre: row.prepend ? ` → ${row.prepend}` : '' })}</Tag>
          </Tooltip>
        )
      },
    },
    {
      title: t('费率(元/分)'),
      dataIndex: 'rate_per_min',
      width: 100,
      responsive: ['lg'],
      render: (v: number) =>
        v && v > 0 ? (
          <Tooltip title={t('同字冠多线路按费率升序 failover，便宜的先试')}>
            <Tag color="green" variant="filled">{(v / 100).toFixed(2)}</Tag>
          </Tooltip>
        ) : (
          <Tooltip title={t('未设费率：LCR 选路时视为最贵，排在最后')}>
            <span className="cell-dim">{t('未设')}</span>
          </Tooltip>
        ),
    },
    {
      title: t('租户'),
      dataIndex: 'tenant_id',
      width: 80,
      responsive: ['lg'],
      render: (v: number) =>
        v > 0 ? (
          <Tooltip title={t('出局通话按租户 {{n}} 计费出账', { n: v })}>
            <Tag color="gold" variant="filled">{v}</Tag>
          </Tooltip>
        ) : (
          <Tooltip title={t('未挂租户：按分机绑定或默认账归属')}>
            <span className="cell-dim">—</span>
          </Tooltip>
        ),
    },
    {
      title: t('入方向落点'),
      dataIndex: 'extension',
      width: 110,
      responsive: ['md'],
      render: (v: string) =>
        v ? (
          <Tooltip title={t('来电路由到该号码（分机/队列/IVR 接入号）')}>
            <Tag color="geekblue" variant="filled">{v}</Tag>
          </Tooltip>
        ) : (
          <span className="cell-dim">—</span>
        ),
    },
    {
      title: t('保活'),
      dataIndex: 'ping_seconds',
      width: 90,
      responsive: ['lg'],
      render: (v: number) =>
        v ? <Tag variant="filled">{v}s</Tag> : <span className="cell-dim">{t('关')}</span>,
    },
    {
      title: t('出局并发'),
      dataIndex: 'max_concurrent',
      width: 90,
      responsive: ['lg'],
      render: (v: number) =>
        v > 0 ? (
          <Tooltip title={t('本线路出局同时最多 {{n}} 路，超限自动 USER_BUSY 拒接', { n: v })}>
            <Tag color="volcano" variant="filled">{v}</Tag>
          </Tooltip>
        ) : (
          <span className="cell-dim">{t('不限')}</span>
        ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: row.name })}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(row) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: {
                title: t('确认删除线路 {{name}}？', { name: row.name }),
                description: t('进行中的外线通话会被摘除网关'),
              },
              onClick: () => { void handleDelete(row.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-trunks-page"
      toolbar={
        <TableToolbar
          title="SIP 线路"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('接入线路')}</Button>
              <Button icon={<NodeIndexOutlined />} onClick={openPreview}>{t('选路预览')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Table
          rowKey="name"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          scroll={{ x: 1000 }}
          locale={{ emptyText: <GlassEmpty text="暂无 SIP 线路（接入运营商/落地网关后分机可外呼、外线可进 IVR）" compact /> }}
        />

      <Modal
        title={editing ? t('编辑线路：{{name}}', { name: editing.name }) : t('接入 SIP 线路')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saving}
        okText={editing ? t('保存并重载') : t('接入')}
        destroyOnHidden
        width={620}
      >
        <Form form={form} layout="vertical" onFinish={handleSave} style={{ marginTop: 8 }}>
          <Form.Item name="mode" label={t('对接模式')} rules={[{ required: true }]}>
            <Radio.Group optionType="button" buttonStyle="solid">
              {(Object.keys(MODE_META) as TrunkMode[]).map((m) => (
                <Radio.Button key={m} value={m}>
                  <Tooltip title={t(MODE_META[m].hint)}>{t(MODE_META[m].label)}</Tooltip>
                </Radio.Button>
              ))}
            </Radio.Group>
          </Form.Item>

          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="name"
              label={t('线路名称')}
              style={{ flex: 1 }}
              rules={[
                { required: true, message: t('请输入线路名称') },
                { pattern: /^[a-z0-9][a-z0-9_-]{1,31}$/, message: t('2-32 位小写字母/数字/_-') },
              ]}
            >
              <Input placeholder={t('如 provider-a')} prefix={<ApiOutlined />} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="realm"
              label={t('SIP 服务器（realm）')}
              style={{ flex: 1 }}
              rules={[{ required: true, message: t('请输入 SIP 服务器地址') }, hostRule]}
            >
              <Input placeholder={t('sip.provider.com 或 192.168.1.50:5060')} prefix={<GlobalOutlined />} />
            </Form.Item>
          </Space>

          <Form.Item noStyle shouldUpdate={(p, c) => p.mode !== c.mode}>
            {({ getFieldValue }) =>
              getFieldValue('mode') !== 'peer' && (
                <Space size={16} style={{ display: 'flex' }} align="start">
                  <Form.Item
                    name="username"
                    label={t('账号')}
                    style={{ flex: 1 }}
                    rules={[{ required: true, message: t('该模式需要账号') }]}
                  >
                    <Input placeholder={t('线路账号')} maxLength={64} />
                  </Form.Item>
                  <Form.Item
                    name="password"
                    label={t('密码')}
                    style={{ flex: 1 }}
                    rules={[{ required: true, message: t('该模式需要密码') }]}
                  >
                    <Input.Password placeholder={t('线路密码')} maxLength={64} />
                  </Form.Item>
                </Space>
              )
            }
          </Form.Item>

          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="extension"
              label={t('入方向落点')}
              style={{ flex: 1 }}
              tooltip={t('本线路来电路由到该号码：分机 / 5000 队列 / IVR 接入号；留空跟随账号')}
              rules={[{ pattern: /^[0-9]{0,16}$/, message: t('数字') }]}
            >
              <Input placeholder={t('如 6000（进 IVR）')} />
            </Form.Item>
            <Form.Item
              name="dial_prefix"
              label={t('出局字冠')}
              style={{ flex: 1 }}
              tooltip={t('分机拨 字冠+号码 走本线路，如 9：拨 913800001111')}
              rules={[{ pattern: /^[0-9*#]{0,6}$/, message: t('0-6 位 [0-9*#]') }]}
            >
              <Input placeholder={t('如 9')} maxLength={6} />
            </Form.Item>
            <Form.Item
              name="prepend"
              label={t('送号前缀')}
              style={{ flex: 1 }}
              tooltip={t('去冠后再加到被叫前面送给运营商（tech prefix），按需填')}
              rules={[{ pattern: /^[0-9*#+]{0,16}$/, message: t('0-16 位 [0-9*#+]') }]}
            >
              <Input placeholder={t('如 0086')} maxLength={16} />
            </Form.Item>
            <Form.Item
              name="caller_number"
              label={t('出局显号')}
              style={{ flex: 1 }}
              tooltip={t('出局主叫号码改写（运营商分配的中继号）')}
              rules={[{ pattern: /^[0-9+]{0,32}$/, message: t('数字/+') }]}
            >
              <Input placeholder={t('如 057188880000')} maxLength={32} />
            </Form.Item>
          </Space>

          <Collapse
            ghost
            items={[{
              key: 'advanced',
              label: t('高级参数（费率选路 / 传输 / 保活 / 显号 From / 注册周期）'),
              children: (
                <>
                  <Space size={16} style={{ display: 'flex' }} align="start">
                    <Form.Item
                      name="rate_yuan"
                      label={t('费率(元/分)')}
                      style={{ width: 150 }}
                      tooltip={t('同一出局字冠有多条线路时，按费率升序 failover：便宜的先试，接不通自动切下一条。留空=未设，选路时视为最贵排最后')}
                    >
                      <InputNumber min={0} max={100} step={0.01} precision={2} style={{ width: '100%' }} placeholder={t('如 0.12')} />
                    </Form.Item>
                    <Form.Item
                      name="sort_weight"
                      label={t('排序权重')}
                      style={{ width: 130 }}
                      tooltip={t('同费率线路的平级排序，数字小的排前面（先试）；默认 0')}
                    >
                      <InputNumber min={0} max={999} style={{ width: '100%' }} placeholder="0" />
                    </Form.Item>
                    <Form.Item
                      name="tenant_id"
                      label={t('归属租户 ID')}
                      style={{ width: 130 }}
                      tooltip={t('转售计费：出局通话按该租户账户计费出账（话单带 cc_tenant_id 回中台）；留空=默认账。同一出局字冠组以首选线路的租户为准')}
                    >
                      <InputNumber min={1} max={999999} style={{ width: '100%' }} placeholder={t('默认账')} />
                    </Form.Item>
                  </Space>
                  <Space size={16} style={{ display: 'flex' }} align="start">
                    <Form.Item name="register_transport" label={t('传输协议')} style={{ width: 130 }}>
                      <Select placeholder={t('UDP(默认)')} allowClear>
                        <Select.Option value="udp">UDP</Select.Option>
                        <Select.Option value="tcp">TCP</Select.Option>
                        <Select.Option value="tls">TLS</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item
                      name="ping_seconds"
                      label={t('OPTIONS 保活(秒)')}
                      style={{ width: 150 }}
                      tooltip={t('按间隔发 OPTIONS 探测，失败自动把线路标记为 DOWN；0 关闭')}
                    >
                      <InputNumber min={0} max={3600} style={{ width: '100%' }} placeholder={t('0=关')} />
                    </Form.Item>
                    <Form.Item name="expire_seconds" label={t('注册有效期(秒)')} style={{ width: 150 }}>
                      <InputNumber min={60} max={86400} style={{ width: '100%' }} placeholder="600" />
                    </Form.Item>
                    <Form.Item name="retry_seconds" label={t('失败重试(秒)')} style={{ width: 130 }}>
                      <InputNumber min={5} max={3600} style={{ width: '100%' }} placeholder="30" />
                    </Form.Item>
                    <Form.Item
                      name="max_concurrent"
                      label={t('最大并发')}
                      style={{ width: 130 }}
                      tooltip={t('本线路出局同时最多几路（mod_hash 后端 limit）；超限自动 USER_BUSY 拒接；0=不限')}
                    >
                      <InputNumber min={0} max={500} style={{ width: '100%' }} placeholder={t('0=不限')} />
                    </Form.Item>
                  </Space>
                  <Space size={16} style={{ display: 'flex' }} align="start">
                    <Form.Item
                      name="from_user"
                      label={t('From 用户')}
                      style={{ flex: 1 }}
                      tooltip={t('出局 From 头用户名，空跟随账号')}
                    >
                      <Input maxLength={64} placeholder={t('空=账号')} />
                    </Form.Item>
                    <Form.Item
                      name="from_domain"
                      label={t('From 域')}
                      style={{ flex: 1 }}
                      tooltip={t('出局 From 头域名，空跟随 realm')}
                      rules={[{ pattern: /^$|^[A-Za-z0-9.-]{2,64}(:\d{2,5})?$/, message: t('域名或 IP') }]}
                    >
                      <Input maxLength={64} placeholder={t('空=realm')} />
                    </Form.Item>
                    <Form.Item
                      name="caller_id_in_from"
                      label={t('透传主叫到 From')}
                      valuePropName="checked"
                      tooltip={t('呼入转外呼时把原主叫放进 From 头（需运营商允许）')}
                    >
                      <Switch />
                    </Form.Item>
                  </Space>
                </>
              ),
            }]}
          />

          <Form.Item name="proxy" label={t('Proxy（可选，默认同 realm）')} rules={[hostRule]} style={{ marginTop: 8 }}>
            <Input placeholder={t('出向代理，通常留空')} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('选路预览（LCR）')}
        open={previewOpen}
        onCancel={() => setPreviewOpen(false)}
        footer={<Button onClick={() => setPreviewOpen(false)}>{t('关闭')}</Button>}
        destroyOnHidden
        width={560}
      >
        <div style={{ marginTop: 8 }}>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              placeholder={t('输入完整拨号（含字冠，如 9008613800138000）')}
              value={previewNumber}
              onChange={(e) => setPreviewNumber(e.target.value)}
              onPressEnter={runPreview}
              prefix={<NodeIndexOutlined />}
              maxLength={32}
              allowClear
            />
            <Button type="primary" loading={previewing} onClick={runPreview}>{t('预览')}</Button>
          </Space.Compact>
          <div className="cell-dim" style={{ fontSize: 12, margin: '10px 0' }}>
            {t('先走便宜线路，失败自动切下一条（同费率按排序权重）。仅模拟计算，不发起真实呼叫。')}
          </div>
          {previewMatched === false ? (
            <Empty description={t('没有匹配的出局字冠线路（检查号码字冠是否配了对应线路）')} />
          ) : previewRows.length > 0 ? (
            <Table
              rowKey="order"
              size="small"
              pagination={false}
              dataSource={previewRows}
              columns={[
                {
                  title: t('次序'),
                  dataIndex: 'order',
                  width: 70,
                  render: (v: number) => (
                    <Tag color={v === 1 ? 'green' : 'default'} variant="filled">
                      {v === 1 ? t('首选') : t('第 {{n}} 条', { n: v })}
                    </Tag>
                  ),
                },
                {
                  title: t('线路'),
                  dataIndex: 'gateway',
                  render: (v: string) => <span className="cell-mono">{v}</span>,
                },
                {
                  title: t('费率(元/分)'),
                  dataIndex: 'rate_per_min',
                  width: 110,
                  render: (v: number) =>
                    v && v > 0 ? (v / 100).toFixed(2) : <span className="cell-dim">{t('未设')}</span>,
                },
              ]}
             scroll={{ x: 960 }} />
          ) : null}
        </div>
      </Modal>
    </ListPageShell>
  )
}
