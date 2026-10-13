import type { ReactNode } from 'react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Drawer, Form, Input, Modal, Select, Space, Table, Tag, Tooltip , Grid } from 'antd'
import { ApartmentOutlined, PlusOutlined, ReloadOutlined, EyeOutlined, PlayCircleOutlined, CheckOutlined, StopOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  activateHermesTask,
  cancelHermesTask,
  completeHermesTask,
  createHermesTask,
  getHermesTask,
  listHermesClusters,
  listHermesPolicies,
  listHermesTasks,
  updateHermesTaskRun,
  type HermesCluster,
  type HermesPolicy,
  type HermesTask,
  type HermesTaskDetail,
  type HermesTaskPayload,
  type HermesTaskRun,
  type HermesTaskRunStatus,
  type HermesTaskStatus,
} from '@/api/ai/hermes'
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
import { useIncrementalList } from '../useIncrementalList'
import { taskRunStateLabels, taskStateLabels, taskStateTone } from '../_shared'

const STATUS_FILTERS: { label: string; value: HermesTaskStatus | '' }[] = [
  { label: '全部', value: '' },
  { label: '待激活', value: 'pending' },
  { label: '执行中', value: 'active' },
  { label: '已完成', value: 'completed' },
  { label: '已取消', value: 'cancelled' },
]

export default function HermesCollaborationsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:task:write')

  const [status, setStatus] = useState<HermesTaskStatus | ''>('')

  const [modalOpen, setModalOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const [detail, setDetail] = useState<HermesTaskDetail | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)

  // run 结果输入：受控 Modal 比 modal.confirm 里嵌 Input 干净
  const [resultTarget, setResultTarget] = useState<{ run: HermesTaskRun; to: 'completed' | 'failed' } | null>(null)
  const [resultText, setResultText] = useState('')
  const [resultSubmitting, setResultSubmitting] = useState(false)

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ status: status || undefined, page: pagination.page, page_size: pagination.page_size }),
    [status, pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesTasks(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: tasks, total, loading, reload } = useTableQuery({ params, fetcher })

  // 辅助选择器：集群/策略按页追加加载
  const clusters = useIncrementalList<HermesCluster>(
    useCallback((page, pageSize) => listHermesClusters({ page, page_size: pageSize }), []),
  )
  const policies = useIncrementalList<HermesPolicy>(
    useCallback((page, pageSize) => listHermesPolicies({ page, page_size: pageSize }), []),
  )

  const clusterOptions = useMemo(
    () => clusters.items.filter((c) => c.enabled).map((c) => ({ value: c.id, label: c.name })),
    [clusters.items],
  )
  const clusterPolicies = useMemo(
    () => policies.items.filter((p) => p.profile === 'company_cluster'),
    [policies.items],
  )

  const refreshDetail = useCallback(async (id: number) => {
    setDetailLoading(true)
    try {
      setDetail(await getHermesTask(id))
    } catch {
      // Request layer already reports the server message.
    } finally {
      setDetailLoading(false)
    }
  }, [])

  const openDetail = async (id: number) => {
    setDrawerOpen(true)
    await refreshDetail(id)
  }

  const openCreate = () => {
    if (clusterOptions.length === 0) {
      message.warning(t('请先到「Agent 集群」创建并启用一个集群'))
      return
    }
    form.setFieldsValue({ cluster_id: clusterOptions[0]?.value, policy_id: undefined })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const payload: HermesTaskPayload = {
        name: values.name,
        description: values.description,
        cluster_id: values.cluster_id ?? 0,
        policy_id: values.policy_id ?? 0,
      }
      await createHermesTask(payload)
      message.success(t('任务已创建'))
      setModalOpen(false)
      void reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const refreshIfCurrent = (taskID: number) => {
    if (detail?.task.id === taskID) void refreshDetail(taskID)
  }

  const activate = async (record: HermesTask) => {
    try {
      await activateHermesTask(record.id)
      message.success(t('任务已激活，执行记录已生成'))
      void reload()
      refreshIfCurrent(record.id)
    } catch {
      // Request layer already reports the server message.
    }
  }

  const complete = async (record: HermesTask) => {
    try {
      await completeHermesTask(record.id)
      message.success(t('任务已完成，未执行的执行记录已标记为失败'))
      void reload()
      refreshIfCurrent(record.id)
    } catch {
      // Request layer already reports the server message.
    }
  }

  const cancel = async (record: HermesTask) => {
    try {
      await cancelHermesTask(record.id)
      message.success(t('任务已取消'))
      void reload()
      refreshIfCurrent(record.id)
    } catch {
      // Request layer already reports the server message.
    }
  }

  const advanceRun = (run: HermesTaskRun, to: HermesTaskRunStatus) => {
    if (to === 'completed' || to === 'failed') {
      setResultTarget({ run, to })
      setResultText('')
      return
    }
    // 走到这里 to 只可能是 'running'（completed/failed 已在上方分支返回）
    void updateHermesTaskRun(run.id, { status: to as 'running' })
      .then(() => {
        message.success(t('执行记录已更新'))
        if (detail) void refreshDetail(detail.task.id)
      })
      .catch(() => {
        // Request layer already reports the server message.
      })
  }

  const submitRunResult = async () => {
    if (!resultTarget) return
    const text = resultText.trim()
    if (!text) {
      message.warning(t('结果不能为空'))
      return
    }
    setResultSubmitting(true)
    try {
      await updateHermesTaskRun(resultTarget.run.id, { status: resultTarget.to, result: text })
      message.success(t('执行记录已更新'))
      setResultTarget(null)
      if (detail) void refreshDetail(detail.task.id)
    } catch {
      // Request layer already reports the server message.
    } finally {
      setResultSubmitting(false)
    }
  }

  const columns: ColumnsType<HermesTask> = [
    { title: t('任务'), dataIndex: 'name', render: (value: string, record) => <a onClick={() => void openDetail(record.id)}>{value}</a> },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: HermesTaskStatus) => <StatusPill tone={taskStateTone(value)} label={taskStateLabels[value] ?? value} />,
    },
    {
      title: t('执行进度'),
      dataIndex: 'run_total',
      width: 110,
      render: (_, record) =>
        record.run_total ? `${record.run_done ?? 0}/${record.run_total}` : <span className="cell-muted">—</span>,
    },
    {
      title: t('目标集群'),
      dataIndex: 'cluster_name',
      width: 140,
      // 后端 HermesTaskView 已带 cluster_name，不再客户端查 clusters 表（超 100 / 集群已删会退化成 #id）
      render: (value: string, record) =>
        value || (record.cluster_id ? `#${record.cluster_id}` : <span className="cell-muted">{t('未指定')}</span>),
    },
    { title: t('描述'), dataIndex: 'description', ellipsis: true, render: (value: string) => <span className="cell-muted">{value || '—'}</span> },
    { title: t('创建时间'), dataIndex: 'created_at', width: 160, render: (value: string) => formatDateTime(value) },
    {
      title: t('操作'),
      width: compactActions ? 48 : 132,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'detail', label: t('详情'), icon: <EyeOutlined />, onClick: () => { void openDetail(record.id) } },
            {
              key: 'activate',
              label: t('激活'),
              icon: <PlayCircleOutlined />,
              show: canWrite && record.status === 'pending',
              onClick: () => { void activate(record) },
            },
            {
              key: 'complete',
              label: t('完成'),
              icon: <CheckOutlined />,
              show: canWrite && record.status === 'active',
              onClick: () => { void complete(record) },
            },
            {
              key: 'cancel',
              label: t('取消'),
              icon: <StopOutlined />,
              danger: true,
              show: canWrite && (record.status === 'pending' || record.status === 'active'),
              onClick: () => { void cancel(record) },
            },
          ]}
        />
      ),
    },
  ]

  const runColumns: ColumnsType<HermesTaskRun & { instance_name: string }> = [
    {
      title: t('实例'),
      dataIndex: 'instance_name',
      render: (value: string, record) => (
        <div>
          <div>{value || `#${record.instance_id}`}</div>
          <span className="cell-mono cell-muted">id {record.instance_id}</span>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: HermesTaskRunStatus) => <StatusPill tone={taskStateTone(value)} label={taskRunStateLabels[value] ?? value} />,
    },
    { title: t('结果'), dataIndex: 'result', ellipsis: true, render: (value: string) => (value ? <span className="cell-muted">{value}</span> : '—') },
    { title: t('开始'), dataIndex: 'started_at', width: 160, render: (value?: string) => (value ? formatDateTime(value) : '—') },
    { title: t('结束'), dataIndex: 'finished_at', width: 160, render: (value?: string) => (value ? formatDateTime(value) : '—') },
    {
      title: t('操作'),
      width: compactActions ? 48 : 120,
      align: 'center' as const,
      render: (_, record) => {
        const actions: { to: HermesTaskRunStatus; label: string; icon: ReactNode; danger?: boolean }[] = []
        if (record.status === 'assigned') {
          actions.push(
            { to: 'running', label: t('开始执行'), icon: <PlayCircleOutlined /> },
            { to: 'failed', label: t('标记失败'), icon: <StopOutlined />, danger: true },
          )
        } else if (record.status === 'running') {
          actions.push(
            { to: 'completed', label: t('完成'), icon: <CheckOutlined /> },
            { to: 'failed', label: t('失败'), icon: <StopOutlined />, danger: true },
          )
        }
        if (!canWrite || actions.length === 0) return null
        return (
          <TableRowActions
            menuOnly={compactActions}
            maxInline={2}
            actions={actions.map(({ to, label, icon, danger }) => ({
              key: to,
              label,
              icon,
              danger,
              onClick: () => advanceRun(record, to),
            }))}
          />
        )
      },
    },
  ]

  return (
    <ListPageShell
      className="hermes-collaborations-page"
      toolbar={
      <TableToolbar
        title="多 Agent 任务"
        total={total}
        icon={<ApartmentOutlined />}
        gradient="linear-gradient(135deg, #6366f1, #0ea5e9)"
        glow="rgba(99, 102, 241, 0.28)"
        description={t('控制面编排：创建任务、指派集群、追踪执行记录。端侧执行由员工手工触发。')}
        extra={
          <Space wrap>
            <Select<HermesTaskStatus | ''>
              value={status}
              onChange={(value) => { setStatus(value); pagination.resetPagination() }}
              options={STATUS_FILTERS.map((f) => ({ value: f.value, label: t(f.label) }))}
              style={{ width: 110 }}
            />
            <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void clusters.reload(); void policies.reload() }} loading={loading} />
            {canWrite && (
              <Tooltip title={clusterOptions.length === 0 ? t('暂无启用集群，请先到「Agent 集群」创建') : ''}>
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate} disabled={clusterOptions.length === 0}>
                  {t('新建任务')}
                </Button>
              </Tooltip>
            )}
          </Space>
        }
      />}
    >

        <Table
          rowKey="id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={tasks}
          scroll={{ x: 900 }}
          locale={{ emptyText: <GlassEmpty text="暂无任务" compact /> }}
          pagination={{
            total,
            current: pagination.page,
            pageSize: pagination.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => pagination.setPage(page, page_size),
          }}
        />

      <Modal
        title={t('新建多 Agent 任务')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSubmit()}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label={t('任务名')} rules={[{ required: true, message: t('请输入任务名') }, { max: 120 }]}>
            <Input placeholder={t('如：Q3 市场调研分解')} />
          </Form.Item>
          <Form.Item
            name="description"
            label={t('任务正文')}
            rules={[{ required: true, message: t('请输入任务正文') }]}
            extra={t('服务端明文存储，供拆解、审批与审计。')}
          >
            <Input.TextArea rows={5} maxLength={10000} placeholder={t('任务目标、拆解要求与验收口径…')} showCount />
          </Form.Item>
          <Form.Item
            name="cluster_id"
            label={t('目标集群')}
            rules={[
              { required: true, message: t('请选择集群') },
              // 防 cluster_id=0：无集群时 openCreate 已拦截，这里兜底确保提交真实集群
              { validator: (_, value: number | undefined) => (value && value > 0 ? Promise.resolve() : Promise.reject(new Error(t('请选择有效的集群')))) },
            ]}
          >
            <Select
              options={clusterOptions}
              placeholder={t('选择已启用的 Agent 集群')}
              notFoundContent={t('暂无启用集群，请先到「Agent 集群」创建')}
              onPopupScroll={clusters.loadMore}
              loading={clusters.loading}
            />
          </Form.Item>
          <Form.Item name="policy_id" label={t('dispatcher 策略（可选）')} extra={t('留空则沿用集群策略。仅 company_cluster 策略可选。')}>
            <Select
              allowClear
              options={clusterPolicies.map((p) => ({ value: p.id, label: `${p.name} · R${p.revision}` }))}
              placeholder={t('沿用集群策略')}
              notFoundContent={t('暂无 company_cluster 策略')}
              onPopupScroll={policies.loadMore}
              loading={policies.loading}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer title={detail ? t('任务详情 · {{name}}', { name: detail.task.name }) : t('任务详情')} open={drawerOpen} onClose={() => setDrawerOpen(false)} width={720}>
        {detail && (
          <div>
            <Space size={16} wrap style={{ marginBottom: 16 }}>
              <StatusPill tone={taskStateTone(detail.task.status)} label={taskStateLabels[detail.task.status] ?? detail.task.status} />
              {detail.cluster && <Tag color="geekblue">{t('集群：{{name}}', { name: detail.cluster.name })}</Tag>}
              {detail.policy && <Tag>{t('策略：{{name}} · R{{rev}}', { name: detail.policy.name, rev: detail.policy.revision })}</Tag>}
            </Space>
            <div className="cell-muted" style={{ marginBottom: 16, whiteSpace: 'pre-wrap' }}>
              {detail.task.description}
            </div>
            <Table
              rowKey="id"
              size="small"
              loading={detailLoading}
              columns={runColumns}
              dataSource={detail.runs}
              pagination={false}
              scroll={{ x: 760 }}
              locale={{ emptyText: <GlassEmpty text="该任务还没有执行记录" compact /> }}
            />
          </div>
        )}
      </Drawer>

      <Modal
        title={resultTarget?.to === 'failed' ? t('填写失败原因') : t('填写执行结果')}
        open={resultTarget !== null}
        onCancel={() => setResultTarget(null)}
        onOk={() => void submitRunResult()}
        confirmLoading={resultSubmitting}
        destroyOnHidden
      >
        <Input.TextArea
          value={resultText}
          onChange={(e) => setResultText(e.target.value)}
          rows={4}
          maxLength={4000}
          showCount
          placeholder={resultTarget?.to === 'failed' ? t('失败原因（必填）') : t('结果摘要（必填）')}
        />
      </Modal>
    </ListPageShell>
  )
}
