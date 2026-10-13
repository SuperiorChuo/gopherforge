import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Card, Col, Drawer, Empty, Form, Input, InputNumber, List, Modal, Pagination, Popconfirm, Progress, Row, Select, Skeleton, Space, Tag, Typography,
} from 'antd'
import { AuditOutlined, PlusOutlined, ReloadOutlined, SearchOutlined, PayCircleOutlined, FileTextOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import request from '@/utils/request'
import {
  addPayment, createContract, fenToYuan, getContract, listContracts, listCustomers,
  listOpportunities, updateContract, yuanToFen, CONTRACT_STATUS_META, PAYMENT_STATUS_META,
  type CrmContract, type CrmCustomer, type CrmOpportunity, type CrmPayment,
} from '@/api/crm'
import { listTodoTasks } from '@/api/bpm'
import BpmInstanceTimeline from '@/components/bpm/BpmInstanceTimeline'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

// 状态渐变色带（与 CRM 各页同视觉语言）
const STATUS_GRADIENTS: Record<string, string> = {
  draft: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
  approving: 'linear-gradient(90deg, #faad14, #ffd666)',
  active: 'linear-gradient(90deg, #1677ff, #36cfc9)',
  void: 'linear-gradient(90deg, #ff4d4f, #ff7875)',
}

// 本地合并 approving（BPM 审批中）状态；BPM 未启用时后端不会产生该状态，展示不受影响
const STATUS_META: Record<string, { label: string; color: string }> = {
  ...CONTRACT_STATUS_META,
  approving: { label: '审批中', color: 'gold' },
}

interface SearchParams {
  keyword?: string
  status?: string
  page: number
  page_size: number
}

export default function CrmContractsPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<CrmContract[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 12 })
  const [customers, setCustomers] = useState<CrmCustomer[]>([])
  const [opps, setOpps] = useState<CrmOpportunity[]>([])
  const [creating, setCreating] = useState(false)
  const [detail, setDetail] = useState<{ contract: CrmContract; payments: CrmPayment[]; paid_cents: number } | null>(null)
  const [paying, setPaying] = useState(false)
  // BPM 审批流可用性探测：探测失败（服务未部署/接口 404/无权限）时整页隐藏审批相关 UI，不影响现有功能
  const [bpmAvailable, setBpmAvailable] = useState(false)
  // 当前详情合同的审批时间线可见性（查无实例时由组件回调隐藏区块）
  const [detailBpmVisible, setDetailBpmVisible] = useState(true)
  const [submittingApproval, setSubmittingApproval] = useState<number | null>(null)
  const [form] = Form.useForm()
  const [payForm] = Form.useForm()
  const [searchForm] = Form.useForm()

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listContracts({ ...p })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取合同失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void fetchList(params)
  }, [params, fetchList])

  useEffect(() => {
    listCustomers({ page: 1, page_size: 100 }).then((r) => setCustomers(r.list ?? [])).catch(() => {})
    // 静默探测 BPM 服务：成功即认为审批功能可用
    listTodoTasks({ page: 1, page_size: 1 }, true)
      .then(() => setBpmAvailable(true))
      .catch(() => setBpmAvailable(false))
  }, [])

  const customerName = (id: number) => customers.find((c) => c.id === id)?.name || t('客户 #{{id}}', { id })

  async function onCustomerChange(customerId: number) {
    form.setFieldValue('opportunity_id', undefined)
    try {
      const res = await listOpportunities({ customer_id: customerId, stage: 'won', page: 1, page_size: 50 })
      setOpps(res.list ?? [])
    } catch {
      setOpps([])
    }
  }

  async function onCreate() {
    const values = await form.validateFields()
    try {
      await createContract({
        no: values.no, title: values.title, customer_id: values.customer_id,
        opportunity_id: values.opportunity_id || undefined,
        amount_cents: yuanToFen(values.amount_yuan),
        status: values.status || 'draft', signed_at: values.signed_at || undefined, note: values.note,
      })
      message.success(t('合同已创建'))
      setCreating(false)
      form.resetFields()
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建失败'))
    }
  }

  async function openDetail(id: number) {
    try {
      setDetailBpmVisible(true)
      setDetail(await getContract(id))
    } catch {
      message.error(t('加载失败'))
    }
  }

  // 提交审批：走 CRM 后端（表单快照由业务后端权威生成，前端不直接调 bpm 发起接口）
  async function onSubmitApproval(contract: CrmContract) {
    setSubmittingApproval(contract.id)
    try {
      await request.post(`/api/v1/crm/contracts/${contract.id}/submit-approval`, undefined, { silent: true })
      message.success(t('已提交审批，可在合同详情查看审批进度'))
      if (detail?.contract.id === contract.id) await openDetail(contract.id)
      void fetchList(params)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : ''
      // 接口不存在/服务未部署 → 判定 BPM 不可用并隐藏入口，避免反复报错
      if (msg.includes('404') || msg.toLowerCase().includes('not found')) {
        setBpmAvailable(false)
        message.warning(t('审批服务暂不可用'))
      } else {
        message.error(msg || t('提交审批失败'))
      }
    } finally {
      setSubmittingApproval(null)
    }
  }

  async function onSetStatus(status: string) {
    if (!detail) return
    try {
      await updateContract(detail.contract.id, { status })
      message.success(t('状态已更新'))
      await openDetail(detail.contract.id)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('更新失败'))
    }
  }

  async function onAddPayment() {
    if (!detail) return
    const values = await payForm.validateFields()
    setPaying(true)
    try {
      const res = await addPayment(detail.contract.id, {
        amount_cents: yuanToFen(values.amount_yuan),
        paid_at: values.paid_at || undefined, method: values.method || 'transfer', note: values.note,
      })
      // BPM 回款审批（M2）：响应带 pending_approval 表示进入审批流，确认后才计入统计
      if (res?.status === 'pending_approval') {
        message.success(t('回款已提交审批，确认后计入统计'))
      } else {
        message.success(t('回款已登记'))
      }
      payForm.resetFields()
      await openDetail(detail.contract.id)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('登记失败'))
    } finally {
      setPaying(false)
    }
  }

  return (
    <div className="page-list crm-contracts-page">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" initialValues={params}
          onFinish={(v) => setParams({ ...params, page: 1, ...v })}>
          <Form.Item name="keyword">
            <Input placeholder={t('编号/标题')} prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 110 }} allowClear
              options={Object.entries(bpmAvailable ? STATUS_META : CONTRACT_STATUS_META).map(([value, m]) => ({ value, label: t(m.label) }))} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.resetFields(); setParams({ page: 1, page_size: 10 }) }}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12, marginBottom: 16 }}>
          <Space size={10}>
            <Typography.Title level={5} style={{ margin: 0 }}>{t('合同与回款')}</Typography.Title>
            <Tag variant="filled">{total}</Tag>
          </Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>{t('新建合同')}</Button>
        </div>
        {loading ? (
          <Row gutter={[16, 16]}>
            {Array.from({ length: 8 }, (_, i) => (
              <Col xs={24} sm={12} lg={8} xl={6} key={i}>
                <Card bordered={false}><Skeleton active paragraph={{ rows: 3 }} /></Card>
              </Col>
            ))}
          </Row>
        ) : list.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('暂无合同')} style={{ padding: 48 }} />
        ) : (
          <Row gutter={[16, 16]}>
            {list.map((row) => {
              const meta = STATUS_META[row.status] ?? { label: row.status, color: 'default' }
              const paid = row.paid_cents ?? 0
              const pct = row.amount_cents > 0 ? Math.round((paid / row.amount_cents) * 100) : 0
              return (
                <Col xs={24} sm={12} lg={8} xl={6} key={row.id}>
                  <Card
                    hoverable bordered={false}
                    styles={{ body: { padding: 0 } }}
                    style={{ overflow: 'hidden', height: '100%' }}
                    onClick={() => void openDetail(row.id)}
                  >
                    <div style={{ height: 6, background: STATUS_GRADIENTS[row.status] || STATUS_GRADIENTS.draft }} />
                    <div style={{ padding: '14px 16px 12px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                        <div style={{ minWidth: 0 }}>
                          <Typography.Text strong style={{ fontSize: 16, lineHeight: 1.3, display: 'block' }} ellipsis={{ tooltip: row.title }}>
                            {row.title}
                          </Typography.Text>
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            <FileTextOutlined style={{ marginRight: 4 }} />
                            <span className="cell-mono">{row.no}</span>
                          </Typography.Text>
                        </div>
                        <Tag color={meta.color} variant="filled" style={{ marginInlineEnd: 0, flexShrink: 0 }}>{t(meta.label)}</Tag>
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                        {customerName(row.customer_id)}
                      </Typography.Text>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginTop: 10 }}>
                        <Typography.Text strong style={{ fontSize: 18, color: 'var(--c-success)' }}>¥{fenToYuan(row.amount_cents)}</Typography.Text>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>{t('已回 ¥{{amount}}', { amount: fenToYuan(paid) })}</Typography.Text>
                      </div>
                      <Progress percent={pct} size="small" style={{ marginTop: 4 }}
                        status={pct >= 100 ? 'success' : row.status === 'active' ? 'active' : 'normal'} />
                      <div style={{
                        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                        marginTop: 8, paddingTop: 8, borderTop: '1px solid rgba(128,128,128,0.15)',
                      }}>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(row.created_at)}</Typography.Text>
                        <Space size={8}>
                          {bpmAvailable && row.status === 'draft' && (
                            <Button
                              type="link" size="small" icon={<AuditOutlined />}
                              style={{ padding: 0, height: 'auto', fontSize: 12 }}
                              loading={submittingApproval === row.id}
                              onClick={(e) => { e.stopPropagation(); void onSubmitApproval(row) }}
                            >
                              {t('提交审批')}
                            </Button>
                          )}
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>{t('回款 {{n}}%', { n: pct })}</Typography.Text>
                        </Space>
                      </div>
                    </div>
                  </Card>
                </Col>
              )
            })}
          </Row>
        )}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
          <Pagination total={total} current={params.page} pageSize={params.page_size}
            showSizeChanger showTotal={(n) => t('共 {{n}} 份合同', { n })}
            pageSizeOptions={[8, 12, 24, 48]}
            onChange={(page, page_size) => setParams({ ...params, page, page_size })} />
        </div>
      </Card>

      <Modal title={t('新建合同')} open={creating} onCancel={() => setCreating(false)} onOk={() => void onCreate()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="no" label={t('合同编号')} rules={[{ required: true }]} style={{ flex: 1 }}>
              <Input placeholder="HT-2026-001" />
            </Form.Item>
            <Form.Item name="status" label={t('状态')} style={{ width: 120 }} initialValue="draft">
              <Select options={Object.entries(CONTRACT_STATUS_META).filter(([v]) => v !== 'void').map(([value, m]) => ({ value, label: t(m.label) }))} />
            </Form.Item>
          </Space>
          <Form.Item name="title" label={t('合同标题')} rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="customer_id" label={t('客户')} rules={[{ required: true, message: t('请选择客户') }]}>
            <Select showSearch optionFilterProp="label" placeholder={t('选择客户')}
              onChange={(v) => void onCustomerChange(v)}
              options={customers.map((c) => ({ value: c.id, label: c.name }))} />
          </Form.Item>
          <Form.Item name="opportunity_id" label={t('关联商机（可选，选赢单商机自动带金额）')}>
            <Select allowClear placeholder={t('该客户的赢单商机')}
              options={opps.map((o) => ({ value: o.id, label: `${o.name}（¥${fenToYuan(o.amount_cents)}）` }))} />
          </Form.Item>
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="amount_yuan" label={t('合同金额（元）')} style={{ flex: 1 }}>
              <InputNumber style={{ width: '100%' }} min={0} precision={2} placeholder={t('关联商机可留空')} />
            </Form.Item>
            <Form.Item name="signed_at" label={t('签约日期')} style={{ flex: 1 }}>
              <Input placeholder="2026-07-18" />
            </Form.Item>
          </Space>
          <Form.Item name="note" label={t('备注')}><Input.TextArea rows={2} /></Form.Item>
        </Form>
      </Modal>

      <Drawer title={detail ? t('合同 {{no}}', { no: detail.contract.no }) : ''} open={!!detail}
        onClose={() => setDetail(null)} width={480} destroyOnHidden>
        {detail && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <div>
              <div style={{ fontWeight: 600, fontSize: 16 }}>{detail.contract.title}</div>
              <Text type="secondary">{customerName(detail.contract.customer_id)}</Text>
            </div>
            <Space size={12} wrap>
              <Tag color={(STATUS_META[detail.contract.status] ?? { color: 'default' }).color} variant="filled">
                {t((STATUS_META[detail.contract.status] ?? { label: detail.contract.status }).label)}
              </Tag>
              <Text strong>{t('合同额 ¥{{amount}}', { amount: fenToYuan(detail.contract.amount_cents) })}</Text>
              <Text type="success">{t('已回 ¥{{amount}}', { amount: fenToYuan(detail.paid_cents) })}</Text>
            </Space>
            <Progress
              percent={detail.contract.amount_cents > 0 ? Math.round((detail.paid_cents / detail.contract.amount_cents) * 100) : 0} />
            {detail.contract.status === 'approving' ? (
              <Alert type="warning" showIcon message={t('合同审批中，通过前不可编辑或变更状态')} />
            ) : (
              <Space wrap>
                {bpmAvailable && detail.contract.status === 'draft' ? (
                  <Popconfirm
                    title={t('提交审批？')}
                    description={t('提交后合同进入审批流，审批期间不可编辑')}
                    onConfirm={() => void onSubmitApproval(detail.contract)}
                  >
                    <Button type="primary" icon={<AuditOutlined />} loading={submittingApproval === detail.contract.id}>
                      {t('提交审批')}
                    </Button>
                  </Popconfirm>
                ) : null}
                {detail.contract.status === 'draft' ? (
                  <Button type={bpmAvailable ? 'default' : 'primary'} onClick={() => void onSetStatus('active')}>{t('生效')}</Button>
                ) : null}
                {detail.contract.status !== 'void' ? (
                  <Button danger onClick={() => void onSetStatus('void')}>{t('作废')}</Button>
                ) : null}
              </Space>
            )}

            {bpmAvailable && detailBpmVisible && (
              <Card size="small" title={<><AuditOutlined /> {t('审批进度')}</>} bordered={false}>
                <BpmInstanceTimeline
                  key={`${detail.contract.id}-${detail.contract.status}`}
                  bizType="crm_contract"
                  bizId={detail.contract.id}
                  showHeader
                  onUnavailable={() => setDetailBpmVisible(false)}
                />
              </Card>
            )}

            <Card size="small" title={<><PayCircleOutlined /> {t('登记回款')}</>} bordered={false}>
              <Form form={payForm} layout="vertical">
                <Space size={12} style={{ display: 'flex' }}>
                  <Form.Item name="amount_yuan" label={t('金额（元）')} rules={[{ required: true }]} style={{ flex: 1 }}>
                    <InputNumber style={{ width: '100%' }} min={0.01} precision={2} />
                  </Form.Item>
                  <Form.Item name="method" label={t('方式')} initialValue="transfer" style={{ width: 110 }}>
                    <Select options={[
                      { value: 'transfer', label: t('转账') }, { value: 'cash', label: t('现金') }, { value: 'other', label: t('其他') },
                    ]} />
                  </Form.Item>
                </Space>
                <Space size={12} style={{ display: 'flex' }}>
                  <Form.Item name="paid_at" label={t('回款日期')} style={{ flex: 1 }}>
                    <Input placeholder={t('默认今天')} />
                  </Form.Item>
                  <Form.Item name="note" label={t('备注')} style={{ flex: 1 }}><Input /></Form.Item>
                </Space>
                <Button type="primary" loading={paying} disabled={detail.contract.status !== 'active'}
                  onClick={() => void onAddPayment()}>
                  {detail.contract.status === 'active' ? t('登记回款') : t('合同生效后可回款')}
                </Button>
              </Form>
            </Card>

            <List size="small" header={<Text strong>{t('回款记录（{{n}}）', { n: detail.payments.length })}</Text>}
              dataSource={detail.payments}
              locale={{ emptyText: <GlassEmpty text="暂无回款" compact /> }}
              renderItem={(p) => {
                // 回款审批状态（M2）：空值为未走审批的存量，按已确认兼容展示
                const statusMeta = PAYMENT_STATUS_META[p.status || 'confirmed'] ?? PAYMENT_STATUS_META.confirmed
                return (
                  <List.Item>
                    <List.Item.Meta
                      title={<Space size={8}><Text strong>¥{fenToYuan(p.amount_cents)}</Text>
                        <Tag variant="filled">{t(({ transfer: '转账', cash: '现金', other: '其他' })[p.method] ?? '') || p.method}</Tag>
                        <Tag color={statusMeta.color}>{t(statusMeta.label)}</Tag></Space>}
                      description={`${formatDateTime(p.paid_at)}${p.note ? ` · ${p.note}` : ''}`} />
                  </List.Item>
                )
              }} />
          </Space>
        )}
      </Drawer>
    </div>
  )
}
