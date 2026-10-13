import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Button, Card, DatePicker, Form, Input, InputNumber, Modal,
  Space, Statistic, Switch, Table, Tabs, Tag, Tooltip, Typography, Grid } from 'antd'
import {
  PlusOutlined, ReloadOutlined, PayCircleOutlined, DeleteOutlined, EditOutlined,
  DownloadOutlined, FileDoneOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { type Dayjs } from 'dayjs'
import { message } from '@/utils/feedback'
import {
  listCcRates, upsertCcRate, deleteCcRate, getCcAccount, rechargeCcAccount,
  listCcCharges, ccFenToYuan, ccYuanToFen,
  listCcAccountTxns, listCcBills, exportCcBill,
  listCcAccountsOverview, adminRechargeCcAccount, adminSetCcAccountStatus, adminGenerateCcBills,
  type CcRate, type CcAccountResult, type CcCallCharge, type CcAccountTxn,
  type CcBill, type CcAccountOverviewRow,
} from '@/api/cc'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'

const { Text, Paragraph } = Typography
const { RangePicker } = DatePicker

// 时长「N分N秒」，0 显示 —
function formatDuration(sec: number, t: TFunction) {
  if (!sec) return '—'
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m > 0 ? t('{{m}}分{{s}}秒', { m, s }) : t('{{s}}秒', { s })
}

// ---- 账户页签 ----
function AccountTab() {
  const { t } = useTranslation()
  const [data, setData] = useState<CcAccountResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [recharging, setRecharging] = useState(false)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await getCcAccount())
    } catch {
      message.error(t('获取账户信息失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { void load() }, [load])

  const handleRecharge = async (values: { amount_yuan: number; remark?: string }) => {
    try {
      await rechargeCcAccount(ccYuanToFen(values.amount_yuan), values.remark?.trim())
      message.success(t('操作成功'))
      setRecharging(false)
      form.resetFields()
      void load()
    } catch {
      message.error(t('操作失败'))
    }
  }

  const acc = data?.account
  const frozen = acc?.status === 'frozen'

  return (
    <Card bordered={false} loading={loading}>
      <Space size={48} wrap style={{ marginBottom: 24 }}>
        <Statistic
          title={<Space>{t('账户余额（元）')}{frozen && <Tag color="error">{t('已冻结')}</Tag>}</Space>}
          value={Number(((acc?.balance ?? 0) / 100).toFixed(2))}
          precision={2}
          prefix="¥"
          valueStyle={{ color: (acc?.balance ?? 0) < 0 ? 'var(--c-error-strong)' : 'var(--c-success)', fontSize: 32 }}
        />
        <Statistic title={t('透支额度（元）')} value={ccFenToYuan(acc?.credit_limit)} prefix="¥" />
        <Statistic title={t('本月消费（元）')} value={ccFenToYuan(data?.month_spent)} prefix="¥" />
        <Statistic title={t('今日消费（元）')} value={ccFenToYuan(data?.today_spent)} prefix="¥" />
      </Space>
      <div>
        <Space>
          <Button type="primary" icon={<PayCircleOutlined />} onClick={() => setRecharging(true)}>{t('充值 / 调减')}</Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
        </Space>
        <Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          {t('充值填正数、调减填负数（如 -50 表示扣减 50 元）。可用额 = 余额 + 透支额度，可用额 ≤ 0 时外呼被拦截（402）。')}
        </Paragraph>
      </div>

      <Modal
        title={t('充值 / 调减')}
        open={recharging}
        onCancel={() => { setRecharging(false); form.resetFields() }}
        onOk={() => form.submit()}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleRecharge}>
          <Form.Item
            name="amount_yuan"
            label={t('金额（元，正=充值 / 负=调减）')}
            rules={[
              { required: true, message: t('请输入金额') },
              {
                validator: (_, v) =>
                  v === 0 ? Promise.reject(new Error(t('金额不能为 0'))) : Promise.resolve(),
              },
            ]}
          >
            <InputNumber style={{ width: '100%' }} precision={2} placeholder={t('如 100 或 -50')} />
          </Form.Item>
          <Form.Item name="remark" label={t('备注（进资金流水，便于对账）')}>
            <Input maxLength={256} placeholder={t('如 2026-07 预存款 / 误充冲正')} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

// ---- 资金流水页签 ----
function TxnsTab() {
  const { t } = useTranslation()
  const [list, setList] = useState<CcAccountTxn[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCcAccountTxns({ page, page_size: pageSize })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取资金流水失败'))
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, t])

  useEffect(() => { void load() }, [load])

  const columns: ColumnsType<CcAccountTxn> = [
    { title: t('时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('类型'),
      dataIndex: 'kind',
      width: 90,
      render: (v: string) =>
        v === 'recharge'
          ? <Tag color="green" variant="filled">{t('充值')}</Tag>
          : <Tag color="orange" variant="filled">{t('调减')}</Tag>,
    },
    {
      title: t('金额（元）'),
      dataIndex: 'amount',
      width: 130,
      render: (v: number) => (
        <Text type={v >= 0 ? 'success' : 'warning'}>{v >= 0 ? '+' : '-'}¥{ccFenToYuan(Math.abs(v))}</Text>
      ),
    },
    {
      title: t('完成后余额（元）'),
      dataIndex: 'balance_after',
      width: 150,
      render: (v: number) => <span className="cell-mono">¥{ccFenToYuan(v)}</span>,
    },
    {
      title: t('操作人'),
      dataIndex: 'operator_id',
      width: 100,
      render: (v: number) => (v ? <span className="cell-mono">#{v}</span> : <span className="cell-dim">{t('系统')}</span>),
    },
    { title: t('备注'), dataIndex: 'remark', render: (v: string) => v || <span className="cell-dim">—</span> },
  ]

  return (
    <Card bordered={false}>
      <TableToolbar
        title="资金流水"
        total={total}
        extra={<Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>}
      />
      <Paragraph type="secondary" style={{ marginTop: -8, marginBottom: 12 }}>
        {t('每笔充值/调减的审计留痕；话费扣减见「话费流水」页签。')}
      </Paragraph>
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        scroll={{ x: 800 }}
        locale={{ emptyText: <GlassEmpty text="暂无资金流水" compact /> }}
        pagination={{
          total, current: page, pageSize, showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
      />
    </Card>
  )
}

// ---- 账单页签（平台看全部，租户看自己） ----
function BillsTab({ isPlatform }: { isPlatform: boolean }) {
  const { t } = useTranslation()
  const [list, setList] = useState<CcBill[]>([])
  const [loading, setLoading] = useState(false)
  const [period, setPeriod] = useState<Dayjs | null>(null)
  const [generating, setGenerating] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCcBills(period?.format('YYYY-MM'))
      setList(res.list ?? [])
    } catch {
      message.error(t('获取账单失败'))
    } finally {
      setLoading(false)
    }
  }, [period, t])

  useEffect(() => { void load() }, [load])

  const handleGenerate = async () => {
    const p = (period ?? dayjs().subtract(1, 'month')).format('YYYY-MM')
    setGenerating(true)
    try {
      const res = await adminGenerateCcBills(p)
      message.success(t('账期 {{period}} 出账完成：{{n}} 个租户', { period: p, n: res.generated }))
      void load()
    } catch {
      message.error(t('出账失败'))
    } finally {
      setGenerating(false)
    }
  }

  const columns: ColumnsType<CcBill> = [
    { title: t('账期'), dataIndex: 'period', width: 110, render: (v: string) => <Tag color="geekblue" variant="filled">{v}</Tag> },
    ...(isPlatform
      ? [{ title: t('租户'), dataIndex: 'tenant_id', width: 90, render: (v: number) => <span className="cell-mono">#{v}</span> } as ColumnsType<CcBill>[number]]
      : []),
    { title: t('计费通话数'), dataIndex: 'call_count', width: 110 },
    { title: t('计费时长'), dataIndex: 'total_bill_sec', width: 120, render: (v: number) => formatDuration(v, t) },
    {
      title: t('账单金额（元）'),
      dataIndex: 'total_amount',
      width: 140,
      render: (v: number) => <Text strong>¥{ccFenToYuan(v)}</Text>,
    },
    { title: t('生成时间'), dataIndex: 'generated_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      render: (_: unknown, b: CcBill) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'export',
              label: t('导出明细'),
              icon: <DownloadOutlined />,
              onClick: () => { void exportCcBill(b).catch(() => message.error(t('导出失败'))) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <Card bordered={false}>
      <TableToolbar
        title="月账单"
        total={list.length}
        extra={
          <Space>
            <DatePicker picker="month" value={period} onChange={setPeriod} allowClear placeholder={t('全部账期')} />
            {isPlatform && (
              <Tooltip title={t('按所选账期（默认上月）汇总各租户话费流水出账；重复出账按最新流水覆盖重算')}>
                <Button type="primary" icon={<FileDoneOutlined />} loading={generating} onClick={() => void handleGenerate()}>
                  {t('生成账单')}
                </Button>
              </Tooltip>
            )}
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
          </Space>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
        scroll={{ x: 1000 }}
        locale={{ emptyText: <GlassEmpty text={isPlatform ? '暂无账单，选账期点「生成账单」出账' : '暂无账单，等待平台出账'} compact /> }}
      />
    </Card>
  )
}

// ---- 租户账务总览页签（仅平台管理员） ----
function OverviewTab() {
  const { t } = useTranslation()
  const [list, setList] = useState<CcAccountOverviewRow[]>([])
  const [loading, setLoading] = useState(false)
  const [rechargeRow, setRechargeRow] = useState<CcAccountOverviewRow | null>(null)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCcAccountsOverview()
      setList(res.list ?? [])
    } catch {
      message.error(t('获取租户总览失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { void load() }, [load])

  const handleAdminRecharge = async (values: { amount_yuan: number; remark?: string }) => {
    if (!rechargeRow) return
    try {
      await adminRechargeCcAccount(rechargeRow.tenant_id, ccYuanToFen(values.amount_yuan), values.remark?.trim())
      message.success(t('租户 {{n}} 操作成功', { n: rechargeRow.tenant_id }))
      setRechargeRow(null)
      form.resetFields()
      void load()
    } catch {
      message.error(t('操作失败'))
    }
  }

  const handleToggleFreeze = async (row: CcAccountOverviewRow, freeze: boolean) => {
    try {
      await adminSetCcAccountStatus(row.tenant_id, freeze ? 'frozen' : 'normal')
      message.success(freeze ? t('租户 {{n}} 已冻结（外呼拦截）', { n: row.tenant_id }) : t('租户 {{n}} 已解冻', { n: row.tenant_id }))
      void load()
    } catch {
      message.error(t('操作失败'))
    }
  }

  const columns: ColumnsType<CcAccountOverviewRow> = [
    { title: t('租户'), dataIndex: 'tenant_id', width: 80, render: (v: number) => <Tag color="geekblue" variant="filled">#{v}</Tag> },
    {
      title: t('余额（元）'),
      dataIndex: 'balance',
      width: 130,
      render: (v: number) => (
        <span className="cell-mono" style={{ color: v < 0 ? 'var(--c-error-strong)' : undefined }}>¥{ccFenToYuan(v)}</span>
      ),
    },
    { title: t('透支额度（元）'), dataIndex: 'credit_limit', width: 130, render: (v: number) => `¥${ccFenToYuan(v)}` },
    {
      title: t('可用额（元）'),
      width: 130,
      render: (_: unknown, r: CcAccountOverviewRow) => {
        const avail = r.balance + r.credit_limit
        return avail <= 0
          ? <Text type="danger" strong>¥{ccFenToYuan(avail)}</Text>
          : <span className="cell-mono">¥{ccFenToYuan(avail)}</span>
      },
    },
    { title: t('本月消费（元）'), dataIndex: 'month_spent', width: 130, render: (v: number) => `¥${ccFenToYuan(v)}` },
    { title: t('今日消费（元）'), dataIndex: 'today_spent', width: 130, render: (v: number) => `¥${ccFenToYuan(v)}` },
    {
      title: t('冻结'),
      dataIndex: 'status',
      width: 90,
      render: (v: string, r: CcAccountOverviewRow) => (
        <Tooltip title={v === 'frozen' ? t('已冻结：该租户外呼被拦截（403）') : t('冻结后该租户外呼被拦截')}>
          <Switch checked={v === 'frozen'} onChange={(checked) => void handleToggleFreeze(r, checked)} />
        </Tooltip>
      ),
    },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      render: (_: unknown, r: CcAccountOverviewRow) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'recharge',
              label: t('充值 / 调减'),
              icon: <PayCircleOutlined />,
              onClick: () => setRechargeRow(r),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <Card bordered={false}>
      <TableToolbar
        title="租户账务总览"
        total={list.length}
        extra={<Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>}
      />
      <Paragraph type="secondary" style={{ marginTop: -8, marginBottom: 12 }}>
        {t('发生过账务动作（充值/入账）的租户才会出现在这里。可用额 ≤ 阈值的租户每小时站内信预警（每租户每天一条）。')}
      </Paragraph>
      <Table
        rowKey="tenant_id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
        scroll={{ x: 1100 }}
        locale={{ emptyText: <GlassEmpty text="暂无租户账户" compact /> }}
      />

      <Modal
        title={rechargeRow ? t('租户 {{n}}：充值 / 调减', { n: rechargeRow.tenant_id }) : ''}
        open={!!rechargeRow}
        onCancel={() => { setRechargeRow(null); form.resetFields() }}
        onOk={() => form.submit()}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleAdminRecharge}>
          <Form.Item
            name="amount_yuan"
            label={t('金额（元，正=充值 / 负=调减）')}
            rules={[
              { required: true, message: t('请输入金额') },
              {
                validator: (_, v) =>
                  v === 0 ? Promise.reject(new Error(t('金额不能为 0'))) : Promise.resolve(),
              },
            ]}
          >
            <InputNumber style={{ width: '100%' }} precision={2} placeholder={t('如 100 或 -50')} />
          </Form.Item>
          <Form.Item name="remark" label={t('备注（进该租户资金流水）')}>
            <Input maxLength={256} placeholder={t('如 对公转账入账')} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

// ---- 费率页签 ----
function RatesTab() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [list, setList] = useState<CcRate[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<CcRate | null>(null)
  const [open, setOpen] = useState(false)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCcRates()
      setList(res.list ?? [])
    } catch {
      message.error(t('获取费率失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { void load() }, [load])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ billing_step: 60, connect_fee_yuan: 0 })
    setOpen(true)
  }

  const openEdit = (r: CcRate) => {
    setEditing(r)
    form.setFieldsValue({
      prefix: r.prefix,
      name: r.name,
      rate_per_min_yuan: r.rate_per_min / 100,
      billing_step: r.billing_step,
      connect_fee_yuan: r.connect_fee / 100,
    })
    setOpen(true)
  }

  const handleSubmit = async (values: {
    prefix: string
    name?: string
    rate_per_min_yuan: number
    billing_step: number
    connect_fee_yuan: number
  }) => {
    try {
      await upsertCcRate({
        prefix: values.prefix,
        name: values.name,
        rate_per_min: ccYuanToFen(values.rate_per_min_yuan),
        billing_step: values.billing_step,
        connect_fee: ccYuanToFen(values.connect_fee_yuan),
      })
      message.success(t('保存成功'))
      setOpen(false)
      void load()
    } catch {
      message.error(t('保存失败'))
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteCcRate(id)
      message.success(t('已删除'))
      void load()
    } catch {
      message.error(t('删除失败'))
    }
  }

  const columns: ColumnsType<CcRate> = [
    { title: t('被叫前缀'), dataIndex: 'prefix', width: 120, render: (v: string) => <span className="cell-mono">{v}</span> },
    { title: t('名称'), dataIndex: 'name', render: (v: string) => v || '—' },
    { title: t('每分钟（元）'), dataIndex: 'rate_per_min', width: 130, render: (v: number) => `¥${ccFenToYuan(v)}` },
    { title: t('计费步长（秒）'), dataIndex: 'billing_step', width: 130 },
    { title: t('接通费（元）'), dataIndex: 'connect_fee', width: 120, render: (v: number) => `¥${ccFenToYuan(v)}` },
    {
      title: t('操作'),
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      render: (_: unknown, r: CcRate) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(r) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除该费率？'),
              onClick: () => { void handleDelete(r.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <Card bordered={false}>
      <TableToolbar
        title="费率表"
        total={list.length}
        extra={
          <Space>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建费率')}</Button>
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
          </Space>
        }
      />
      <Paragraph type="secondary" style={{ marginTop: -8, marginBottom: 12 }}>
        {t('被叫按')}<Text strong>{t('最长前缀')}</Text>{t('匹配（如 00 优先于 0）；未命中任何费率 = 免费不计费、不拦截。')}
      </Paragraph>
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
        scroll={{ x: 800 }}
        locale={{ emptyText: <GlassEmpty text="暂无费率，新建后外呼才会计费" compact /> }}
      />

      <Modal
        title={editing ? t('编辑费率 {{prefix}}', { prefix: editing.prefix }) : t('新建费率')}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="prefix"
            label={t('被叫前缀')}
            rules={[
              { required: true, message: t('请输入前缀') },
              { pattern: /^\d{1,8}$/, message: t('前缀为 1-8 位数字') },
            ]}
          >
            <Input placeholder={t('如 0（国内长途）、00（国际）、1（手机）')} disabled={!!editing} />
          </Form.Item>
          <Form.Item name="name" label={t('名称')}>
            <Input placeholder={t('如 国内长途')} />
          </Form.Item>
          <Form.Item
            name="rate_per_min_yuan"
            label={t('每分钟费率（元）')}
            rules={[{ required: true, message: t('请输入费率') }]}
          >
            <InputNumber style={{ width: '100%' }} min={0} precision={2} placeholder={t('如 0.15')} />
          </Form.Item>
          <Form.Item
            name="billing_step"
            label={t('计费步长（秒）')}
            rules={[{ required: true, message: t('请输入步长') }]}
            tooltip={t('不足一步进按一步计。常见 60（整分钟）或 6（6 秒）')}
          >
            <InputNumber style={{ width: '100%' }} min={1} precision={0} placeholder="60" />
          </Form.Item>
          <Form.Item name="connect_fee_yuan" label={t('接通费（元）')} initialValue={0}>
            <InputNumber style={{ width: '100%' }} min={0} precision={2} placeholder="0" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

// ---- 话费流水页签 ----
function ChargesTab() {
  const { t } = useTranslation()
  const [list, setList] = useState<CcCallCharge[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCcCharges({
        page,
        page_size: pageSize,
        from: range?.[0]?.startOf('day').format('YYYY-MM-DD HH:mm:ss'),
        to: range?.[1]?.endOf('day').format('YYYY-MM-DD HH:mm:ss'),
      })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取流水失败'))
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, range, t])

  useEffect(() => { void load() }, [load])

  const columns: ColumnsType<CcCallCharge> = [
    { title: t('时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
    { title: t('主叫'), dataIndex: 'caller', width: 140, render: (v: string) => <span className="cell-mono">{v || '—'}</span> },
    { title: t('被叫'), dataIndex: 'callee', width: 140, render: (v: string) => <span className="cell-mono">{v || '—'}</span> },
    { title: t('时长'), dataIndex: 'bill_sec', width: 100, render: (v: number) => formatDuration(v, t) },
    {
      title: t('金额（元）'),
      dataIndex: 'amount',
      width: 120,
      render: (v: number) => <Text type="danger">-¥{ccFenToYuan(v)}</Text>,
    },
    {
      title: t('扣后余额（元）'),
      dataIndex: 'balance_after',
      width: 140,
      render: (v: number) => <span className="cell-mono" style={{ color: v < 0 ? 'var(--c-error-strong)' : undefined }}>¥{ccFenToYuan(v)}</span>,
    },
  ]

  return (
    <Card bordered={false}>
      <TableToolbar
        title="话费流水"
        total={total}
        extra={
          <Space>
            <RangePicker
              value={range}
              onChange={(v) => { setRange(v as [Dayjs, Dayjs] | null); setPage(1) }}
              allowClear
            />
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
          </Space>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        scroll={{ x: 900 }}
        locale={{ emptyText: <GlassEmpty text="暂无话费流水" compact /> }}
        pagination={{
          total,
          current: page,
          pageSize,
          showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
      />
    </Card>
  )
}

export default function CcBillingPage() {
  const { t } = useTranslation()
  // 平台租户探测：总览接口 200=平台管理员（显示总览页签与出账按钮），403=普通租户。
  const [isPlatform, setIsPlatform] = useState(false)
  useEffect(() => {
    listCcAccountsOverview().then(() => setIsPlatform(true)).catch(() => setIsPlatform(false))
  }, [])

  return (
    <div className="page-list cc-billing-page">
      <Tabs
        defaultActiveKey="account"
        items={[
          { key: 'account', label: t('账户'), children: <AccountTab /> },
          { key: 'rates', label: t('费率'), children: <RatesTab /> },
          { key: 'charges', label: t('话费流水'), children: <ChargesTab /> },
          { key: 'txns', label: t('资金流水'), children: <TxnsTab /> },
          { key: 'bills', label: t('月账单'), children: <BillsTab isPlatform={isPlatform} /> },
          ...(isPlatform
            ? [{ key: 'overview', label: t('租户总览'), children: <OverviewTab /> }]
            : []),
        ]}
      />
    </div>
  )
}
