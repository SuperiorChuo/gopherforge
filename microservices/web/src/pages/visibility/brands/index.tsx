import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Drawer, Form, Input, Modal, Select, Space, Spin, Table, Tag, Typography, Grid } from 'antd'
import { AuditOutlined, BulbOutlined, PlusOutlined, ReloadOutlined, TeamOutlined, EditOutlined, DeleteOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import AiMarkdown from '@/components/common/AiMarkdown'
import {
  analyzeBrandNaming, batchCreateBrands, createBrand, deleteBrand, generateBrands, listBrands, updateBrand,
  type BrandCandidate, type NamingAnalysis, type VisBrand,
} from '@/api/visibility'

const { Text, Paragraph } = Typography

export default function VisibilityBrandsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [list, setList] = useState<VisBrand[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [kind, setKind] = useState<string | undefined>()
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<VisBrand | null>(null)
  const [form] = Form.useForm()
  // AI 识别品牌/竞品
  const [genOpen, setGenOpen] = useState(false)
  const [genForm] = Form.useForm()
  const [generating, setGenerating] = useState(false)
  const [savingGen, setSavingGen] = useState(false)
  const [candidates, setCandidates] = useState<BrandCandidate[]>([])
  const [selectedGen, setSelectedGen] = useState<number[]>([])
  // 命名分析
  const [namingOpen, setNamingOpen] = useState(false)
  const [analyzing, setAnalyzing] = useState(false)
  const [naming, setNaming] = useState<NamingAnalysis | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listBrands({ kind, page, page_size: 20 })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('加载品牌失败'))
    } finally {
      setLoading(false)
    }
  }, [kind, page, t])

  useEffect(() => {
    void load()
  }, [load])

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ kind: 'brand' })
    setOpen(true)
  }

  function openEdit(row: VisBrand) {
    setEditing(row)
    // 先重置：后端 omitempty 字段（aliases/website/note）为空时响应里没有该 key，
    // 只 setFieldsValue 会残留上一次编辑的值并被保存进当前记录
    form.resetFields()
    form.setFieldsValue({
      name: row.name,
      kind: row.kind,
      aliases: row.aliases ?? '',
      website: row.website ?? '',
      note: row.note ?? '',
    })
    setOpen(true)
  }

  async function onSubmit() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editing) {
        await updateBrand(editing.id, values)
        message.success(t('已更新'))
      } else {
        await createBrand(values)
        message.success(t('已创建'))
      }
      setOpen(false)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  function openGen() {
    setCandidates([])
    setSelectedGen([])
    genForm.resetFields()
    setGenOpen(true)
  }

  async function onGenerate() {
    const values = await genForm.validateFields().catch(() => null)
    if (!values) return
    setGenerating(true)
    try {
      const res = await generateBrands({
        site_url: values.site_url?.trim() || undefined,
        hint: values.hint?.trim() || undefined,
      })
      const items = res.items ?? []
      setCandidates(items)
      setSelectedGen(items.map((_, i) => i))
      if (res.site_fetched === false) {
        message.warning(t('网页内容抓取失败（服务器访问不了该站点），已根据网址和 AI 已有认知识别，结果可能不够准，请核对后再入库'))
      }
      if (items.length === 0) message.info(t('AI 这次没识别出新品牌，补充说明后再试试'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('AI 识别失败'))
    } finally {
      setGenerating(false)
    }
  }

  function editCandidate(idx: number, patch: Partial<BrandCandidate>) {
    setCandidates((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  async function onSaveGen() {
    const picks = selectedGen
      .map((i) => candidates[i])
      .filter((it): it is BrandCandidate => Boolean(it && it.name.trim()))
    if (picks.length === 0) {
      message.warning(t('先勾选要加入的品牌'))
      return
    }
    setSavingGen(true)
    try {
      const res = await batchCreateBrands(picks)
      message.success(res.skipped > 0
        ? t('已加入 {{a}} 个，跳过 {{b}} 个重复', { a: res.created, b: res.skipped })
        : t('已加入 {{n}} 个', { n: res.created }))
      setGenOpen(false)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入失败'))
    } finally {
      setSavingGen(false)
    }
  }

  async function runNamingAnalysis() {
    setAnalyzing(true)
    try {
      const res = await analyzeBrandNaming()
      setNaming(res)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('命名分析失败'))
    } finally {
      setAnalyzing(false)
    }
  }

  function openNaming() {
    setNamingOpen(true)
    // analyzing 也要挡：分析中关掉抽屉再打开会并发触发第二次 150 秒长调用
    if (!naming && !analyzing) void runNamingAnalysis()
  }

  const riskTag = (r: string) =>
    r === 'high' ? <Tag color="red" variant="filled">{t('高风险')}</Tag>
    : r === 'low' ? <Tag variant="filled">{t('低风险')}</Tag>
    : <Tag color="orange" variant="filled">{t('中风险')}</Tag>

  const columns: ColumnsType<VisBrand> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (v, r) => (
        <Space>
          <span>{v}</span>
          <Tag color={r.kind === 'competitor' ? 'orange' : 'blue'} variant="filled">
            {r.kind === 'competitor' ? t('竞品') : t('品牌')}
          </Tag>
        </Space>
      ),
    },
    { title: t('其他叫法'), dataIndex: 'aliases', ellipsis: true, render: (v) => v || '—' },
    {
      title: t('网站'),
      dataIndex: 'website',
      ellipsis: true,
      render: (v?: string) => (v ? <a href={v} target="_blank" rel="noreferrer">{v}</a> : '—'),
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, r) => (
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
              confirm: t('删除该品牌？'),
              onClick: () => {
                void deleteBrand(r.id).then(() => {
                  // 删掉末页最后一条后回退一页，避免停在超范围的空页
                  if (list.length === 1 && page > 1) setPage(page - 1)
                  else void load()
                })
              },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <div className="page-list">
      <div className="list-main-card" style={{ padding: 16 }}>
        <TableToolbar
          title="品牌与竞品"
          icon={<TeamOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #6366f1)"
          glow="rgba(99, 102, 241, 0.35)"
          description="记录你的品牌和竞争对手，AI 提及检测时按这些名字识别"
          extra={
            <Space>
              <Select
                allowClear
                placeholder={t('类型')}
                style={{ width: 120 }}
                value={kind}
                onChange={(v) => { setKind(v); setPage(1) }}
                options={[
                  { label: t('自有品牌'), value: 'brand' },
                  { label: t('竞品'), value: 'competitor' },
                ]}
              />
              <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
              <Button icon={<AuditOutlined />} onClick={openNaming}>{t('命名分析')}</Button>
              <Button icon={<BulbOutlined />} onClick={openGen}>{t('AI 识别')}</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增')}</Button>
            </Space>
          }
        />
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="暂无品牌，可点「初始化演示数据」或新增" /> }}
          pagination={{ current: page, total, pageSize: 20, onChange: setPage }}
         scroll={{ x: 960 }} />
      </div>

      <Drawer
        title={t('品牌命名分析')}
        open={namingOpen}
        onClose={() => setNamingOpen(false)}
        width={620}
        extra={
          <Button size="small" loading={analyzing} onClick={() => void runNamingAnalysis()}>{t('重新分析')}</Button>
        }
      >
        {analyzing && !naming ? (
          <Space direction="vertical" align="center" style={{ width: '100%', padding: '48px 0' }}>
            <Spin />
            <Text type="secondary">{t('AI 正在体检你的品牌名：撞名风险、检测数据、改名候选……约需 1 分钟')}</Text>
          </Space>
        ) : null}
        {naming ? (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <div>
              <Text strong>{t('总体判断')}</Text>
              <div style={{ marginTop: 8 }}><AiMarkdown content={naming.summary} /></div>
            </div>
            <div>
              <Text strong>{t('相似撞名检测')}</Text>
              {(naming.collisions?.length ?? 0) > 0 ? (
                <Table
                  style={{ marginTop: 8 }}
                  rowKey="name"
                  size="small"
                  pagination={false}
                  columns={[
                    { title: t('名称'), dataIndex: 'name', width: 150 },
                    { title: t('风险'), dataIndex: 'risk', width: 90, render: (v) => riskTag(v as string) },
                    { title: t('原因'), dataIndex: 'reason' },
                  ]}
                  dataSource={naming.collisions ?? []}
                 scroll={{ x: 960 }} />
              ) : (
                <Alert style={{ marginTop: 8 }} type="success" showIcon message={t('没有发现明显撞名，名字辨识度不错')} />
              )}
            </div>
            <div>
              <Text strong>{t('改名候选')}</Text>
              <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
                {(naming.suggestions ?? []).map((s) => (
                  <div key={s.name} style={{ border: '1px solid rgba(148,163,184,.2)', borderRadius: 8, padding: '8px 12px' }}>
                    <Text strong copyable>{s.name}</Text>
                    <div><Text type="secondary">{s.reason}</Text></div>
                  </div>
                ))}
                {(naming.suggestions?.length ?? 0) === 0 ? <Text type="secondary">{t('AI 认为现名无需更换，未给出候选。')}</Text> : null}
              </Space>
            </div>
          </Space>
        ) : null}
        {!analyzing && !naming ? (
          <Text type="secondary">{t('分析失败或还没开始，点右上角「重新分析」。')}</Text>
        ) : null}
      </Drawer>

      <Modal
        title={t('AI 识别品牌与竞品')}
        open={genOpen}
        onCancel={() => setGenOpen(false)}
        width={720}
        destroyOnHidden
        footer={[
          <Button key="cancel" onClick={() => setGenOpen(false)}>{t('取消')}</Button>,
          <Button key="gen" icon={<BulbOutlined />} loading={generating} onClick={() => void onGenerate()}>
            {candidates.length > 0 ? t('重新识别') : t('开始识别')}
          </Button>,
          <Button key="save" type="primary" loading={savingGen} disabled={selectedGen.length === 0} onClick={() => void onSaveGen()}>
            {t('加入选中（{{n}}）', { n: selectedGen.length })}
          </Button>,
        ]}
      >
        <Form form={genForm} layout="vertical">
          <Form.Item
            name="site_url"
            label={t('网址（可选）')}
            rules={[{ pattern: /^https?:\/\/.+/i, message: t('以 http:// 或 https:// 开头') }]}
            help={t('填了就抓取网页内容，识别站点自身品牌 + 同类竞品；不填则根据已录入的自有品牌推荐竞品')}
          >
            <Input allowClear placeholder={t('例：https://github.com/你的账号/你的项目')} />
          </Form.Item>
          <Form.Item name="hint" label={t('行业/受众补充说明（可选）')} help={t('填了识别更准，例如目标客户、地域、开源/商业')}>
            <Input.TextArea rows={2} placeholder={t('例：面向国内中小企业技术团队的开源后台框架')} />
          </Form.Item>
        </Form>
        {candidates.length > 0 ? (
          <>
            <Text type="secondary">{t('勾选要加入品牌库的项，名称、别名、类型都可以直接修改；入库时自动跳过重复的：')}</Text>
            <Table
              style={{ marginTop: 8 }}
              rowKey={(_, i) => String(i)}
              size="small"
              pagination={false}
              scroll={{ y: 320 }}
              rowSelection={{
                selectedRowKeys: selectedGen.map(String),
                onChange: (keys) => setSelectedGen(keys.map((k) => Number(k))),
              }}
              columns={[
                {
                  title: t('类型'),
                  dataIndex: 'kind',
                  width: 110,
                  render: (v, _r, idx) => (
                    <Select
                      size="small"
                      value={v as string}
                      style={{ width: 96 }}
                      onChange={(nv) => editCandidate(idx, { kind: nv })}
                      options={[
                        { label: t('自有品牌'), value: 'brand' },
                        { label: t('竞品'), value: 'competitor' },
                      ]}
                    />
                  ),
                },
                {
                  title: t('名称（可编辑）'),
                  dataIndex: 'name',
                  width: 170,
                  render: (v, _r, idx) => (
                    <Input size="small" value={v as string} onChange={(e) => editCandidate(idx, { name: e.target.value })} />
                  ),
                },
                {
                  title: t('别名'),
                  dataIndex: 'aliases',
                  render: (v, _r, idx) => (
                    <Input size="small" value={(v as string) || ''} placeholder={t('逗号分隔')} onChange={(e) => editCandidate(idx, { aliases: e.target.value })} />
                  ),
                },
                {
                  title: t('网站'),
                  dataIndex: 'website',
                  ellipsis: true,
                  render: (v?: string) => (v ? <a href={v} target="_blank" rel="noreferrer">{v}</a> : '—'),
                },
              ]}
              dataSource={candidates}
            />
          </>
        ) : (
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            {t('填你的产品网址（GitHub 仓库、官网都行），AI 会识别站点品牌并列出真实竞品；也可以不填网址，让 AI 根据已录入的自有品牌推荐竞品。识别结果可编辑、勾选后批量入库。')}
          </Paragraph>
        )}
      </Modal>

      <Modal
        title={editing ? t('编辑品牌') : t('新增品牌')}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => void onSubmit()}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
            <Input maxLength={128} />
          </Form.Item>
          <Form.Item name="kind" label={t('类型')} rules={[{ required: true }]}>
            <Select options={[
              { label: t('自有品牌'), value: 'brand' },
              { label: t('竞品'), value: 'competitor' },
            ]}
            />
          </Form.Item>
          <Form.Item
            name="aliases"
            label={t('其他叫法（逗号分隔）')}
            help={t('例如：简称、英文名、常见错拼写。AI 检测时这些写法都会被识别为同一个品牌')}
          >
            <Input placeholder={t('如 go-admin-kit,goadmin')} />
          </Form.Item>
          <Form.Item name="website" label={t('官网（选填）')}>
            <Input placeholder="https://" />
          </Form.Item>
          <Form.Item name="note" label={t('备注（选填）')}>
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
