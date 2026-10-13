import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Checkbox, Col, Descriptions, Drawer, Form, Input, Modal, Progress, Row, Select, Space, Statistic, Table, Tag, Tooltip, Typography,
  Grid,
} from 'antd'
import { BulbOutlined, PlayCircleOutlined, PlusOutlined, ReloadOutlined, ThunderboltOutlined, SwapOutlined, EditOutlined, DeleteOutlined, EyeOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePollingTask } from '@/hooks/usePollingTask'
import AiMarkdown from '@/components/common/AiMarkdown'
import {
  batchCreateGeoPrompts, compareGeoRun, createBrand, createGeoPrompt, createGeoRun, deleteGeoPrompt, generateGeoAdvice,
  generateGeoPrompts, getGeoRun, listGeoPrompts, listGeoRuns, saveGeoAdviceItems, updateGeoPrompt,
  type GeneratedPrompt, type SuggestedBrand, type VisAdviceItem, type VisGeoCompare, type VisGeoPrompt, type VisGeoResult, type VisGeoRun,
} from '@/api/visibility'

const catLabel = (v: string) => ({ category: '品类词', comparison: '对比题', scenario: '场景题', other: '其他' }[v] ?? v)

// hostnameOf 取域名用于来源展示，解析失败时退回原串（仓库无现成工具）。
const hostnameOf = (raw: string) => {
  try {
    return new URL(raw).hostname.replace(/^www\./, '')
  } catch {
    return raw
  }
}

const { Text, Paragraph } = Typography

export default function VisibilityGeoPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const sentimentTag = (s?: string) => {
    if (s === 'positive') return <Tag color="green" variant="filled">{t('评价正面')}</Tag>
    if (s === 'negative') return <Tag color="red" variant="filled">{t('评价负面')}</Tag>
    if (s === 'neutral') return <Tag variant="filled">{t('评价中性')}</Tag>
    return null
  }
  const [prompts, setPrompts] = useState<VisGeoPrompt[]>([])
  const [runs, setRuns] = useState<VisGeoRun[]>([])
  // 后端是否允许模拟演示检测（VISIBILITY_DEMO_MODE，生产默认关）
  const [demoMode, setDemoMode] = useState(false)
  // 本次检测使用的生成式引擎：chatgpt（真实 ChatGPT 联网搜索）| selfhost（自建 bing 接地检索）
  const [runEngine, setRunEngine] = useState<'chatgpt' | 'selfhost'>('chatgpt')
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [promptOpen, setPromptOpen] = useState(false)
  const [editingPrompt, setEditingPrompt] = useState<VisGeoPrompt | null>(null)
  const [form] = Form.useForm()
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRun, setDetailRun] = useState<VisGeoRun | null>(null)
  const [results, setResults] = useState<VisGeoResult[]>([])
  const [activeRunId, setActiveRunId] = useState<number | null>(null)
  // AI 生成问题
  const [genOpen, setGenOpen] = useState(false)
  const [genForm] = Form.useForm()
  const [generating, setGenerating] = useState(false)
  const [savingGen, setSavingGen] = useState(false)
  const [candidates, setCandidates] = useState<GeneratedPrompt[]>([])
  const [selectedGen, setSelectedGen] = useState<number[]>([])
  const [suggestedBrand, setSuggestedBrand] = useState<SuggestedBrand | null>(null)
  const [addingBrand, setAddingBrand] = useState(false)
  const [brandAdded, setBrandAdded] = useState(false)
  // AI 优化建议 + 可执行清单 + 历史对比
  const [advising, setAdvising] = useState(false)
  const [adviceItems, setAdviceItems] = useState<VisAdviceItem[]>([])
  const [compare, setCompare] = useState<VisGeoCompare | null>(null)
  const [comparePrev, setComparePrev] = useState<VisGeoRun | null>(null)
  const [compareLoading, setCompareLoading] = useState(false)
  // 详情抽屉当前展示的 run id：openRun 竞态与 onAdvice 长请求写回都以它为准
  const detailRunIdRef = useRef<number | null>(null)
  // adviceItems 的同步镜像：勾选走请求前取值不能依赖异步的 setState
  const adviceItemsRef = useRef<VisAdviceItem[]>([])
  // 保存请求串行链：并发 PUT 是整表覆盖，乱序到达会让后点的勾选被先点的旧表覆盖
  const saveChainRef = useRef<Promise<void>>(Promise.resolve())
  const syncAdviceItems = (items: VisAdviceItem[]) => {
    adviceItemsRef.current = items
    setAdviceItems(items)
  }

  const { polling } = usePollingTask({
    taskId: activeRunId,
    fetcher: getGeoRun,
    isDone: (d) => d.run.status !== 'running',
    onTick: (d) => {
      // 抽屉可能已切到别的 run：后台轮询不得覆盖当前展示
      if (detailRunIdRef.current === d.run.id) {
        setDetailRun(d.run)
        setResults(d.results || [])
      }
    },
    onFinish: (d) => {
      if (d.run.status === 'done') {
        message.success(t('检测完成：{{a}} 个问题中 {{b}} 个提到了你的品牌', { a: d.run.prompt_count, b: d.run.mention_count }))
        // 抽屉正开着这条 run 时，完成后自动带出历史对比
        if (detailRunIdRef.current === d.run.id) void loadCompare(d.run.id)
      } else {
        message.error(d.run.error || t('检测失败'))
      }
      void load()
    },
    onError: () => {
      // 清掉 runId 才能对同一 run 重启轮询（后端 reused 返回相同 id）
      setActiveRunId(null)
      message.warning(t('进度查询连续失败已暂停，检测仍在后台进行，稍后点「检测记录」查看结果'))
    },
  })

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [p, r] = await Promise.all([
        listGeoPrompts({ page: 1, page_size: 50 }),
        listGeoRuns({ page: 1, page_size: 20 }),
      ])
      setPrompts(p.list ?? [])
      setRuns(r.list ?? [])
      // 「模拟演示」入口由后端能力决定：非演示环境 mode=stub 会被直接拒绝，
      // 按钮就不该出现（VISIBILITY_DEMO_MODE，生产默认关）。
      setDemoMode(Boolean(r.demo_mode))
    } catch {
      message.error(t('加载数据失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void load()
  }, [load])

  function openAddPrompt() {
    setEditingPrompt(null)
    form.resetFields()
    form.setFieldsValue({ category: 'category' })
    setPromptOpen(true)
  }

  function openEditPrompt(row: VisGeoPrompt) {
    setEditingPrompt(row)
    form.setFieldsValue({ category: row.category, question: row.question })
    setPromptOpen(true)
  }

  async function onSavePrompt() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editingPrompt) {
        await updateGeoPrompt(editingPrompt.id, values)
        message.success(t('问题已更新'))
      } else {
        await createGeoPrompt(values)
        message.success(t('问题已添加'))
      }
      setPromptOpen(false)
      form.resetFields()
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  function openGen() {
    setCandidates([])
    setSelectedGen([])
    setSuggestedBrand(null)
    setBrandAdded(false)
    genForm.resetFields()
    setGenOpen(true)
  }

  async function onGenerate() {
    const values = await genForm.validateFields().catch(() => null)
    if (!values) return
    setGenerating(true)
    try {
      const res = await generateGeoPrompts({
        site_url: values.site_url?.trim() || undefined,
        hint: values.hint?.trim() || undefined,
        count: values.count,
      })
      const items = res.items ?? []
      setCandidates(items)
      setSelectedGen(items.map((_, i) => i))
      setSuggestedBrand(res.suggested_brand ?? null)
      setBrandAdded(false)
      if (res.site_fetched === false) {
        message.warning(t('网页内容抓取失败（服务器访问不了该站点），已根据网址和 AI 已有认知生成，请核对后再入库'))
      }
      if (items.length === 0) message.info(t('AI 这次没给出问题，换个描述再试试'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('AI 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  async function onAddSuggestedBrand() {
    if (!suggestedBrand) return
    setAddingBrand(true)
    try {
      await createBrand({ name: suggestedBrand.name, aliases: suggestedBrand.aliases, kind: 'brand' })
      setBrandAdded(true)
      message.success(t('品牌「{{name}}」已加入品牌库', { name: suggestedBrand.name }))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入品牌库失败'))
    } finally {
      setAddingBrand(false)
    }
  }

  function editCandidate(idx: number, patch: Partial<GeneratedPrompt>) {
    setCandidates((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  async function onSaveGen() {
    const picks = selectedGen
      .map((i) => candidates[i])
      .filter((it): it is GeneratedPrompt => Boolean(it && it.question.trim()))
    if (picks.length === 0) {
      message.warning(t('先勾选要加入的问题'))
      return
    }
    setSavingGen(true)
    try {
      const res = await batchCreateGeoPrompts(picks)
      message.success(res.skipped > 0
        ? t('已加入 {{a}} 个问题，跳过 {{b}} 个重复', { a: res.created, b: res.skipped })
        : t('已加入 {{n}} 个问题', { n: res.created }))
      setGenOpen(false)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入失败'))
    } finally {
      setSavingGen(false)
    }
  }

  async function onRun(mode: 'stub' | 'llm', engine?: 'chatgpt' | 'selfhost') {
    setSubmitting(true)
    try {
      const res = await createGeoRun({ mode, engine: mode === 'llm' ? engine : 'stub' })
      if (res.reused) {
        message.info(t('已有检测正在进行，为你打开进度'))
      }
      detailRunIdRef.current = res.run.id
      syncAdviceItems([])
      setCompare(null)
      setComparePrev(null)
      setDetailRun(res.run)
      setResults([])
      setActiveRunId(res.run.id)
      setDetailOpen(true)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('启动检测失败'))
    } finally {
      setSubmitting(false)
    }
  }

  async function onAdvice() {
    if (!detailRun) return
    const runId = detailRun.id
    setAdvising(true)
    try {
      const res = await generateGeoAdvice(runId)
      // 生成最长 3 分钟，期间用户可能切到别的 run 详情：只更新还是同一 run 的抽屉
      setDetailRun((prev) => (prev && prev.id === runId ? { ...prev, advice: res.advice } : prev))
      if (detailRunIdRef.current === runId) {
        syncAdviceItems(res.advice_items || [])
        message.success(t('优化建议与行动清单已生成'))
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('生成建议失败'))
    } finally {
      setAdvising(false)
    }
  }

  async function loadCompare(runId: number) {
    setCompareLoading(true)
    try {
      const res = await compareGeoRun(runId)
      // 快速切换 run 详情时，慢的旧响应不得覆盖当前抽屉的对比数据
      if (detailRunIdRef.current !== runId) return
      if (res.has_previous && res.compare) {
        setCompare(res.compare)
        setComparePrev(res.previous || null)
      } else {
        setCompare(null)
        setComparePrev(null)
      }
    } catch {
      if (detailRunIdRef.current === runId) {
        setCompare(null)
        setComparePrev(null)
      }
    } finally {
      if (detailRunIdRef.current === runId) setCompareLoading(false)
    }
  }

  function toggleAdviceItem(id: string, done: boolean) {
    if (!detailRun) return
    const runId = detailRun.id
    // 用 ref 镜像取当前值：setState 的 updater 不保证同步执行
    adviceItemsRef.current = adviceItemsRef.current.map((it) => (it.id === id ? { ...it, done } : it))
    setAdviceItems(adviceItemsRef.current)
    // 串行发送，且发送时取 ref 最新值：连点两项时第二个请求必然包含第一个的状态，
    // 消除并发整表 PUT 乱序到达导致的静默覆盖丢失
    saveChainRef.current = saveChainRef.current.then(async () => {
      if (detailRunIdRef.current !== runId) return
      try {
        await saveGeoAdviceItems(runId, adviceItemsRef.current)
      } catch {
        // 已切到别的 run 时不回滚也不报错，避免把旧 run 的清单写进新抽屉
        if (detailRunIdRef.current !== runId) return
        message.error(t('保存勾选状态失败'))
        adviceItemsRef.current = adviceItemsRef.current.map((it) => (it.id === id ? { ...it, done: !done } : it))
        setAdviceItems(adviceItemsRef.current)
      }
    })
  }

  async function openRun(id: number) {
    detailRunIdRef.current = id
    try {
      const res = await getGeoRun(id)
      // 快速连点两条记录：慢的旧响应不得覆盖后打开的详情
      if (detailRunIdRef.current !== id) return
      setDetailRun(res.run)
      setResults(res.results || [])
      syncAdviceItems(res.advice_items || [])
      // 仍在跑的继续轮询；已结束的只在「当前轮询的就是它」时才停，
      // 不能误杀另一个正在后台轮询的 run
      setActiveRunId((prev) => (res.run.status === 'running' ? id : prev === id ? null : prev))
      setDetailOpen(true)
      if (res.run.status === 'done') {
        void loadCompare(id)
      } else {
        setCompare(null)
        setComparePrev(null)
      }
    } catch {
      if (detailRunIdRef.current === id) message.error(t('加载结果失败'))
    }
  }

  // ChatGPT 联网搜索复杂题每题约 3 分钟，自建检索约 2 分钟
  const llmMinutes = Math.max(1, Math.ceil(prompts.length * (runEngine === 'chatgpt' ? 3 : 2)))
  const recommendedCount = results.filter((r) => r.recommended).length
  const competitorCount = results.filter((r) => r.mention_competitor).length
  const matchedBrands = Array.from(
    new Set(results.flatMap((r) => (r.matched_brands || '').split(',').map((s) => s.trim()).filter(Boolean))),
  )

  const promptCols: ColumnsType<VisGeoPrompt> = [
    { title: t('分类'), dataIndex: 'category', width: 100, render: (v) => t(catLabel(v as string)) },
    { title: t('问题'), dataIndex: 'question', ellipsis: true },
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
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEditPrompt(r) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('删除这个问题？'),
              onClick: () => { void deleteGeoPrompt(r.id).then(load) },
            },
          ]}
        />
      ),
    },
  ]

  const runCols: ColumnsType<VisGeoRun> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: t('方式'),
      dataIndex: 'engine',
      width: 140,
      render: (_, r) => {
        const eng = r.engine || 'selfhost'
        if (r.mode === 'stub') return <Tag variant="filled">{t('模拟演示')}</Tag>
        if (eng === 'chatgpt') return <Tag color="purple" variant="filled">{t('ChatGPT 联网')}</Tag>
        return <Tag color="geekblue" variant="filled">{t('自建检索')}</Tag>
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 110,
      render: (v, r) => {
        if (v === 'running') return <Tag color="processing" variant="filled">{t('进行中 {{a}}/{{b}}', { a: r.done_count, b: r.prompt_count })}</Tag>
        if (v === 'failed') return <Tooltip title={r.error}><Tag color="red" variant="filled">{t('失败')}</Tag></Tooltip>
        return <Tag color="green" variant="filled">{t('完成')}</Tag>
      },
    },
    {
      title: t('提到品牌'),
      width: 100,
      render: (_, r) => `${r.mention_count}/${r.prompt_count}`,
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => formatDateTime(v),
    },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      render: (_, r) => (
        <TableRowActions
          maxInline={1}
          actions={[{ key: 'detail', label: t('详情'), icon: <EyeOutlined />, onClick: () => { void openRun(r.id) } }]}
        />
      ),
    },
  ]

  const isRunning = detailRun?.status === 'running'
  const percent = detailRun && detailRun.prompt_count > 0
    ? Math.round(((detailRun.done_count ?? 0) / detailRun.prompt_count) * 100)
    : 0

  return (
    <div className="page-list">
      <div className="list-main-card" style={{ padding: 16 }}>
        <TableToolbar
          title="AI 提及检测"
          icon={<ThunderboltOutlined />}
          gradient="linear-gradient(135deg, #f472b6, #8b5cf6)"
          glow="rgba(139, 92, 246, 0.4)"
          description="模拟用户向 AI 提问，看看 AI 的回答里有没有提到你的品牌"
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
              <Tooltip title={t('根据你在「品牌与竞品」里填的品牌，让 AI 帮你想出用户可能会问的问题')}>
                <Button icon={<BulbOutlined />} onClick={openGen}>{t('AI 生成问题')}</Button>
              </Tooltip>
              <Button icon={<PlusOutlined />} onClick={openAddPrompt}>{t('加问题')}</Button>
              {demoMode ? (
                <Tooltip title={t('不调用真实 AI，用模板拼出示例回答快速看效果；结果只做演示，不能当作真实检测数据')}>
                  <Button icon={<PlayCircleOutlined />} loading={submitting || polling} onClick={() => void onRun('stub')}>
                    {t('模拟演示')}
                  </Button>
                </Tooltip>
              ) : null}
              <Select
                value={runEngine}
                onChange={(v) => setRunEngine(v as 'chatgpt' | 'selfhost')}
                style={{ width: 170 }}
                options={[
                  { label: t('ChatGPT 联网搜索'), value: 'chatgpt' },
                  { label: t('自建检索'), value: 'selfhost' },
                ]}
              />
              <Tooltip title={t('用真实 AI 回答每个问题并分析提及情况，预计约 {{n}} 分钟，可随时关闭页面稍后回来看结果', { n: llmMinutes })}>
                <Button type="primary" loading={submitting || polling} onClick={() => void onRun('llm', runEngine)}>
                  {t('真实 AI 检测')}
                </Button>
              </Tooltip>
            </Space>
          }
        />
        <Paragraph type="secondary">
          {t('先在下方添加「用户可能问 AI 的问题」，然后选引擎点「真实 AI 检测」。「ChatGPT 联网搜索」问的是真实生成式引擎并带回真实引用；「自建检索」用 bing 检索注入。检测在后台进行，做完会自动提醒你；系统每周一凌晨自动用 ChatGPT 跑一次。想对比两个引擎，就分别跑一次，在检测详情里用历史对比看差异。')}
        </Paragraph>

        <Text strong>{t('检测问题（{{n}}）', { n: prompts.length })}</Text>
        <Table
          style={{ marginTop: 8, marginBottom: 24 }}
          rowKey="id"
          size="small"
          loading={loading}
          columns={promptCols}
          dataSource={prompts}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="还没有问题，点右上角「加问题」" compact /> }}
         scroll={{ x: 960 }} />

        <Text strong>{t('检测记录')}</Text>
        <Table
          style={{ marginTop: 8 }}
          rowKey="id"
          size="small"
          loading={loading}
          columns={runCols}
          dataSource={runs}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="还没做过检测" compact /> }}
         scroll={{ x: 960 }} />
      </div>

      <Modal title={editingPrompt ? t('编辑检测问题') : t('添加检测问题')} open={promptOpen} onCancel={() => setPromptOpen(false)} onOk={() => void onSavePrompt()} destroyOnHidden>
        <Form form={form} layout="vertical" initialValues={{ category: 'category' }}>
          <Form.Item
            name="category"
            label={t('问题类型')}
            help={t('品类词：问某类产品（如“有什么好用的后台系统”）；对比题：和竞品比较；场景题：描述使用场景求推荐')}
          >
            <Select options={[
              { label: t('品类词（问某类产品）'), value: 'category' },
              { label: t('对比题（和竞品比较）'), value: 'comparison' },
              { label: t('场景题（描述场景求推荐）'), value: 'scenario' },
              { label: t('其他'), value: 'other' },
            ]}
            />
          </Form.Item>
          <Form.Item
            name="question"
            label={t('问题内容')}
            rules={[{ required: true, message: t('请输入问题') }]}
            help={t('写用户真实会问 AI 的话。检测时会拿这个问题问 AI，再分析回答里有没有提到你的品牌')}
          >
            <Input.TextArea rows={3} placeholder={t('例：有没有适合中小团队的开源后台管理系统推荐？')} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('AI 生成检测问题')}
        open={genOpen}
        onCancel={() => setGenOpen(false)}
        width={720}
        destroyOnHidden
        footer={[
          <Button key="cancel" onClick={() => setGenOpen(false)}>{t('取消')}</Button>,
          <Button key="gen" icon={<BulbOutlined />} loading={generating} onClick={() => void onGenerate()}>
            {candidates.length > 0 ? t('重新生成') : t('开始生成')}
          </Button>,
          <Button key="save" type="primary" loading={savingGen} disabled={selectedGen.length === 0} onClick={() => void onSaveGen()}>
            {t('加入选中（{{n}}）', { n: selectedGen.length })}
          </Button>,
        ]}
      >
        <Form form={genForm} layout="vertical" initialValues={{ count: 8 }}>
          <Form.Item
            name="site_url"
            label={t('网址（可选）')}
            rules={[{ pattern: /^https?:\/\/.+/i, message: t('以 http:// 或 https:// 开头') }]}
            help={t('填了就抓取这个网页的内容来生成问题，并自动识别站点品牌；不填则按「品牌与竞品」里的资料生成')}
          >
            <Input allowClear placeholder={t('例：https://your-product.com')} />
          </Form.Item>
          <Form.Item
            name="hint"
            label={t('行业/受众补充说明（可选）')}
            help={t('填了能更贴合，例如目标客户、使用场景、地域')}
          >
            <Input.TextArea rows={2} placeholder={t('例：面向国内中小企业技术团队的开源后台框架')} />
          </Form.Item>
          <Form.Item name="count" label={t('生成数量')}>
            <Select
              options={[5, 8, 12, 20].map((n) => ({ label: t('{{n}} 个', { n }), value: n }))}
              style={{ width: 140 }}
            />
          </Form.Item>
        </Form>
        {suggestedBrand ? (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 12 }}
            message={
              <Space wrap>
                <span>
                  {t('识别到站点品牌：')}<Text strong>{suggestedBrand.name}</Text>
                  {suggestedBrand.aliases ? <Text type="secondary">{t('（别名：{{name}}）', { name: suggestedBrand.aliases })}</Text> : null}
                </span>
                {brandAdded ? (
                  <Tag color="green" variant="filled">{t('已加入品牌库')}</Tag>
                ) : (
                  <Button size="small" type="primary" ghost loading={addingBrand} onClick={() => void onAddSuggestedBrand()}>
                    {t('加入品牌库')}
                  </Button>
                )}
              </Space>
            }
          />
        ) : null}
        {candidates.length > 0 ? (
          <>
            <Text type="secondary">{t('勾选要加入的问题，问题文字和分类都可以直接修改；入库时自动跳过与题库重复的：')}</Text>
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
                  title: t('分类'),
                  dataIndex: 'category',
                  width: 120,
                  render: (v, _r, idx) => (
                    <Select
                      size="small"
                      value={v as string}
                      style={{ width: 104 }}
                      onChange={(nv) => editCandidate(idx, { category: nv })}
                      options={[
                        { label: t('品类词'), value: 'category' },
                        { label: t('对比题'), value: 'comparison' },
                        { label: t('场景题'), value: 'scenario' },
                        { label: t('其他'), value: 'other' },
                      ]}
                    />
                  ),
                },
                {
                  title: t('问题（可编辑）'),
                  dataIndex: 'question',
                  render: (v, _r, idx) => (
                    <Input.TextArea
                      size="small"
                      autoSize={{ minRows: 1, maxRows: 3 }}
                      value={v as string}
                      onChange={(e) => editCandidate(idx, { question: e.target.value })}
                    />
                  ),
                },
              ]}
              dataSource={candidates}
            />
          </>
        ) : (
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            {t('填一个产品官网网址（或留空用品牌资料），点「开始生成」，AI 会站在潜在用户角度想出他们可能向 AI 提问的问题。生成后可编辑、勾选加入检测题库。')}
          </Paragraph>
        )}
      </Modal>

      <Drawer
        title={detailRun ? t('检测 #{{n}}', { n: detailRun.id }) : t('检测结果')}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width={560}
      >
        {isRunning ? (
          <div style={{ marginBottom: 16 }}>
            <Progress percent={percent} status="active" />
            <Text type="secondary">
              {t('已完成 {{a}}/{{b}} 个问题，正在向 AI 提问……可以关闭此窗口，稍后在「检测记录」里查看', { a: detailRun?.done_count ?? 0, b: detailRun?.prompt_count ?? 0 })}
            </Text>
          </div>
        ) : null}
        {detailRun?.status === 'failed' ? (
          <Alert type="error" showIcon style={{ marginBottom: 16 }} message={t('检测失败')} description={detailRun.error || t('未知原因')} />
        ) : null}
        {detailRun && detailRun.status === 'done' ? (
          <>
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <Statistic
                  title={t('提及率')}
                  value={detailRun.prompt_count > 0 ? Math.round((detailRun.mention_count / detailRun.prompt_count) * 100) : 0}
                  suffix="%"
                />
              </Col>
              <Col span={8}>
                <Statistic title={t('正面推荐')} value={recommendedCount} suffix={t('条')} />
              </Col>
              <Col span={8}>
                <Statistic title={t('提到竞品')} value={competitorCount} suffix={t('条')} />
              </Col>
            </Row>
            {matchedBrands.length > 0 ? (
              <div style={{ marginBottom: 16 }}>
                <Text type="secondary">{t('回答中出现过的品牌：')}</Text>{' '}
                {matchedBrands.map((b) => <Tag key={b} variant="filled">{b}</Tag>)}
              </div>
            ) : null}
            <Descriptions size="small" column={1} bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label={t('方式')}>
                {detailRun.mode === 'stub' ? t('模拟演示') : detailRun.engine === 'chatgpt' ? t('真实 AI · ChatGPT 联网搜索') : t('真实 AI · 自建检索')}
              </Descriptions.Item>
              <Descriptions.Item label={t('时间')}>{formatDateTime(detailRun.created_at)}</Descriptions.Item>
            </Descriptions>
            {/* 历史对比 */}
            {detailRun.status === 'done' ? (
              <div
                style={{
                  border: '1px solid rgba(59, 130, 246, .35)',
                  borderRadius: 10,
                  padding: 12,
                  marginBottom: 16,
                  background: 'rgba(59, 130, 246, .06)',
                }}
              >
                <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 8 }}>
                  <Text strong><SwapOutlined /> {t('与上次检测对比')}</Text>
                  <Button size="small" loading={compareLoading} onClick={() => void loadCompare(detailRun.id)}>
                    {t('刷新对比')}
                  </Button>
                </Space>
                {!compare ? (
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    {compareLoading ? t('加载中…') : t('暂无更早的完成记录，再跑一次检测后可对比提及率变化。')}
                  </Paragraph>
                ) : (
                  <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    <Row gutter={12}>
                      <Col span={8}>
                        <Statistic
                          title={t('本次提及率 (#{{n}})', { n: compare.current_id })}
                          value={Math.round(compare.current_rate * 10) / 10}
                          suffix="%"
                        />
                      </Col>
                      <Col span={8}>
                        <Statistic
                          title={t('上次 (#{{a}}{{b}})', {
                            a: compare.previous_id,
                            b: comparePrev?.mode ? ` · ${comparePrev.mode}` : '',
                          })}
                          value={Math.round(compare.previous_rate * 10) / 10}
                          suffix="%"
                        />
                      </Col>
                      <Col span={8}>
                        <Statistic
                          title={t('变化')}
                          value={Math.round(compare.rate_delta * 10) / 10}
                          suffix="pt"
                          valueStyle={{ color: compare.rate_delta >= 0 ? 'var(--c-success)' : 'var(--c-error)' }}
                          prefix={compare.rate_delta >= 0 ? '+' : ''}
                        />
                      </Col>
                    </Row>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {t('提及数 {{a}} → {{b}}（Δ {{c}}）· 新提到 {{d}} 题 · 新丢失 {{e}} 题 · 仍未提到 {{f}} 题', {
                        a: compare.previous_mentions,
                        b: compare.current_mentions,
                        c: (compare.mention_delta >= 0 ? '+' : '') + compare.mention_delta,
                        d: compare.newly_mentioned?.length || 0,
                        e: compare.newly_missed?.length || 0,
                        f: compare.still_missing?.length || 0,
                      })}
                    </Text>
                    {(compare.newly_mentioned?.length || 0) > 0 ? (
                      <div>
                        <Text type="success" style={{ fontSize: 12 }}>{t('新提到：')}</Text>
                        {compare.newly_mentioned!.slice(0, 5).map((q) => (
                          <Tag key={q} color="green" style={{ marginTop: 4 }}>{q.slice(0, 40)}{q.length > 40 ? '…' : ''}</Tag>
                        ))}
                      </div>
                    ) : null}
                    {(compare.newly_missed?.length || 0) > 0 ? (
                      <div>
                        <Text type="danger" style={{ fontSize: 12 }}>{t('新丢失：')}</Text>
                        {compare.newly_missed!.slice(0, 5).map((q) => (
                          <Tag key={q} color="red" style={{ marginTop: 4 }}>{q.slice(0, 40)}{q.length > 40 ? '…' : ''}</Tag>
                        ))}
                      </div>
                    ) : null}
                  </Space>
                )}
              </div>
            ) : null}

            <div
              style={{
                border: '1px solid rgba(139, 92, 246, .35)',
                borderRadius: 10,
                padding: 12,
                marginBottom: 16,
                background: 'rgba(139, 92, 246, .06)',
              }}
            >
              <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: detailRun.advice ? 8 : 0 }}>
                <Text strong><BulbOutlined /> {t('AI 优化建议')}</Text>
                <Button size="small" loading={advising} onClick={() => void onAdvice()}>
                  {detailRun.advice ? t('重新生成') : t('生成建议')}
                </Button>
              </Space>
              {detailRun.advice ? (
                <>
                  {adviceItems.length > 0 ? (
                    <div style={{ marginBottom: 12 }}>
                      <Text strong style={{ fontSize: 13 }}>{t('可执行清单')}</Text>
                      <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
                        {t('已完成 {{a}}/{{b}}', { a: adviceItems.filter((i) => i.done).length, b: adviceItems.length })}
                      </Text>
                      <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
                        {adviceItems.map((it) => (
                          <div
                            key={it.id}
                            style={{
                              display: 'flex',
                              gap: 8,
                              alignItems: 'flex-start',
                              opacity: it.done ? 0.65 : 1,
                            }}
                          >
                            <Checkbox
                              checked={!!it.done}
                              onChange={(e) => void toggleAdviceItem(it.id, e.target.checked)}
                            />
                            <div style={{ flex: 1 }}>
                              <Space size={6} wrap>
                                <Text delete={!!it.done} strong={!it.done}>{it.title}</Text>
                                {it.priority === 'high' ? <Tag color="red">{t('高优')}</Tag> : null}
                                {it.priority === 'medium' ? <Tag color="orange">{t('中')}</Tag> : null}
                                {it.priority === 'low' ? <Tag>{t('低')}</Tag> : null}
                              </Space>
                              {it.detail ? (
                                <Paragraph type="secondary" style={{ marginBottom: 0, fontSize: 12 }}>
                                  {it.detail}
                                </Paragraph>
                              ) : null}
                              {it.related ? (
                                <Text type="secondary" style={{ fontSize: 11 }}>{t('关联：{{name}}', { name: it.related })}</Text>
                              ) : null}
                            </div>
                          </div>
                        ))}
                      </Space>
                    </div>
                  ) : null}
                  <AiMarkdown content={detailRun.advice} />
                </>
              ) : (
                <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  {t('根据这次检测的逐题结果，生成可勾选的行动清单 + 完整建议正文，并与上次检测对比提及率变化。')}
                </Paragraph>
              )}
            </div>
          </>
        ) : null}
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          {results.map((r) => (
            <div key={r.id} style={{ border: '1px solid rgba(148,163,184,.2)', borderRadius: 10, padding: 12 }}>
              <Text strong>{r.question}</Text>
              <div style={{ margin: '8px 0' }}>
                {r.mention_brand ? <Tag color="green" variant="filled">{t('提到品牌')}</Tag> : <Tag variant="filled">{t('没提到品牌')}</Tag>}
                {r.mention_competitor ? <Tag color="orange" variant="filled">{t('提到竞品')}</Tag> : null}
                {r.recommended ? <Tag color="blue" variant="filled">{t('正面推荐')}</Tag> : null}
                {sentimentTag(r.sentiment)}
                {r.list_position > 0 ? <Tag color="purple" variant="filled">{t('推荐榜第 {{n}} 位', { n: r.list_position })}</Tag> : null}
                {r.has_link ? <Tag variant="filled">{t('引用来源')}</Tag> : null}
                {r.analyze_by === 'heuristic' && detailRun?.mode === 'llm' ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>{t('（关键词分析）')}</Text>
                ) : null}
              </div>
              <Paragraph type="secondary" style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
                {r.answer}
              </Paragraph>
              {r.sources && r.sources.length > 0 ? (
                <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {t('来源引用（{{a}}/{{b}}）', { a: r.sources.filter((s) => s.cited).length, b: r.sources.length })}
                  </Text>
                  {r.sources.map((s, i) => {
                    const host = hostnameOf(s.url)
                    return (
                      <div key={i} style={{ fontSize: 12, display: 'flex', gap: 6, alignItems: 'baseline', opacity: s.cited ? 1 : 0.55 }}>
                        <Text style={{ fontSize: 12 }} type={s.cited ? undefined : 'secondary'}>
                          {s.cited ? t('被引用') : t('未引用')} ·{' '}
                          <a href={s.url} target="_blank" rel="noopener noreferrer">{s.title || host}</a>
                          <Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{host}</Text>
                        </Text>
                        {s.own_domain ? <Tag color="green" style={{ marginInlineEnd: 0 }}>{t('自有')}</Tag> : null}
                      </div>
                    )
                  })}
                </div>
              ) : detailRun?.mode === 'llm' && detailRun?.engine !== 'chatgpt' && r.engine !== 'llm_fallback_stub' ? (
                <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
                  {t('未检索到来源（已降级为无来源回答）')}
                </Text>
              ) : null}
            </div>
          ))}
        </Space>
      </Drawer>
    </div>
  )
}
