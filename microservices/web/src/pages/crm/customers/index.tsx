import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Card, Col, Descriptions, Drawer, Empty, Form, Input, List, Modal, Pagination, Popconfirm, Radio, Row, Segmented, Select, Skeleton, Space, Tabs, Tag, Tooltip, Typography,
} from 'antd'
import {
  PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined, FlagOutlined, SettingOutlined,
  UserOutlined, EditOutlined, MergeCellsOutlined, DownloadOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { message } from '@/utils/feedback'
import {
  claimCustomer, createContact, createCustomer, createFollowUp, deleteContact, exportCustomers,
  fenToYuan, findDupGroups,
  getCustomerDetail, getCustomerTimeline, getMyScope, listCustomers, mergeCustomers, releaseCustomer, updateCustomer,
  CONTRACT_STATUS_META, STAGE_META,
  type CrmCustomer, type CrmDupGroup, type CustomerDetail,
} from '@/api/crm'
import { listTags } from '@/api/crm/tag'
import type { CrmTag } from '@/api/crm/tag'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import CrmDialButton from '@/components/crm/CrmDialButton'
import { listCcCalls, type CcCall } from '@/api/cc'
import { listVisitorConversations, listMessages, type ImConversation, type ImMessage } from '@/api/im'

const { Text } = Typography

interface SearchParams {
  keyword?: string
  level?: string
  scope?: string
  page: number
  page_size: number
}

const SCOPE_OPTIONS = [
  { label: '全部', value: 'all' },
  { label: '我的', value: 'mine' },
  { label: '今日', value: 'today' },
  { label: '跨期', value: 'overdue' },
  { label: '回访', value: 'revisit' },
  { label: '公海', value: 'pool' },
]

const LEVEL_COLORS: Record<string, string> = { A: 'gold', B: 'blue', C: 'default' }

// 顶部渐变色带：级别定调（A 金 / B 蓝 / C 灰），公海统一青色
const LEVEL_GRADIENTS: Record<string, string> = {
  A: 'linear-gradient(90deg, #f5c518, #fa8c16)',
  B: 'linear-gradient(90deg, #1677ff, #36cfc9)',
  C: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
}
const POOL_GRADIENT = 'linear-gradient(90deg, #13c2c2, #36cfc9)'

function MetricBlock({ value, label, accent }: { value: string | number; label: string; accent?: string }) {
  return (
    <div style={{
      flex: 1, textAlign: 'center', padding: '6px 4px', borderRadius: 8,
      background: 'rgba(128,128,128,0.08)',
    }}>
      <div style={{ fontSize: 16, fontWeight: 600, lineHeight: 1.2, color: accent }}>{value}</div>
      <div style={{ fontSize: 11, opacity: 0.6 }}>{label}</div>
    </div>
  )
}


export default function CrmCustomersPage() {
  const { t } = useTranslation()

  function renderImContent(content: string) {
    try {
      const o = JSON.parse(content)
      if (o.event === 'recalled') return t('（消息已撤回）')
      if (o.event) return t('[系统] {{text}}', { text: o.text || o.event })
      if (o.url) return t('[附件] {{name}}', { name: o.name || o.url })
      return o.text ?? content
    } catch {
      return content
    }
  }

  const [list, setList] = useState<CrmCustomer[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 12 })
  const [exporting, setExporting] = useState(false)
  const [editing, setEditing] = useState<CrmCustomer | null>(null)
  const [creating, setCreating] = useState(false)
  const [detail, setDetail] = useState<CustomerDetail | null>(null)
  const [form] = Form.useForm()
  const [contactForm] = Form.useForm()
  const [followForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const navigate = useNavigate()
  const [myScope, setMyScope] = useState<string>('all')
  const [allTags, setAllTags] = useState<CrmTag[]>([])
  const [dupOpen, setDupOpen] = useState(false)
  const [dupGroups, setDupGroups] = useState<CrmDupGroup[]>([])
  const [dupLoading, setDupLoading] = useState(false)
  // 查重失败必须与空结果分离，避免网络错误被伪装成“未发现重复客户”。
  const [dupError, setDupError] = useState(false)
  // 每组选中的主客户 id（key → primaryId）
  const [primaryPick, setPrimaryPick] = useState<Record<string, number>>({})
  const [merging, setMerging] = useState<string | null>(null)

  useEffect(() => {
    getMyScope().then((r) => setMyScope(r.scope)).catch(() => {})
    listTags().then((r: CrmTag[]) => setAllTags(r ?? [])).catch(() => {})
  }, [])
  const [calls, setCalls] = useState<CcCall[]>([])
  const [imConvs, setImConvs] = useState<ImConversation[]>([])
  const [imMsgs, setImMsgs] = useState<{ conv: ImConversation; messages: ImMessage[] } | null>(null)

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listCustomers({ ...p })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取客户失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void fetchList(params)
  }, [params, fetchList])

  async function openDetail(id: number) {
    try {
      const d = await getCustomerDetail(id)
      setDetail(d)
      setCalls([])
      setImConvs([])
      if ((d.customer.im_visitor_id ?? 0) > 0) {
        listVisitorConversations(d.customer.im_visitor_id!)
          .then((r) => setImConvs(r.list ?? [])).catch(() => {})
      }
      if (d.customer.phone) {
        listCcCalls({ page: 1, page_size: 20, number: d.customer.phone })
          .then((r) => setCalls(r.list ?? [])).catch(() => {})
      }
    } catch {
      message.error(t('加载详情失败'))
    }
  }

  function openEditor(row: CrmCustomer | null) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
      name: row?.name || '', industry: row?.industry || '', level: row?.level || 'C',
      phone: row?.phone || '', address: row?.address || '', note: row?.note || '',
      tag_ids: (row as any).tags?.map((t: CrmTag) => t.id) ?? [],
    })
  }

  async function onSave() {
    const values = await form.validateFields()
    try {
      if (editing) await updateCustomer(editing.id, values)
      else await createCustomer(values)
      message.success(t('已保存'))
      setEditing(null)
      setCreating(false)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  async function onAddContact() {
    if (!detail) return
    const values = await contactForm.validateFields()
    try {
      await createContact(detail.customer.id, values)
      contactForm.resetFields()
      await openDetail(detail.customer.id)
      message.success(t('联系人已添加'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('添加失败'))
    }
  }

  async function onDelContact(id: number) {
    if (!detail) return
    try {
      await deleteContact(id)
      await openDetail(detail.customer.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('删除失败'))
    }
  }

  async function onAddFollow() {
    if (!detail) return
    const values = await followForm.validateFields()
    try {
      await createFollowUp({ target_type: 'customer', target_id: detail.customer.id, content: values.content, next_at: values.next_at })
      followForm.resetFields()
      await openDetail(detail.customer.id)
      message.success(t('跟进已记录'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('记录失败'))
    }
  }

  async function onClaim(row: CrmCustomer) {
    try {
      await claimCustomer(row.id)
      message.success(t('已领取「{{name}}」', { name: row.name }))
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('领取失败'))
    }
  }

  async function onRelease() {
    if (!detail) return
    try {
      await releaseCustomer(detail.customer.id)
      message.success(t('已释放到公海'))
      setDetail(null)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('释放失败'))
    }
  }

  async function openDup() {
    setDupOpen(true)
    setDupLoading(true)
    setDupError(false)
    // 新请求开始前清掉旧结果，失败时只显示错误态，不让陈旧数据冒充本次结果。
    setDupGroups([])
    try {
      const res = await findDupGroups()
      const groups = res.groups ?? []
      setDupGroups(groups)
      // 默认主客户 = 每组第一个（ID 最小，最早建档）
      const pick: Record<string, number> = {}
      for (const g of groups) pick[g.key] = g.customers[0]?.id
      setPrimaryPick(pick)
    } catch {
      setDupError(true)
      message.error(t('查重失败'))
    } finally {
      setDupLoading(false)
    }
  }

  async function onMergeGroup(g: CrmDupGroup) {
    const primary = primaryPick[g.key]
    if (!primary) return
    const sources = g.customers.map((c) => c.id).filter((id) => id !== primary)
    setMerging(g.key)
    try {
      const res = await mergeCustomers(primary, sources)
      message.success(t('已合并 {{n}} 个客户进 #{{id}}', { n: res.merged, id: primary }))
      setDupGroups((prev) => prev.filter((x) => x.key !== g.key))
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('合并失败'))
    } finally {
      setMerging(null)
    }
  }

  return (
    <div className="page-list crm-customers-page">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" initialValues={params}
          onFinish={(v) => setParams({ ...params, page: 1, ...v })}>
          <Form.Item name="keyword">
            <Input placeholder={t('名称/电话')} prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="level">
            <Select placeholder={t('级别')} style={{ width: 100 }} allowClear
              options={['A', 'B', 'C'].map((v) => ({ value: v, label: t('{{v}} 级', { v }) }))} />
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
            <Typography.Title level={5} style={{ margin: 0 }}>{t('客户')}</Typography.Title>
            <Tag variant="filled">{total}</Tag>
            <Tooltip title={t('今日=今天成为我的；跨期=按回收规则已超期未跟进；回访=最新跟进提醒已到期')}>
              <Typography.Text type="secondary" style={{ fontSize: 12, cursor: 'help' }}>{t('视图说明')}</Typography.Text>
            </Tooltip>
          </Space>
          <Space wrap>
            <Segmented
              options={(myScope === 'self' ? SCOPE_OPTIONS.filter((o) => o.value !== 'all') : SCOPE_OPTIONS).map((o) => ({ ...o, label: t(o.label) }))}
              value={params.scope || (myScope === 'self' ? 'mine' : 'all')}
              onChange={(v) => setParams({ ...params, page: 1, scope: v === 'all' ? undefined : String(v) })} />
            <Button icon={<MergeCellsOutlined />} onClick={() => void openDup()}>{t('查重合并')}</Button>
            <Button
              icon={<DownloadOutlined />}
              loading={exporting}
              onClick={() => {
                setExporting(true)
                void exportCustomers({
                  keyword: params.keyword, level: params.level, scope: params.scope,
                }).catch(() => {}).finally(() => setExporting(false))
              }}
            >
              {t('导出')}
            </Button>
            <Button icon={<SettingOutlined />} onClick={() => navigate('/crm/pool-rules')}>{t('回收规则')}</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>{t('新建客户')}</Button>
          </Space>
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
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('暂无客户')} style={{ padding: 48 }} />
        ) : (
          <Row gutter={[16, 16]}>
            {list.map((cu) => {
              const isPool = cu.owner_user_id === 0
              const gradient = isPool ? POOL_GRADIENT : (LEVEL_GRADIENTS[cu.level] || LEVEL_GRADIENTS.C)
              return (
                <Col xs={24} sm={12} lg={8} xl={6} key={cu.id}>
                  <Card
                    hoverable
                    bordered={false}
                    styles={{ body: { padding: 0 } }}
                    style={{ overflow: 'hidden', height: '100%' }}
                    onClick={() => void openDetail(cu.id)}
                  >
                    <div style={{ height: 6, background: gradient }} />
                    <div style={{ padding: '14px 16px 12px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                        <Typography.Text strong style={{ fontSize: 17, lineHeight: 1.3 }} ellipsis={{ tooltip: cu.name }}>
                          {cu.name}
                        </Typography.Text>
                        <Tag color={isPool ? 'cyan' : (LEVEL_COLORS[cu.level] || 'default')} variant="filled" style={{ marginInlineEnd: 0, flexShrink: 0 }}>
                          {isPool ? t('公海') : t('{{v}} 级', { v: cu.level || 'C' })}
                        </Tag>
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 2 }}>
                        {[cu.industry, cu.phone].filter(Boolean).join(' · ') || '—'}
                      </Typography.Text>
                      <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
                        <MetricBlock value={cu.open_opp_count ?? 0} label={t('进行中商机')} accent="var(--c-primary)" />
                        <MetricBlock value={cu.contract_count ?? 0} label={t('合同')} />
                        <MetricBlock value={(cu.contract_cents ?? 0) > 0 ? `¥${fenToYuan(cu.contract_cents)}` : '—'} label={t('签约额')} accent="var(--c-success)" />
                      </div>
                      <div style={{
                        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                        marginTop: 12, paddingTop: 10, borderTop: '1px solid rgba(128,128,128,0.15)',
                      }}>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          <UserOutlined style={{ marginRight: 4 }} />
                          {isPool ? t('待领取') : `#${cu.owner_user_id}`}
                        </Typography.Text>
                        <Space size={0} onClick={(e) => e.stopPropagation()}>
                          {isPool ? (
                            <Button type="link" size="small" icon={<FlagOutlined />} onClick={() => void onClaim(cu)}>{t('领取')}</Button>
                          ) : null}
                          <CrmDialButton phone={cu.phone} label="" />
                          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditor(cu)} />
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
            showSizeChanger showTotal={(n) => t('共 {{n}} 个客户', { n })}
            pageSizeOptions={[8, 12, 24, 48]}
            onChange={(page, page_size) => setParams({ ...params, page, page_size })} />
        </div>
      </Card>

      <Modal title={editing ? t('编辑客户 #{{id}}', { id: editing.id }) : t('新建客户')} open={creating || !!editing}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label={t('客户名称')} rules={[{ required: true }]}><Input /></Form.Item>
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="industry" label={t('行业')} style={{ flex: 1 }}><Input /></Form.Item>
            <Form.Item name="level" label={t('级别')} style={{ width: 120 }}>
              <Select options={['A', 'B', 'C'].map((v) => ({ value: v, label: t('{{v}} 级', { v }) }))} />
            </Form.Item>
          </Space>
          <Form.Item name="phone" label={t('电话')}><Input /></Form.Item>
          <Form.Item name="address" label={t('地址')}><Input /></Form.Item>
          <Form.Item name="note" label={t('备注')}><Input.TextArea rows={2} /></Form.Item>
          <Form.Item name="tag_ids" label={t('标签')}>
            <Select mode="multiple" placeholder={t('选择标签')} allowClear>
              {allTags.map((t) => <Select.Option key={t.id} value={t.id}>
                <Space><span style={{ width: 12, height: 12, borderRadius: 3, background: t.color, display: 'inline-block' }} /><span>{t.name}</span></Space>
              </Select.Option>)}
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Drawer title={detail ? t('客户：{{name}}', { name: detail.customer.name }) : t('客户详情')} open={!!detail}
        onClose={() => setDetail(null)} width={560} destroyOnHidden>
        {detail && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions column={{ xs: 1, sm: 2 }} size="small" bordered>
              <Descriptions.Item label={t('级别')}><Tag color={LEVEL_COLORS[detail.customer.level] || 'default'} variant="filled">{detail.customer.level}</Tag></Descriptions.Item>
              <Descriptions.Item label={t('行业')}>{detail.customer.industry || '—'}</Descriptions.Item>
              <Descriptions.Item label={t('电话')}><span className="cell-mono">{detail.customer.phone || '—'}</span></Descriptions.Item>
              <Descriptions.Item label={t('创建')}>{formatDateTime(detail.customer.created_at)}</Descriptions.Item>
              {detail.customer.note ? <Descriptions.Item label={t('备注')} span={2}>{detail.customer.note}</Descriptions.Item> : null}
            </Descriptions>
            {detail.customer.owner_user_id > 0 ? (
              <Popconfirm title={t('释放后该客户进入公海，其他成员可领取，确认？')} onConfirm={() => void onRelease()}>
                <Button danger>{t('释放到公海')}</Button>
              </Popconfirm>
            ) : null}
            <Tabs
              size="small"
              items={[
                {
                  key: 'contacts', label: t('联系人 {{n}}', { n: detail.contacts.length }),
                  children: (
                    <>
                      <List size="small" dataSource={detail.contacts}
                        locale={{ emptyText: <GlassEmpty text="暂无联系人" compact /> }}
                        renderItem={(ct) => (
                          <List.Item actions={[
                            <Popconfirm key="d" title={t('删除联系人？')} onConfirm={() => void onDelContact(ct.id)}>
                              <Button type="text" size="small" danger icon={<DeleteOutlined />} />
                            </Popconfirm>,
                          ]}>
                            <List.Item.Meta
                              title={<Space size={6}>{ct.name}{ct.is_primary ? <Tag color="gold" variant="filled">{t('主要')}</Tag> : null}{ct.title ? <Text type="secondary">{ct.title}</Text> : null}</Space>}
                              description={<span className="cell-mono" style={{ fontSize: 12 }}>{[ct.phone, ct.email].filter(Boolean).join(' · ') || '—'}</span>} />
                          </List.Item>
                        )} />
                      <Form form={contactForm} layout="inline" style={{ marginTop: 8 }}>
                        <Form.Item name="name" rules={[{ required: true, message: t('姓名') }]}><Input placeholder={t('姓名')} style={{ width: 90 }} /></Form.Item>
                        <Form.Item name="title"><Input placeholder={t('职位')} style={{ width: 90 }} /></Form.Item>
                        <Form.Item name="phone"><Input placeholder={t('电话')} style={{ width: 130 }} /></Form.Item>
                        <Form.Item><Button icon={<PlusOutlined />} onClick={() => void onAddContact()} /></Form.Item>
                      </Form>
                    </>
                  ),
                },
                {
                  key: 'opps', label: t('商机 {{n}}', { n: detail.opportunities.length }),
                  children: (
                    <List size="small" dataSource={detail.opportunities}
                      locale={{ emptyText: <GlassEmpty text="暂无商机（去商机管道创建）" compact /> }}
                      renderItem={(o) => {
                        const meta = STAGE_META[o.stage] ?? { label: o.stage, color: 'default' }
                        return (
                          <List.Item>
                            <List.Item.Meta
                              title={<Space size={6}>{o.name}<Tag color={meta.color} variant="filled">{t(meta.label)}</Tag></Space>}
                              description={`¥${fenToYuan(o.amount_cents)} · ${formatDateTime(o.created_at)}`} />
                          </List.Item>
                        )
                      }} />
                  ),
                },
                {
                  key: 'contracts', label: t('合同 {{n}}', { n: detail.contracts.length }),
                  children: (
                    <List size="small" dataSource={detail.contracts}
                      locale={{ emptyText: <GlassEmpty text="暂无合同" compact /> }}
                      renderItem={(ct) => {
                        const meta = CONTRACT_STATUS_META[ct.status] ?? { label: ct.status, color: 'default' }
                        return (
                          <List.Item>
                            <List.Item.Meta
                              title={<Space size={6}><span className="cell-mono">{ct.no}</span>{ct.title}<Tag color={meta.color} variant="filled">{t(meta.label)}</Tag></Space>}
                              description={`¥${fenToYuan(ct.amount_cents)}`} />
                          </List.Item>
                        )
                      }} />
                  ),
                },
                {
                  key: 'calls', label: t('通话 {{n}}', { n: calls.length }),
                  children: (
                    <>
                      {detail.customer.phone ? (
                        <CrmDialButton phone={detail.customer.phone} label={t('呼叫 {{phone}}', { phone: detail.customer.phone })} size="middle" />
                      ) : <Typography.Text type="secondary">{t('客户未留电话')}</Typography.Text>}
                      <List size="small" style={{ marginTop: 8, maxHeight: 260, overflow: 'auto' }} dataSource={calls}
                        locale={{ emptyText: <GlassEmpty text="暂无通话记录" compact /> }}
                        renderItem={(call) => (
                          <List.Item>
                            <List.Item.Meta
                              title={<Space size={6}>
                                <Tag variant="filled" color={call.direction === 'outbound' ? 'blue' : 'green'}>
                                  {call.direction === 'outbound' ? t('呼出') : t('呼入')}
                                </Tag>
                                <span className="cell-mono" style={{ fontSize: 12 }}>{call.caller} → {call.callee}</span>
                                <Tag variant="filled">{call.status}</Tag>
                                {call.duration_sec ? <Typography.Text type="secondary">{call.duration_sec}s</Typography.Text> : null}
                              </Space>}
                              description={formatDateTime(call.started_at || call.created_at)} />
                          </List.Item>
                        )} />
                    </>
                  ),
                },
                ...((detail.customer.im_visitor_id ?? 0) > 0 ? [{
                  key: 'im', label: t('IM 对话 {{n}}', { n: imConvs.length }),
                  children: (
                    <List size="small" style={{ maxHeight: 300, overflow: 'auto' }} dataSource={imConvs}
                      locale={{ emptyText: <GlassEmpty text="暂无 IM 会话" compact /> }}
                      renderItem={(cv) => (
                        <List.Item style={{ cursor: 'pointer' }}
                          onClick={() => void listMessages(cv.public_id).then((r) => setImMsgs({ conv: cv, messages: r.messages ?? [] })).catch(() => message.error(t('加载消息失败')))}>
                          <List.Item.Meta
                            title={<Space size={6}>
                              <span className="cell-mono" style={{ fontSize: 12 }}>{cv.public_id.slice(0, 8)}</span>
                              <Tag variant="filled">{cv.status}</Tag>
                              {cv.rating_score ? <Tag color="gold" variant="filled">★ {cv.rating_score}</Tag> : null}
                            </Space>}
                            description={`${cv.last_message_preview || '—'} · ${formatDateTime(cv.created_at)}`} />
                        </List.Item>
                      )} />
                  ),
                }] : []),
                {
                  key: 'follow', label: t('跟进 {{n}}', { n: detail.follow_ups.length }),
                  children: (
                    <>
                      <Form form={followForm} layout="vertical">
                        <Form.Item name="content" rules={[{ required: true, message: t('请填写跟进内容') }]}>
                          <Input.TextArea rows={2} placeholder={t('本次跟进内容…')} />
                        </Form.Item>
                        <Space>
                          <Form.Item name="next_at" noStyle><Input placeholder={t('下次跟进（2026-07-25 10:00）')} style={{ width: 240 }} /></Form.Item>
                          <Button type="primary" onClick={() => void onAddFollow()}>{t('记录')}</Button>
                        </Space>
                      </Form>
                      <List size="small" style={{ marginTop: 12, maxHeight: 220, overflow: 'auto' }} dataSource={detail.follow_ups}
                        locale={{ emptyText: <GlassEmpty text="暂无跟进" compact /> }}
                        renderItem={(f) => (
                          <List.Item>
                            <List.Item.Meta
                              title={<Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(f.created_at)}{f.next_at ? t(' · 下次：{{time}}', { time: formatDateTime(f.next_at) }) : ''}</Text>}
                              description={f.content} />
                          </List.Item>
                        )} />
                    </>
                  ),
                },
                {
                  key: 'timeline', label: t('时间线'),
                  children: (
                    <TimelineTab customerId={detail.customer.id} />
                  ),
                },
              ]}
            />
          </Space>
        )}
      </Drawer>

      <Modal title={t('重复客户检测')} open={dupOpen} onCancel={() => setDupOpen(false)} footer={null} width={680} destroyOnHidden>
        {dupLoading ? <Skeleton active paragraph={{ rows: 4 }} /> : dupError ? (
          <Alert
            type="error"
            showIcon
            message={t('查重失败')}
            description={t('无法加载重复客户，请重试。')}
            action={<Button size="small" onClick={() => void openDup()}>{t('重试')}</Button>}
            style={{ margin: 16 }}
          />
        ) : dupGroups.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('未发现重复客户（按号码尾 11 位与同名检测）')} style={{ padding: 32 }} />
        ) : (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('选择每组要保留的主客户，其余客户的联系人 / 商机 / 合同 / 跟进将并入主客户后删除。合并不可撤销。')}
            </Typography.Text>
            {dupGroups.map((g) => (
              <Card key={g.key} size="small" bordered
                title={<Space size={8}>
                  <Tag color={g.kind === 'phone' ? 'blue' : 'purple'} variant="filled">{g.kind === 'phone' ? t('同号码') : t('同名')}</Tag>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{g.key.split(':')[1]}</Typography.Text>
                </Space>}
                extra={
                  <Popconfirm title={t('确认合并该组 {{n}} 个客户？不可撤销', { n: g.customers.length })} onConfirm={() => void onMergeGroup(g)}>
                    <Button type="primary" size="small" loading={merging === g.key}>{t('合并')}</Button>
                  </Popconfirm>
                }>
                <Radio.Group value={primaryPick[g.key]}
                  onChange={(e) => setPrimaryPick((prev) => ({ ...prev, [g.key]: e.target.value }))}
                  style={{ width: '100%' }}>
                  <Space direction="vertical" size={6} style={{ width: '100%' }}>
                    {g.customers.map((cu) => (
                      <Radio key={cu.id} value={cu.id} style={{ width: '100%' }}>
                        <Space size={8} wrap>
                          <Typography.Text strong>#{cu.id} {cu.name}</Typography.Text>
                          {cu.phone ? <span className="cell-mono" style={{ fontSize: 12 }}>{cu.phone}</span> : null}
                          {cu.owner_user_id > 0
                            ? <Tag color="blue" variant="filled">#{cu.owner_user_id}</Tag>
                            : <Tag color="cyan" variant="filled">{t('公海')}</Tag>}
                          {primaryPick[g.key] === cu.id ? <Tag color="green" variant="filled">{t('保留为主')}</Tag> : null}
                        </Space>
                      </Radio>
                    ))}
                  </Space>
                </Radio.Group>
              </Card>
            ))}
          </Space>
        )}
      </Modal>

      <Modal title={imMsgs ? t('IM 会话 {{id}}…', { id: imMsgs.conv.public_id.slice(0, 8) }) : ''} open={!!imMsgs}
        onCancel={() => setImMsgs(null)} footer={null} width={520} destroyOnHidden>
        <List size="small" style={{ maxHeight: 420, overflow: 'auto' }} dataSource={imMsgs?.messages ?? []}
          locale={{ emptyText: <GlassEmpty text="无消息" compact /> }}
          renderItem={(m) => (
            <List.Item>
              <List.Item.Meta
                title={<Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {t(({ visitor: '访客', agent: '坐席', system: '系统', bot: '机器人' })[m.sender_type] ?? '') || m.sender_type} · #{m.seq}
                </Typography.Text>}
                description={renderImContent(m.content)} />
            </List.Item>
          )} />
      </Modal>

    </div>
  )

function TimelineTab({ customerId }: { customerId: number }) {
  const [events, setEvents] = useState<any[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setLoading(true)
    getCustomerTimeline(customerId).then(r => setEvents(r.events ?? [])).catch(() => {}).finally(() => setLoading(false))
  }, [customerId])

  const typeLabel: Record<string, string> = { followup: '跟进', stage_change: '阶段变更', contract: '合同', lead_convert: '转化' }
  const typeColor: Record<string, string> = { followup: 'blue', stage_change: 'purple', contract: 'green', lead_convert: 'orange' }

  return (
    <List size="small" loading={loading} dataSource={events}
      locale={{ emptyText: <GlassEmpty text="暂无事件" compact /> }}
      renderItem={(e) => (
        <List.Item>
          <List.Item.Meta
            avatar={<Tag color={typeColor[e.type]} style={{ margin: 0 }}>{typeLabel[e.type] ?? e.type}</Tag>}
            title={<span style={{ fontSize: 13 }}>{e.content}</span>}
            description={<Text type="secondary" style={{ fontSize: 11 }}>{formatDateTime(e.created_at)}{e.by_user_name ? ' · ' + e.by_user_name : ''}</Text>}
          />
        </List.Item>
      )} />
  )
}
}
