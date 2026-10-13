import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Drawer, Form, Input, InputNumber, Modal, Select, Space, Spin, Table, Tag } from 'antd'
import { PlusOutlined, ReloadOutlined, RobotOutlined, EyeOutlined, StopOutlined, ThunderboltOutlined, EditOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  cancelAgentRun,
  createAgentDefinition,
  createAgentRun,
  getAgentDefinition,
  listAgentDefinitions,
  listAgentRuns,
  listAgentTools,
  publishAgentVersion,
  type AgentDefinition,
  type AgentRun,
  type AgentRunStatus,
  type AgentToolDefinition,
  type AgentVersion,
} from '@/api/ai/agent'
import { getEnabledAIModels, type AIModel } from '@/api/ai/model'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import RunDrawer from './run-drawer'
import DefinitionEditor from './definition-editor'
import { canRunDefinition } from './definition-form'
import { canCancelRun, defStatusMeta, runStatusMeta } from './_shared'

/** 最近运行固定拉 20 条，不再分页筛选（管理页最小闭环）。 */
const RUNS_PARAMS = { page: 1, page_size: 20 }

/** 幂等键客户端生成：失败重试同键复用，避免重复扣模型账。 */
const newIdempotencyKey = () => `web-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

/** v0 仅允许选择服务端登记的只读工具；空工具列表也可发布为普通 Chat Agent。 */
export default function AgentKernelPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:agent:write')
  const canRun = hasPerm('ai:agent:run')

  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<AgentDefinition | null>(null)
  const [versionTarget, setVersionTarget] = useState<AgentDefinition | null>(null)
  const [runTarget, setRunTarget] = useState<AgentDefinition | null>(null)
  const [runIdempotencyKey, setRunIdempotencyKey] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [detail, setDetail] = useState<{ definition: AgentDefinition; versions: AgentVersion[] } | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState(false)
  const [detailTargetId, setDetailTargetId] = useState<number | null>(null)
  // 详情请求的单调序号：迟到的响应不能覆盖后发起的那次（同 DefinitionEditor 的 requestID 先例）。
  const detailRequestRef = useRef(0)

  const [form] = Form.useForm()
  const [versionForm] = Form.useForm()
  const [runForm] = Form.useForm()

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(() => ({ page: pagination.page, page_size: pagination.page_size }), [pagination.page, pagination.page_size])
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listAgentDefinitions(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: agents, total, loading, error: agentsError, reload } = useTableQuery({ params, fetcher })

  // 发布版本 / 发起 Run 时按需加载：已启用模型 + 服务端登记工具。
  const [models, setModels] = useState<AIModel[]>([])
  const [tools, setTools] = useState<AgentToolDefinition[]>([])
  const [optionsLoading, setOptionsLoading] = useState(false)
  const [optionsError, setOptionsError] = useState(false)
  const loadRuntimeOptions = useCallback(async () => {
    setOptionsLoading(true)
    setOptionsError(false)
    try {
      const [modelList, toolList] = await Promise.all([getEnabledAIModels(), listAgentTools()])
      setModels(modelList ?? [])
      setTools(toolList ?? [])
    } catch {
      // Request layer already reports the server message.
      // 关键：失败必须显式记错——把服务故障渲染成"暂无已启用模型"是假零值，
      // 用户会照着去「AI 模型配置」里找不存在的毛病。
      setOptionsError(true)
    } finally {
      setOptionsLoading(false)
    }
  }, [])

  const openDetail = async (id: number) => {
    const ticket = ++detailRequestRef.current
    setDetailOpen(true)
    setDetailTargetId(id)
    setDetailError(false)
    setDetailLoading(true)
    // 只在**切换到另一个定义**时清空：加载中/失败都不得显示上一个定义的版本列表（那是错数据）。
    // 同一个定义重新加载时保留当前内容（这是合法的"旧数据保留"，也让手动重试不闪空）。
    setDetail((prev) => (prev?.definition.id === id ? prev : null))
    try {
      const next = await getAgentDefinition(id)
      if (ticket !== detailRequestRef.current) return
      // 即便响应正常，也必须确认回来的是请求的那一个定义。
      if (next?.definition?.id !== id) throw new Error('invalid definition details')
      setDetail(next)
    } catch {
      // Request layer already reports the server message.
      if (ticket === detailRequestRef.current) setDetailError(true)
    } finally {
      if (ticket === detailRequestRef.current) setDetailLoading(false)
    }
  }

  // 关闭只是收起抽屉，**不作废**在途请求：既有契约要求"重开同一定义时可用刚取回的缓存"
  // （见 e2e agent-management「迟到B详情与A保存交错」）。跨定义的覆盖由票据守卫负责。
  const closeDetail = () => setDetailOpen(false)

  const openVersion = (definition: AgentDefinition) => {
    versionForm.resetFields()
    versionForm.setFieldsValue({ tools: [], max_steps: 8, timeout_seconds: 120 })
    void loadRuntimeOptions()
    setVersionTarget(definition)
  }

  const openRun = (definition: AgentDefinition) => {
    // 与服务端一致：禁用只阻止新 Run，不影响已有 Run 的详情与取消。
    if (!canRun || definition.status !== 'active') return
    if (!definition.latest_version_id) {
      message.warning(t('请先发布一个版本再发起运行'))
      return
    }
    runForm.resetFields()
    setRunIdempotencyKey(newIdempotencyKey())
    setRunTarget(definition)
  }

  const submitCreate = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      await createAgentDefinition({ agent_key: values.agent_key, name: values.name, description: values.description })
      message.success(t('Agent 已创建，请发布版本后运行'))
      setCreateOpen(false)
      void reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const submitVersion = async () => {
    if (!versionTarget) return
    const values = await versionForm.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      await publishAgentVersion(versionTarget.id, {
        instructions: values.instructions,
        model_id: values.model_id,
        tools: values.tools ?? [],
        max_steps: values.max_steps,
        timeout_seconds: values.timeout_seconds,
      })
      message.success(t('版本已发布，历史版本不可变'))
      setVersionTarget(null)
      void reload()
      if (detail?.definition.id === versionTarget.id) void openDetail(versionTarget.id)
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const submitRun = async () => {
    if (!runTarget || !canRunDefinition(runTarget, canRun)) return
    const values = await runForm.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      // 一个运行弹窗复用同一幂等键：即使首次请求成功但响应丢失，
      // 用户再次提交也只会复用原 Run，不重复扣模型账。
      const idempotencyKey = runIdempotencyKey ?? newIdempotencyKey()
      if (!runIdempotencyKey) setRunIdempotencyKey(idempotencyKey)
      const result = await createAgentRun(runTarget.id, values.input, idempotencyKey)
      message.success(result.reused ? t('复用同幂等键的运行') : t('运行已提交'))
      setRunTarget(null)
      setRunIdempotencyKey(null)
      setDrawerRunId(result.run.id)
      setDrawerOpen(true)
      void reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const cancelRun = async (record: AgentRun) => {
    try {
      await cancelAgentRun(record.id)
      message.success(t('已请求取消'))
      void reloadRuns()
    } catch {
      // Request layer already reports the server message.
    }
  }

  // Run 列表：复用 useTableQuery，固定拉最近 20 条（管理页最小闭环）。
  const runsFetcher = useCallback(async () => {
    const result = await listAgentRuns(RUNS_PARAMS)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: runs, loading: runsLoading, error: runsError, reload: reloadRuns } = useTableQuery({
    params: RUNS_PARAMS,
    fetcher: runsFetcher,
  })

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [drawerRunId, setDrawerRunId] = useState<number | null>(null)
  const openRunDrawer = (id: number) => {
    setDrawerRunId(id)
    setDrawerOpen(true)
  }

  const columns: ColumnsType<AgentDefinition> = [
    {
      title: t('Agent'),
      dataIndex: 'name',
      render: (value: string, record) => (
        <a onClick={() => void openDetail(record.id)}>
          <div>{value}</div>
          <span className="cell-mono cell-muted">{record.agent_key}</span>
        </a>
      ),
    },
    { title: t('描述'), dataIndex: 'description', ellipsis: true, render: (v: string) => <span className="cell-muted">{v || '—'}</span> },
    {
      title: t('最新版本'),
      dataIndex: 'latest_version_id',
      width: 100,
      render: (v?: number) => (v ? <Tag color="geekblue">#{v}</Tag> : <span className="cell-muted">{t('未发布')}</span>),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (v: AgentDefinition['status']) => {
        const meta = defStatusMeta[v]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} /> : v
      },
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 160, render: (v: string) => formatDateTime(v) },
    {
      title: t('操作'),
      width: 150,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          maxInline={2}
          actions={[
            { key: 'detail', label: t('详情'), icon: <EyeOutlined />, onClick: () => { void openDetail(record.id) } },
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, show: canWrite, onClick: () => setEditTarget(record) },
            { key: 'version', label: t('发布版本'), icon: <ThunderboltOutlined />, show: canWrite, onClick: () => openVersion(record) },
            { key: 'run', label: t('运行'), icon: <RobotOutlined />, show: canRunDefinition(record, canRun), onClick: () => openRun(record) },
          ]}
        />
      ),
    },
  ]

  const runColumns: ColumnsType<AgentRun> = [
    { title: 'ID', dataIndex: 'id', width: 70, render: (v: number) => <a onClick={() => openRunDrawer(v)}>{v}</a> },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (v: AgentRunStatus) => {
        const meta = runStatusMeta[v]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} pulse={canCancelRun(v) && v !== 'cancelling'} /> : v
      },
    },
    { title: t('输入'), dataIndex: 'input', ellipsis: true, render: (v: string) => <span className="cell-muted">{v}</span> },
    {
      title: t('输出'),
      dataIndex: 'output',
      ellipsis: true,
      render: (v: string) => (v ? <span className="cell-muted">{v}</span> : '—'),
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 160, render: (v: string) => formatDateTime(v) },
    {
      title: t('操作'),
      width: 100,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          maxInline={2}
          actions={[
            { key: 'detail', label: t('详情'), icon: <EyeOutlined />, onClick: () => openRunDrawer(record.id) },
            { key: 'cancel', label: t('取消'), icon: <StopOutlined />, danger: true, show: canRun && canCancelRun(record.status), onClick: () => { void cancelRun(record) } },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      toolbar={
        <TableToolbar
          title="Agent 管理"
          total={total}
          icon={<RobotOutlined />}
          gradient="linear-gradient(135deg, #6366f1, #0ea5e9)"
          glow="rgba(99, 102, 241, 0.28)"
          description={t('Agent Kernel：定义 Agent、发布不可变版本、发起运行并跟踪步骤与事件。')}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void reloadRuns() }} loading={loading || runsLoading} />
              {canWrite && (
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => { form.resetFields(); setCreateOpen(true) }}
                >
                  {t('新建 Agent')}
                </Button>
              )}
            </Space>
          }
        />
      }
    >
      {(agentsError || runsError) && (
        <Alert
          type="error"
          showIcon
          message={t('列表加载失败')}
          action={<Button size="small" onClick={() => { void reload(); void reloadRuns() }}>{t('重试')}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      <Table
        rowKey="id"
        size="middle"
        loading={loading}
        columns={columns}
        dataSource={agents}
        scroll={{ x: 900 }}
        locale={{ emptyText: <GlassEmpty text={t('暂无 Agent，点工具栏说明开始创建')} compact /> }}
        pagination={{
          total,
          current: pagination.page,
          pageSize: pagination.page_size,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => pagination.setPage(page, page_size),
        }}
        style={{ marginBottom: 24 }}
      />

      <TableToolbar title={t('最近运行')} total={runs.length} icon={<ThunderboltOutlined />} />
      <Table
        rowKey="id"
        size="middle"
        loading={runsLoading}
        columns={runColumns}
        dataSource={runs}
        scroll={{ x: 900 }}
        locale={{ emptyText: <GlassEmpty text={t('暂无运行记录')} compact /> }}
        pagination={false}
      />

      {/* 编辑按定义挂载独立弹窗，关闭/切换时卸载，隔离迟到响应和表单草稿。 */}
      {editTarget && (
        <DefinitionEditor
          key={editTarget.id}
          id={editTarget.id}
          canWrite={canWrite}
          onClose={() => setEditTarget(null)}
          onSaved={(definition) => {
            void reload()
            // PATCH期间可能到达另一个定义的详情，必须在更新时按当前ID判断，
            // 不能把提交时闭包中的A定义拼接到当前B版本列表上。
            setDetail((current) => current?.definition.id === definition.id ? { ...current, definition } : current)
          }}
        />
      )}

      {/* 新建 Agent */}
      <Modal
        title={t('新建 Agent')}
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void submitCreate()}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="agent_key"
            label={t('Agent 标识')}
            rules={[
              { required: true, message: t('请输入 Agent 标识') },
              { pattern: /^[a-z][a-z0-9_-]{1,63}$/, message: t('小写字母开头，仅小写字母/数字/-/_，2-64 位') },
            ]}
            extra={t('创建后不可修改，用于程序引用')}
          >
            <Input placeholder="kb_assistant" maxLength={64} />
          </Form.Item>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }, { max: 100 }]}>
            <Input placeholder={t('如：知识库问答助手')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')} extra={t('可选')}>
            <Input.TextArea rows={3} maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 发布版本 */}
      <Modal
        title={t('发布版本 · {{name}}', { name: versionTarget?.name ?? '' })}
        open={versionTarget !== null}
        onCancel={() => setVersionTarget(null)}
        onOk={() => void submitVersion()}
        confirmLoading={submitting}
        destroyOnHidden
        width={640}
      >
        <Form form={versionForm} layout="vertical" preserve={false}>
          <Form.Item
            name="model_id"
            label={t('模型')}
            rules={[{ required: true, message: t('请选择模型') }]}
          >
            <Select
              placeholder={t('选择已启用的 AI 模型')}
              options={models.map((m) => ({ value: m.id, label: m.display_name || m.name }))}
              notFoundContent={
                optionsError
                  ? <span>{t('模型加载失败')} <Button type="link" size="small" onClick={() => void loadRuntimeOptions()}>{t('重试')}</Button></span>
                  : optionsLoading ? <Spin size="small" /> : t('暂无已启用模型，请先到「AI 模型配置」添加')
              }
            />
          </Form.Item>
          <Form.Item
            name="instructions"
            label={t('指令（Instructions）')}
            rules={[{ required: true, message: t('请输入系统指令') }]}
          >
            <Input.TextArea rows={5} maxLength={20000} showCount placeholder={t('定义 Agent 的角色、目标与行为边界…')} />
          </Form.Item>
          <Form.Item
            name="tools"
            label={t('工具（只读）')}
            extra={t('仅服务端登记的只读工具可选；knowledge_search 复用租户隔离知识库。')}
          >
            <Select
              mode="multiple"
              allowClear
              placeholder={t('可多选')}
              options={tools.map((tool) => ({ value: tool.name, label: `${tool.name} · ${tool.description}` }))}
              notFoundContent={
                optionsError
                  ? <span>{t('工具加载失败')} <Button type="link" size="small" onClick={() => void loadRuntimeOptions()}>{t('重试')}</Button></span>
                  : optionsLoading ? <Spin size="small" /> : t('暂无登记工具')
              }
            />
          </Form.Item>
          <Space size={16} style={{ display: 'flex' }}>
            <Form.Item name="max_steps" label={t('最大步数')} rules={[{ required: true }]} style={{ flex: 1 }}>
              <InputNumber min={1} max={32} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="timeout_seconds" label={t('超时（秒）')} rules={[{ required: true }]} style={{ flex: 1 }}>
              <InputNumber min={10} max={600} style={{ width: '100%' }} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 发起运行 */}
      <Modal
        title={t('运行 · {{name}}', { name: runTarget?.name ?? '' })}
        open={runTarget !== null}
        onCancel={() => { setRunTarget(null); setRunIdempotencyKey(null) }}
        onOk={() => void submitRun()}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={runForm} layout="vertical" preserve={false}>
          <Form.Item name="input" label={t('任务输入')} rules={[{ required: true, message: t('请输入任务输入') }, { max: 20000 }]}>
            <Input.TextArea rows={6} maxLength={20000} showCount placeholder={t('告诉这个 Agent 要做什么…')} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 版本历史 */}
      <Drawer
        title={detail ? t('Agent 详情 · {{name}}', { name: detail.definition.name }) : t('Agent 详情')}
        open={detailOpen}
        onClose={closeDetail}
        width={720}
      >
        {detailLoading && <Spin style={{ display: 'block', margin: '48px auto' }} />}
        {detailError && (
          <Alert
            type="error"
            showIcon
            message={t('详情加载失败')}
            action={<Button size="small" onClick={() => detailTargetId !== null && void openDetail(detailTargetId)}>{t('重试')}</Button>}
          />
        )}
        {detail && (
          <>
            <Space size={12} wrap style={{ marginBottom: 16 }}>
              {defStatusMeta[detail.definition.status] && (
                <StatusPill tone={defStatusMeta[detail.definition.status].tone} label={defStatusMeta[detail.definition.status].label} />
              )}
              <span className="cell-mono cell-muted">{detail.definition.agent_key}</span>
              <span className="cell-muted">{detail.definition.description || '—'}</span>
            </Space>
            <Table
              rowKey="id"
              size="small"
              columns={[
                { title: t('版本'), dataIndex: 'version', width: 80, render: (v: number) => <Tag color="geekblue">v{v}</Tag> },
                { title: t('模型'), dataIndex: 'model_id', width: 90, render: (v: number) => `#${v}` },
                { title: t('工具'), dataIndex: 'tools', render: (tools: string[]) => (tools?.length ? tools.map((tool) => <Tag key={tool}>{tool}</Tag>) : '—') },
                { title: t('步数/超时'), width: 110, render: (_, record: AgentVersion) => `${record.max_steps} / ${record.timeout_seconds}s` },
                { title: t('发布时间'), dataIndex: 'created_at', width: 160, render: (v: string) => formatDateTime(v) },
                {
                  title: t('操作'),
                  width: 80,
                  render: (_, record: AgentVersion) =>
                    record.id === detail.definition.latest_version_id ? <Tag color="success">{t('当前')}</Tag> : null,
                },
              ]}
              dataSource={detail.versions}
              pagination={false}
              locale={{ emptyText: <GlassEmpty text={t('尚未发布版本')} compact /> }}
            />
          </>
        )}
      </Drawer>

      <RunDrawer
        runId={drawerRunId}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        onChanged={() => void reloadRuns()}
      />
    </ListPageShell>
  )
}
