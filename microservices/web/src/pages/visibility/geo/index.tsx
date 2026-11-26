import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Grid, Select, Space, Table, Tag, Tooltip, Typography } from 'antd'
import {
  BulbOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import {
  deleteGeoPrompt,
  listGeoPrompts,
  listGeoRuns,
  type VisGeoPrompt,
  type VisGeoRun,
} from '@/api/visibility'
import { catLabel, type GeoRunEngine } from './helpers'
import { useGeoRunDetail } from './useGeoRunDetail'
import PromptFormModal from './PromptFormModal'
import GeneratePromptsModal from './GeneratePromptsModal'
import RunDetailDrawer from './RunDetailDrawer'

const { Text, Paragraph } = Typography

export default function VisibilityGeoPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const [prompts, setPrompts] = useState<VisGeoPrompt[]>([])
  const [runs, setRuns] = useState<VisGeoRun[]>([])
  // 后端是否允许模拟演示检测（VISIBILITY_DEMO_MODE，生产默认关）
  const [demoMode, setDemoMode] = useState(false)
  // 本次检测使用的生成式引擎：chatgpt（真实 ChatGPT 联网搜索）| selfhost（自建 bing 接地检索）
  const [runEngine, setRunEngine] = useState<GeoRunEngine>('chatgpt')
  const [loading, setLoading] = useState(true)
  const [promptOpen, setPromptOpen] = useState(false)
  const [editingPrompt, setEditingPrompt] = useState<VisGeoPrompt | null>(null)
  const [genOpen, setGenOpen] = useState(false)

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
    let cancelled = false
    void Promise.all([
      listGeoPrompts({ page: 1, page_size: 50 }),
      listGeoRuns({ page: 1, page_size: 20 }),
    ]).then(
      ([p, r]) => {
        if (cancelled) return
        setPrompts(p.list ?? [])
        setRuns(r.list ?? [])
        // 「模拟演示」入口由后端能力决定：非演示环境 mode=stub 会被直接拒绝，
        // 按钮就不该出现（VISIBILITY_DEMO_MODE，生产默认关）。
        setDemoMode(Boolean(r.demo_mode))
      },
      () => {
        if (!cancelled) message.error(t('加载数据失败'))
      },
    ).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => {
      cancelled = true
    }
  }, [t])

  const {
    submitting,
    polling,
    detailOpen,
    detailRun,
    results,
    advising,
    adviceItems,
    compare,
    comparePrev,
    compareLoading,
    onRun,
    openRun,
    onAdvice,
    loadCompare,
    toggleAdviceItem,
    closeDetail,
  } = useGeoRunDetail(load)

  function openAddPrompt() {
    setEditingPrompt(null)
    setPromptOpen(true)
  }

  function openEditPrompt(row: VisGeoPrompt) {
    setEditingPrompt(row)
    setPromptOpen(true)
  }

  // ChatGPT 联网搜索复杂题每题约 3 分钟，自建检索约 2 分钟
  const llmMinutes = Math.max(1, Math.ceil(prompts.length * (runEngine === 'chatgpt' ? 3 : 2)))

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
                <Button icon={<BulbOutlined />} onClick={() => setGenOpen(true)}>{t('AI 生成问题')}</Button>
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
                onChange={(v) => setRunEngine(v as GeoRunEngine)}
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
          scroll={{ x: 960 }}
        />

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
          scroll={{ x: 960 }}
        />
      </div>

      <PromptFormModal
        open={promptOpen}
        editingPrompt={editingPrompt}
        onClose={() => setPromptOpen(false)}
        onSaved={load}
      />

      <GeneratePromptsModal
        open={genOpen}
        onClose={() => setGenOpen(false)}
        onSaved={load}
      />

      <RunDetailDrawer
        open={detailOpen}
        run={detailRun}
        results={results}
        advising={advising}
        adviceItems={adviceItems}
        compare={compare}
        comparePrev={comparePrev}
        compareLoading={compareLoading}
        onClose={closeDetail}
        onAdvice={onAdvice}
        onRefreshCompare={loadCompare}
        onToggleAdviceItem={toggleAdviceItem}
      />
    </div>
  )
}
