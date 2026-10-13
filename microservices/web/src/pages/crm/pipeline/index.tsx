import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Card, Drawer, Form, Input, InputNumber, Modal, Segmented, Select, Space, Steps, Tag, Timeline, Typography,
} from 'antd'
import { PlusOutlined, ReloadOutlined, DollarOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  advanceStage, createOpportunity, fenToYuan, getOpportunity, listCustomers, listOpportunities,
  yuanToFen, STAGE_META,
  type CrmCustomer, type CrmOpportunity, type CrmStageLog,
} from '@/api/crm'
import TableToolbar from '@/components/common/TableToolbar'
import { useAppSelector } from '@/hooks/store'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

// 看板列顺序：4 个进行中阶段 + 终态两列
const BOARD_STAGES = ['initial', 'demand', 'proposal', 'negotiation', 'won', 'lost'] as const
const FLOW_STAGES = ['initial', 'demand', 'proposal', 'negotiation'] as const

// 列头渐变（与客户卡片墙同一视觉语言）
const STAGE_GRADIENTS: Record<string, string> = {
  initial: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
  demand: 'linear-gradient(90deg, #1677ff, #36cfc9)',
  proposal: 'linear-gradient(90deg, #722ed1, #b37feb)',
  negotiation: 'linear-gradient(90deg, #f5c518, #fa8c16)',
  won: 'linear-gradient(90deg, #52c41a, #95de64)',
  lost: 'linear-gradient(90deg, #ff4d4f, #ff7875)',
}

export default function CrmPipelinePage() {
  const { t } = useTranslation()
  const [opps, setOpps] = useState<CrmOpportunity[]>([])
  const [customers, setCustomers] = useState<CrmCustomer[]>([])
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [detail, setDetail] = useState<{ opportunity: CrmOpportunity; stage_logs: CrmStageLog[] } | null>(null)
  const [advancing, setAdvancing] = useState<CrmOpportunity | null>(null)
  const [form] = Form.useForm()
  const [stageForm] = Form.useForm()
  const me = useAppSelector((st) => st.auth.userInfo)
  const myId = me?.id
  const [mineOnly, setMineOnly] = useState(false)
  // 拖拽状态：正在拖的商机 + 悬停中的目标列
  const [dragging, setDragging] = useState<CrmOpportunity | null>(null)
  const [dragOver, setDragOver] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listOpportunities({ page: 1, page_size: 100, owner_user_id: mineOnly && myId ? myId : undefined })
      setOpps(res.list ?? [])
    } catch {
      message.error(t('获取商机失败'))
    } finally {
      setLoading(false)
    }
  }, [mineOnly, myId, t])

  useEffect(() => {
    void load()
    listCustomers({ page: 1, page_size: 100 }).then((r) => setCustomers(r.list ?? [])).catch(() => {})
  }, [load])

  const customerName = useCallback(
    (id: number) => customers.find((c) => c.id === id)?.name || t('客户 #{{id}}', { id }),
    [customers, t],
  )

  const board = useMemo(() => {
    const map: Record<string, CrmOpportunity[]> = {}
    for (const st of BOARD_STAGES) map[st] = []
    for (const o of opps) (map[o.stage] ?? (map[o.stage] = [])).push(o)
    return map
  }, [opps])

  async function onCreate() {
    const values = await form.validateFields()
    try {
      await createOpportunity({
        customer_id: values.customer_id, name: values.name,
        amount_cents: yuanToFen(values.amount_yuan),
        expected_close_at: values.expected_close_at || undefined, note: values.note,
      })
      message.success(t('商机已创建'))
      setCreating(false)
      form.resetFields()
      void load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建失败'))
    }
  }

  async function openDetail(o: CrmOpportunity) {
    try {
      setDetail(await getOpportunity(o.id))
    } catch {
      message.error(t('加载失败'))
    }
  }

  async function onAdvance() {
    if (!advancing) return
    const values = await stageForm.validateFields()
    try {
      await advanceStage(advancing.id, values)
      message.success(t('阶段已更新'))
      setAdvancing(null)
      void load()
      if (detail?.opportunity.id === advancing.id) setDetail(await getOpportunity(advancing.id))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('更新失败'))
    }
  }

  function openAdvance(o: CrmOpportunity, targetStage?: string) {
    setAdvancing(o)
    const idx = FLOW_STAGES.indexOf(o.stage as (typeof FLOW_STAGES)[number])
    const next = targetStage ?? (idx >= 0 && idx < FLOW_STAGES.length - 1 ? FLOW_STAGES[idx + 1] : 'won')
    stageForm.setFieldsValue({ stage: next, note: '', lost_reason: '' })
  }

  // 拖拽换阶段：drop 后仍弹推进确认（保留备注/输单原因留痕，不做静默推进）
  function onDropTo(stage: string) {
    setDragOver(null)
    const o = dragging
    setDragging(null)
    if (!o || o.stage === stage) return
    if (o.stage === 'won' || o.stage === 'lost') {
      message.warning(t('终态商机不可再推进'))
      return
    }
    openAdvance(o, stage)
  }

  const stageValue = (st: string) => board[st]?.reduce((sum, o) => sum + o.amount_cents, 0) ?? 0
  const isTerminalStage = (st: string) => st === 'won' || st === 'lost'

  return (
    <div className="page-list crm-pipeline-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar title="商机管道" total={opps.length}
          description="拖拽卡片到目标列即可推进阶段（弹窗确认留痕）；赢单后可在合同回款页签约"
          extra={
            <Space>
              <Segmented options={[{ label: t('全部'), value: 'all' }, { label: t('我的'), value: 'mine' }]}
                value={mineOnly ? 'mine' : 'all'} onChange={(v) => setMineOnly(v === 'mine')} />
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>{t('新建商机')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>{t('刷新')}</Button>
            </Space>
          } />
        <div style={{ display: 'flex', gap: 12, overflowX: 'auto', paddingTop: 8, alignItems: 'stretch' }}>
          {BOARD_STAGES.map((st) => {
            const meta = STAGE_META[st]
            const items = board[st] ?? []
            const isTerminal = st === 'won' || st === 'lost'
            const droppable = !!dragging && dragging.stage !== st && !isTerminalStage(dragging.stage)
            const hovering = dragOver === st && droppable
            return (
              <div
                key={st}
                style={{
                  minWidth: 220, flex: 1, borderRadius: 12, overflow: 'hidden',
                  background: hovering ? 'rgba(99,102,241,0.14)' : 'rgba(255,255,255,0.025)',
                  border: `1px solid ${hovering ? 'rgba(129,140,248,0.6)' : 'rgba(148,163,184,0.12)'}`,
                  backdropFilter: 'blur(12px)',
                  WebkitBackdropFilter: 'blur(12px)',
                  boxShadow: hovering ? '0 8px 24px rgba(99,102,241,0.2)' : 'inset 0 1px 0 rgba(255,255,255,0.04)',
                  transition: 'all 0.2s cubic-bezier(0.22, 1, 0.36, 1)',
                  display: 'flex', flexDirection: 'column',
                }}
                onDragOver={(e) => { if (droppable) { e.preventDefault(); setDragOver(st) } }}
                onDragLeave={(e) => { if (e.currentTarget === e.target) setDragOver((v) => (v === st ? null : v)) }}
                onDrop={(e) => { e.preventDefault(); onDropTo(st) }}
              >
                <div style={{ height: 4, background: STAGE_GRADIENTS[st] }} />
                <div style={{ padding: '12px 14px 14px', flex: 1, display: 'flex', flexDirection: 'column' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                    <Space size={6}>
                      <Text strong style={{ fontSize: 13.5 }}>{t(meta.label)}</Text>
                      <Tag variant="filled" style={{ marginInlineEnd: 0, borderRadius: 999, padding: '0 8px', fontSize: 11.5 }}>{items.length}</Tag>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 12, fontVariantNumeric: 'tabular-nums' }}>¥{fenToYuan(stageValue(st))}</Text>
                  </div>
                  <Space direction="vertical" size={10} style={{ width: '100%', flex: 1 }}>
                    {items.length === 0 ? (
                      <div style={{
                        border: '1px dashed rgba(148,163,184,0.2)', borderRadius: 10,
                        padding: '24px 0', textAlign: 'center', opacity: 0.6,
                        background: 'rgba(255,255,255,0.01)',
                      }}>
                        <Text type="secondary" style={{ fontSize: 12 }}>{droppable ? t('拖到这里') : t('暂无商机')}</Text>
                      </div>
                    ) : (
                      items.map((o) => (
                        <Card
                          key={o.id} size="small" hoverable
                          draggable={!isTerminal}
                          onDragStart={(e) => { setDragging(o); e.dataTransfer.effectAllowed = 'move' }}
                          onDragEnd={() => { setDragging(null); setDragOver(null) }}
                          onClick={() => void openDetail(o)}
                          style={{
                            cursor: isTerminal ? 'pointer' : 'grab',
                            opacity: dragging?.id === o.id ? 0.35 : 1,
                            borderRadius: 10,
                            border: '1px solid rgba(148,163,184,0.16)',
                            boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
                            transition: 'all 0.2s ease',
                          }}
                        >
                          <div style={{ fontWeight: 600, fontSize: 13.5, marginBottom: 4, color: 'rgba(226,232,240,0.95)' }}>{o.name}</div>
                          <Text type="secondary" style={{ fontSize: 12, display: 'block' }}>{customerName(o.customer_id)}</Text>
                          <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8, alignItems: 'center' }}>
                            <Text strong style={{ fontSize: 13.5, color: '#38bdf8' }}><DollarOutlined /> {fenToYuan(o.amount_cents)}</Text>
                            {!isTerminal ? (
                              <Button size="small" type="link" style={{ padding: '0 4px', fontSize: 12 }}
                                onClick={(e) => { e.stopPropagation(); openAdvance(o) }}>
                                {t('推进')}
                              </Button>
                            ) : (
                              <Text type="secondary" style={{ fontSize: 12 }}>{o.probability}%</Text>
                            )}
                          </div>
                          {!isTerminal && o.probability > 0 ? (
                            <div style={{ marginTop: 8, height: 4, borderRadius: 2, background: 'rgba(148,163,184,0.15)', overflow: 'hidden' }}>
                              <div style={{ width: `${o.probability}%`, height: '100%', background: STAGE_GRADIENTS[st], borderRadius: 2 }} />
                            </div>
                          ) : null}
                        </Card>
                      ))
                    )}
                  </Space>
                </div>
              </div>
            )
          })}
        </div>
      </Card>

      <Modal title={t('新建商机')} open={creating} onCancel={() => setCreating(false)} onOk={() => void onCreate()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Form.Item name="customer_id" label={t('客户')} rules={[{ required: true, message: t('请选择客户') }]}>
            <Select showSearch optionFilterProp="label" placeholder={t('选择客户（先在客户管理创建）')}
              options={customers.map((c) => ({ value: c.id, label: c.name }))} />
          </Form.Item>
          <Form.Item name="name" label={t('商机名称')} rules={[{ required: true }]}><Input /></Form.Item>
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="amount_yuan" label={t('预计金额（元）')} style={{ flex: 1 }}>
              <InputNumber style={{ width: '100%' }} min={0} precision={2} />
            </Form.Item>
            <Form.Item name="expected_close_at" label={t('预计成交日')} style={{ flex: 1 }}>
              <Input placeholder="2026-08-31" />
            </Form.Item>
          </Space>
          <Form.Item name="note" label={t('备注')}><Input.TextArea rows={2} /></Form.Item>
        </Form>
      </Modal>

      <Modal title={t('推进阶段：{{name}}', { name: advancing?.name ?? '' })} open={!!advancing}
        onCancel={() => setAdvancing(null)} onOk={() => void onAdvance()} destroyOnHidden>
        <Form form={stageForm} layout="vertical">
          <Form.Item name="stage" label={t('目标阶段')} rules={[{ required: true }]}>
            <Select options={BOARD_STAGES.filter((s) => s !== advancing?.stage).map((s) => ({
              value: s, label: t(STAGE_META[s].label),
            }))} />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(p, c) => p.stage !== c.stage}>
            {({ getFieldValue }) =>
              getFieldValue('stage') === 'lost' ? (
                <Form.Item name="lost_reason" label={t('输单原因')} rules={[{ required: true, message: t('请填写输单原因') }]}>
                  <Input placeholder={t('价格 / 竞品 / 需求消失…')} />
                </Form.Item>
              ) : null
            }
          </Form.Item>
          <Form.Item name="note" label={t('备注')}><Input placeholder={t('推进说明（计入阶段日志）')} /></Form.Item>
        </Form>
      </Modal>

      <Drawer title={detail ? t('商机：{{name}}', { name: detail.opportunity.name }) : ''} open={!!detail}
        onClose={() => setDetail(null)} width={460} destroyOnHidden>
        {detail && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Space size={8} wrap>
              <Tag color={(STAGE_META[detail.opportunity.stage] ?? { color: 'default' }).color} variant="filled">
                {t((STAGE_META[detail.opportunity.stage] ?? { label: detail.opportunity.stage }).label)}
              </Tag>
              <Text strong>¥{fenToYuan(detail.opportunity.amount_cents)}</Text>
              <Text type="secondary">{t('赢率 {{n}}%', { n: detail.opportunity.probability })}</Text>
            </Space>
            <div>
              <Text type="secondary">{t('客户：')}</Text>{customerName(detail.opportunity.customer_id)}
              {detail.opportunity.expected_close_at ? (
                <div><Text type="secondary">{t('预计成交：')}</Text>{formatDateTime(detail.opportunity.expected_close_at)}</div>
              ) : null}
              {detail.opportunity.lost_reason ? (
                <div><Text type="secondary">{t('输单原因：')}</Text>{detail.opportunity.lost_reason}</div>
              ) : null}
            </div>
            <Steps size="small" direction="vertical"
              current={FLOW_STAGES.indexOf(detail.opportunity.stage as (typeof FLOW_STAGES)[number])}
              items={FLOW_STAGES.map((s) => ({ title: t(STAGE_META[s].label) }))} />
            <div>
              <Text strong>{t('阶段日志')}</Text>
              <Timeline style={{ marginTop: 12 }}
                items={detail.stage_logs.map((l) => ({
                  children: (
                    <div>
                      <Text>{l.from_stage ? `${t((STAGE_META[l.from_stage] ?? { label: l.from_stage }).label)} → ` : ''}
                        {t((STAGE_META[l.to_stage] ?? { label: l.to_stage }).label)}</Text>
                      {l.note ? <div><Text type="secondary" style={{ fontSize: 12 }}>{l.note}</Text></div> : null}
                      <div><Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(l.created_at)}</Text></div>
                    </div>
                  ),
                }))} />
            </div>
            {detail.opportunity.stage !== 'won' && detail.opportunity.stage !== 'lost' ? (
              <Button type="primary" block onClick={() => openAdvance(detail.opportunity)}>{t('推进阶段')}</Button>
            ) : null}
          </Space>
        )}
      </Drawer>
    </div>
  )
}
