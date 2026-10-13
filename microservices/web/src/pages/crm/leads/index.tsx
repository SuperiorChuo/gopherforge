import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Card, Checkbox, Col, Descriptions, Drawer, Empty, Form, Input, InputNumber, List, Modal, Pagination, Row, Segmented, Select, Skeleton, Space, Tag, Typography,
} from 'antd'
import {
  PlusOutlined, ReloadOutlined, SearchOutlined, SwapOutlined, UserSwitchOutlined, FlagOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  assignLeads, claimLead, convertLead, createFollowUp, createLead, getCrmOrg, listFollowUps,
  listLeads, updateLead, yuanToFen, LEAD_STATUS_META,
  type CrmFollowUp, type CrmLead, type OrgDept, type OrgUser,
  getMyScope,
} from '@/api/crm'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import CrmDialButton from '@/components/crm/CrmDialButton'

const { Text } = Typography

interface SearchParams {
  status?: string
  keyword?: string
  scope?: string
  page: number
  page_size: number
}

// 状态渐变色带（与客户卡片墙同一视觉语言）
const STATUS_GRADIENTS: Record<string, string> = {
  new: 'linear-gradient(90deg, #1677ff, #36cfc9)',
  following: 'linear-gradient(90deg, #f5c518, #fa8c16)',
  converted: 'linear-gradient(90deg, #52c41a, #95de64)',
  invalid: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
}
const SOURCE_LABELS: Record<string, string> = { manual: '手工', call: '来电', im: '在线咨询', import: '导入' }

const SCOPE_OPTIONS = [
  { label: '全部', value: 'all' },
  { label: '未分配', value: 'unassigned' },
  { label: '部门池', value: 'dept' },
  { label: '我的', value: 'mine' },
]

export default function CrmLeadsPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<CrmLead[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 12 })

  const [myScope, setMyScope] = useState<string>('all')

  useEffect(() => {
    getMyScope().then((r) => setMyScope(r.scope)).catch(() => {})
  }, [])
  const [editing, setEditing] = useState<CrmLead | null>(null)
  const [creating, setCreating] = useState(false)
  const [converting, setConverting] = useState<CrmLead | null>(null)
  const [followTarget, setFollowTarget] = useState<CrmLead | null>(null)
  const [detailLead, setDetailLead] = useState<CrmLead | null>(null)
  const [followUps, setFollowUps] = useState<CrmFollowUp[]>([])
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [assignOpen, setAssignOpen] = useState(false)
  const [depts, setDepts] = useState<OrgDept[]>([])
  const [users, setUsers] = useState<OrgUser[]>([])
  const [form] = Form.useForm()
  const [convertForm] = Form.useForm()
  const [followForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const [assignForm] = Form.useForm()

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listLeads({ ...p })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
      setSelectedIds([])
    } catch {
      message.error(t('获取名片失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void fetchList(params)
  }, [params, fetchList])

  useEffect(() => {
    getCrmOrg().then((r) => { setDepts(r.departments ?? []); setUsers(r.users ?? []) }).catch(() => {})
  }, [])

  const userName = useMemo(() => {
    const m = new Map(users.map((u) => [u.id, u.name]))
    return (id: number) => m.get(id) || t('用户 #{{id}}', { id })
  }, [users, t])

  const deptName = useMemo(() => {
    const m = new Map(depts.map((d) => [d.id, d.name]))
    return (id: number) => m.get(id) || t('部门 #{{id}}', { id })
  }, [depts, t])

  function openEditor(row: CrmLead | null) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
      name: row?.name || '', company: row?.company || '', phone: row?.phone || '',
      email: row?.email || '', status: row?.status || 'new', note: row?.note || '',
    })
  }

  async function onSave() {
    const values = await form.validateFields()
    try {
      if (editing) await updateLead(editing.id, values)
      else await createLead(values)
      message.success(t('已保存'))
      setEditing(null)
      setCreating(false)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  async function onConvert() {
    if (!converting) return
    const values = await convertForm.validateFields()
    try {
      const res = await convertLead(converting.id, {
        opportunity_name: values.opportunity_name || '',
        amount_cents: yuanToFen(values.amount_yuan),
      })
      message.success(`${t('已转化为客户「{{name}}」', { name: res.customer.name })}${res.opportunity ? t('并创建商机') : ''}`)
      setConverting(null)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('转化失败'))
    }
  }

  async function openFollow(row: CrmLead) {
    setFollowTarget(row)
    followForm.resetFields()
    try {
      const res = await listFollowUps('lead', row.id)
      setFollowUps(res.list ?? [])
    } catch {
      setFollowUps([])
    }
  }

  async function onAddFollow() {
    if (!followTarget) return
    const values = await followForm.validateFields()
    try {
      await createFollowUp({ target_type: 'lead', target_id: followTarget.id, content: values.content, next_at: values.next_at })
      followForm.resetFields()
      const res = await listFollowUps('lead', followTarget.id)
      setFollowUps(res.list ?? [])
      message.success(t('跟进已记录'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('记录失败'))
    }
  }

  async function onAssign() {
    const values = await assignForm.validateFields()
    if (!values.department_id && !values.owner_user_id) {
      message.warning(t('请选择分配到的部门或成员'))
      return
    }
    try {
      const res = await assignLeads(selectedIds, values.department_id, values.owner_user_id)
      message.success(t('已分配 {{n}} 张名片', { n: res.assigned }))
      setAssignOpen(false)
      assignForm.resetFields()
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('分配失败'))
    }
  }

  async function onClaim(row: CrmLead) {
    try {
      await claimLead(row.id)
      message.success(t('已领取「{{name}}」', { name: row.name }))
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('领取失败'))
    }
  }

  return (
    <div className="page-list crm-leads-page">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" initialValues={params}
          onFinish={(v) => setParams({ ...params, page: 1, ...v })}>
          <Form.Item name="keyword">
            <Input placeholder={t('姓名/公司/电话')} prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 120 }} allowClear
              options={Object.entries(LEAD_STATUS_META).map(([value, m]) => ({ value, label: t(m.label) }))} />
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
            <Typography.Title level={5} style={{ margin: 0 }}>{t('所有名片')}</Typography.Title>
            <Tag variant="filled">{total}</Tag>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('未分配 → 部门池 → 个人；点卡片跟进，勾选批量分配')}
            </Typography.Text>
          </Space>
          <Space wrap>
            <Segmented
              options={(myScope === 'self' ? SCOPE_OPTIONS.filter((o) => o.value !== 'all') : SCOPE_OPTIONS).map((o) => ({ ...o, label: t(o.label) }))}
              value={params.scope || (myScope === 'self' ? 'mine' : 'all')}
              onChange={(v) => setParams({ ...params, page: 1, scope: v === 'all' ? undefined : String(v) })} />
            <Button icon={<UserSwitchOutlined />} disabled={selectedIds.length === 0}
              onClick={() => setAssignOpen(true)}>
              {t('分配（{{n}}）', { n: selectedIds.length })}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>{t('新建名片')}</Button>
          </Space>
        </div>
        {loading ? (
          <Row gutter={[16, 16]}>
            {Array.from({ length: 8 }, (_, i) => (
              <Col xs={24} sm={12} lg={8} xl={6} key={i}>
                <Card bordered={false}><Skeleton active paragraph={{ rows: 2 }} /></Card>
              </Col>
            ))}
          </Row>
        ) : list.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('暂无名片')} style={{ padding: 48 }} />
        ) : (
          <Row gutter={[16, 16]}>
            {list.map((row) => {
              const meta = LEAD_STATUS_META[row.status] ?? { label: row.status, color: 'default' }
              const gradient = STATUS_GRADIENTS[row.status] || STATUS_GRADIENTS.new
              const selectable = row.status !== 'converted'
              const checked = selectedIds.includes(row.id)
              return (
                <Col xs={24} sm={12} lg={8} xl={6} key={row.id}>
                  <Card
                    hoverable
                    bordered={false}
                    styles={{ body: { padding: 0 } }}
                    style={{
                      overflow: 'hidden', height: '100%',
                      outline: checked ? '2px solid var(--c-primary)' : '2px solid transparent',
                      transition: 'outline-color 0.15s',
                    }}
                    onClick={() => { setDetailLead(row); openFollow(row) }}
                  >
                    <div style={{ height: 6, background: gradient }} />
                    <div style={{ padding: '12px 14px 10px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                        <Space size={8} align="start" onClick={(e) => e.stopPropagation()}>
                          {selectable ? (
                            <Checkbox checked={checked} style={{ marginTop: 2 }}
                              onChange={(e) => setSelectedIds((prev) =>
                                e.target.checked ? [...prev, row.id] : prev.filter((x) => x !== row.id))} />
                          ) : null}
                          <div>
                            <Typography.Text strong style={{ fontSize: 16, lineHeight: 1.3 }}>{row.name}</Typography.Text>
                            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
                              {[row.company, t(SOURCE_LABELS[row.source] ?? '') || row.source].filter(Boolean).join(' · ')}
                            </Typography.Text>
                          </div>
                        </Space>
                        <Tag color={meta.color} variant="filled" style={{ marginInlineEnd: 0, flexShrink: 0 }}>{t(meta.label)}</Tag>
                      </div>
                      <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 8 }}>
                        <span className="cell-mono" style={{ fontSize: 13 }}>{row.phone || t('未留电话')}</span>
                        {(row as any).score > 0 && (
                          <Tag color={(row as any).score >= 60 ? 'success' : (row as any).score >= 30 ? 'warning' : 'error'} style={{ fontSize: 11, lineHeight: '18px' }}>
                            {(row as any).score}分
                          </Tag>
                        )}
                      </div>
                      <div style={{
                        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                        marginTop: 10, paddingTop: 8, borderTop: '1px solid rgba(128,128,128,0.15)',
                      }}>
                        <span>
                          {row.owner_user_id > 0
                            ? <Tag color="blue" variant="filled">{userName(row.owner_user_id)}</Tag>
                            : (row.department_id ?? 0) > 0
                              ? <Tag color="purple" variant="filled">{t('{{name}} 池', { name: deptName(row.department_id ?? 0) })}</Tag>
                              : <Tag variant="filled">{t('未分配')}</Tag>}
                        </span>
                        <Space size={0} onClick={(e) => e.stopPropagation()}>
                          {row.owner_user_id === 0 && row.status !== 'converted' ? (
                            <Button type="link" size="small" icon={<FlagOutlined />} onClick={() => void onClaim(row)}>{t('领取')}</Button>
                          ) : null}
                          <CrmDialButton phone={row.phone} label="" />
                          <Button type="link" size="small" onClick={() => openEditor(row)}>{t('编辑')}</Button>
                          <Button type="link" size="small" icon={<SwapOutlined />} disabled={row.status === 'converted'}
                            onClick={() => { setConverting(row); convertForm.setFieldsValue({ opportunity_name: t('{{name}}的商机', { name: row.company || row.name }), amount_yuan: null }) }}>
                            {t('转化')}
                          </Button>
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
            showSizeChanger showTotal={(n) => t('共 {{n}} 张名片', { n })}
            pageSizeOptions={[8, 12, 24, 48]}
            onChange={(page, page_size) => setParams({ ...params, page, page_size })} />
        </div>
      </Card>

      <Modal title={t('批量分配 {{n}} 张名片', { n: selectedIds.length })} open={assignOpen}
        onCancel={() => setAssignOpen(false)} onOk={() => void onAssign()} okText={t('分配')} destroyOnHidden>
        <Text type="secondary">{t('只选部门 → 进部门池待成员领取；选到成员 → 直接归属该成员。')}</Text>
        <Form form={assignForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="department_id" label={t('部门')}>
            <Select allowClear placeholder={t('选择部门（可选）')}
              options={depts.map((d) => ({ value: d.id, label: d.name }))}
              onChange={() => assignForm.setFieldValue('owner_user_id', undefined)} />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(p, c) => p.department_id !== c.department_id}>
            {({ getFieldValue }) => {
              const deptId = getFieldValue('department_id')
              const pool = deptId ? users.filter((u) => u.department_id === deptId) : users
              return (
                <Form.Item name="owner_user_id" label={deptId ? t('成员（限该部门）') : t('成员')}>
                  <Select allowClear showSearch optionFilterProp="label" placeholder={t('选择成员（可选）')}
                    options={pool.map((u) => ({ value: u.id, label: u.name }))} />
                </Form.Item>
              )
            }}
          </Form.Item>
        </Form>
      </Modal>

      <Drawer title={detailLead?.name ?? t('名片详情')} open={!!detailLead} onClose={() => { setDetailLead(null); setFollowTarget(null) }} width={480}>
        {detailLead && (
          <>
            <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label={t('公司')}>{detailLead.company || '—'}</Descriptions.Item>
              <Descriptions.Item label={t('电话')}><span className="cell-mono">{detailLead.phone || '—'}</span></Descriptions.Item>
              <Descriptions.Item label={t('邮箱')}>{detailLead.email || '—'}</Descriptions.Item>
              <Descriptions.Item label={t('来源')}><Tag>{SOURCE_LABELS[detailLead.source] || detailLead.source}</Tag></Descriptions.Item>
              <Descriptions.Item label={t('状态')}><Tag color={LEAD_STATUS_META[detailLead.status]?.color}>{t(LEAD_STATUS_META[detailLead.status]?.label ?? detailLead.status)}</Tag></Descriptions.Item>
              <Descriptions.Item label={t('评分')}>{(detailLead as any).score > 0 ? <Tag color={(detailLead as any).score >= 60 ? 'success' : (detailLead as any).score >= 30 ? 'warning' : 'error'}>{(detailLead as any).score}分</Tag> : '—'}</Descriptions.Item>
              <Descriptions.Item label={t('创建时间')} span={2}>{formatDateTime(detailLead.created_at)}</Descriptions.Item>
              {detailLead.note ? <Descriptions.Item label={t('备注')} span={2}>{detailLead.note}</Descriptions.Item> : null}
            </Descriptions>
            <Space style={{ marginBottom: 16 }}>
              <Button size="small" onClick={() => { openEditor(detailLead); setDetailLead(null) }}>{t('编辑')}</Button>
              <CrmDialButton phone={detailLead.phone} label={t('拨打')} size="small" />
              <Button size="small" type="primary" disabled={detailLead.status === 'converted'} onClick={() => { setConverting(detailLead); convertForm.setFieldsValue({ opportunity_name: t('{{name}}的商机', { name: detailLead.company || detailLead.name }), amount_yuan: null }) }}>{t('转化')}</Button>
              {detailLead.owner_user_id === 0 && detailLead.status !== 'converted' ? <Button size="small" onClick={() => void onClaim(detailLead)}>{t('领取')}</Button> : null}
            </Space>
            <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>{t('跟进记录')}</Typography.Text>
            <List size="small" dataSource={followUps}
              locale={{ emptyText: <GlassEmpty text="暂无跟进" compact /> }}
              renderItem={(f) => (
                <List.Item>
                  <List.Item.Meta
                    title={f.content}
                    description={<Text type="secondary" style={{ fontSize: 11 }}>{formatDateTime(f.created_at)}{f.next_at ? ' · ' + t('下次：{{time}}', { time: formatDateTime(f.next_at) }) : ''}</Text>}
                  />
                </List.Item>
              )} />
            <Form form={followForm} layout="vertical" style={{ marginTop: 12 }}>
              <Form.Item name="content" rules={[{ required: true, message: t('请填写跟进内容') }]}>
                <Input.TextArea rows={2} placeholder={t('跟进内容…')} />
              </Form.Item>
              <Space>
                <Form.Item name="next_at" noStyle><Input placeholder={t('下次跟进时间')} style={{ width: 220 }} /></Form.Item>
                <Button type="primary" onClick={() => void onAddFollow()}>{t('记录')}</Button>
              </Space>
            </Form>
          </>
        )}
      </Drawer>

      <Modal title={editing ? t('编辑名片 #{{id}}', { id: editing.id }) : t('新建名片')} open={creating || !!editing}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label={t('姓名')} rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="company" label={t('公司')}><Input /></Form.Item>
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="phone" label={t('电话')} style={{ flex: 1 }}><Input /></Form.Item>
            <Form.Item name="email" label={t('邮箱')} style={{ flex: 1 }}><Input /></Form.Item>
          </Space>
          {editing && editing.status !== 'converted' ? (
            <Form.Item name="status" label={t('状态')}>
              <Select options={[
                { value: 'new', label: t('未处理') }, { value: 'following', label: t('跟进中') }, { value: 'invalid', label: t('无效') },
              ]} />
            </Form.Item>
          ) : null}
          <Form.Item name="note" label={t('备注')}><Input.TextArea rows={2} /></Form.Item>
        </Form>
      </Modal>

      <Modal title={t('转化名片：{{name}}', { name: converting?.name ?? '' })} open={!!converting}
        onCancel={() => setConverting(null)} onOk={() => void onConvert()} okText={t('转化为客户')} destroyOnHidden>
        <Text type="secondary">{t('将创建客户与主联系人；填写商机名称可同时创建商机进入管道。')}</Text>
        <Form form={convertForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="opportunity_name" label={t('商机名称（可选）')}><Input placeholder={t('留空则只建客户')} /></Form.Item>
          <Form.Item name="amount_yuan" label={t('预计金额（元）')}>
            <InputNumber style={{ width: '100%' }} min={0} precision={2} placeholder="0.00" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title={t('跟进记录：{{name}}', { name: followTarget?.name ?? '' })} open={!!followTarget}
        onCancel={() => setFollowTarget(null)} footer={null} destroyOnHidden>
        <Form form={followForm} layout="vertical">
          <Form.Item name="content" label={t('本次跟进')} rules={[{ required: true, message: t('请填写跟进内容') }]}>
            <Input.TextArea rows={2} placeholder={t('沟通结果、客户反馈…')} />
          </Form.Item>
          <Form.Item name="next_at" label={t('下次跟进时间（可选）')}>
            <Input placeholder={t('2026-07-25 或 2026-07-25 10:00')} />
          </Form.Item>
          <Button type="primary" onClick={() => void onAddFollow()}>{t('记录跟进')}</Button>
        </Form>
        <List size="small" style={{ marginTop: 16, maxHeight: 240, overflow: 'auto' }} dataSource={followUps}
          locale={{ emptyText: <GlassEmpty text="暂无跟进记录" compact /> }}
          renderItem={(f) => (
            <List.Item>
              <List.Item.Meta title={<Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(f.created_at)}{f.next_at ? t(' · 下次：{{time}}', { time: formatDateTime(f.next_at) }) : ''}</Text>}
                description={f.content} />
            </List.Item>
          )} />
      </Modal>
    </div>
  )
}
