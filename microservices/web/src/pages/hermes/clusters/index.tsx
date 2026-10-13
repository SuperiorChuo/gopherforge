import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Drawer, Form, Input, Modal, Select, Space, Switch, Table, Tag , Grid } from 'antd'
import { ClusterOutlined, PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  createHermesCluster,
  deleteHermesCluster,
  getHermesCluster,
  listHermesClusters,
  listHermesPolicies,
  updateHermesCluster,
  type HermesCluster,
  type HermesClusterPayload,
  type HermesInstance,
  type HermesPolicy,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import {
  beginHermesRequest,
  clusterFormValues,
  clusterPayload,
  HERMES_INSTANCE_STATUS_LABELS,
  invalidateHermesRequests,
  runLatestHermesRequest,
} from '../admin-contract'
import { useIncrementalList } from '../useIncrementalList'

export default function HermesClustersPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:cluster:write')

  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<HermesCluster | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const [memberOpen, setMemberOpen] = useState(false)
  const [memberCluster, setMemberCluster] = useState<HermesCluster | null>(null)
  const [members, setMembers] = useState<HermesInstance[]>([])
  const [memberLoading, setMemberLoading] = useState(false)
  const memberRequestIDRef = useRef(0)

  // dispatcher 策略下拉：按页追加加载
  const policies = useIncrementalList<HermesPolicy>(
    useCallback((page, pageSize) => listHermesPolicies({ page, page_size: pageSize }), []),
  )
  const clusterPolicies = useMemo(() => policies.items.filter((p) => p.profile === 'company_cluster'), [policies.items])

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesClusters(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: clusters, total, loading, reload } = useTableQuery({ params, fetcher })

  const openCreate = () => {
    setEditRecord(null)
    form.setFieldsValue(clusterFormValues())
    setModalOpen(true)
  }

  const openEdit = (record: HermesCluster) => {
    setEditRecord(record)
    form.setFieldsValue(clusterFormValues(record))
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const payload: HermesClusterPayload = clusterPayload(values)
      if (editRecord) {
        await updateHermesCluster(editRecord.id, payload)
        message.success(t('集群已更新'))
      } else {
        await createHermesCluster(payload)
        message.success(t('集群已创建'))
      }
      setModalOpen(false)
      void reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteHermesCluster(id)
      message.success(t('集群已删除'))
      void reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const openMembers = async (record: HermesCluster) => {
    const requestID = beginHermesRequest(memberRequestIDRef)
    setMemberCluster(record)
    setMemberOpen(true)
    setMembers([])
    setMemberLoading(true)
    await runLatestHermesRequest(
      memberRequestIDRef,
      requestID,
      () => getHermesCluster(record.id),
      (result) => setMembers(result.instances ?? []),
      () => setMemberLoading(false),
    ).catch(() => {
      // Request layer already reports the server message.
    })
  }

  const closeMembers = () => {
    invalidateHermesRequests(memberRequestIDRef)
    setMemberOpen(false)
    setMemberCluster(null)
    setMembers([])
    setMemberLoading(false)
  }

  const policyLabel = (policyId: number) => {
    const policy = policies.items.find((p) => p.id === policyId)
    return policy ? `${policy.name} · R${policy.revision}` : (policyId ? `#${policyId}` : '未绑定')
  }

  const columns: ColumnsType<HermesCluster> = [
    {
      title: t('集群'),
      dataIndex: 'name',
      render: (value: string, record) => (
        <div>
          <div>{value}</div>
          {record.description && <span className="cell-muted">{record.description}</span>}
        </div>
      ),
    },
    {
      title: t('协作通道'),
      dataIndex: 'slug',
      width: 180,
      render: (value: string) => value ? <Tag color="cyan">Kanban · {value}</Tag> : <Tag>Push</Tag>,
    },
    {
      title: t('dispatcher 策略'),
      dataIndex: 'policy_id',
      width: 220,
      render: (value: number) =>
        value ? <Tag color="purple">{t(policyLabel(value))}</Tag> : <Tag>{t('未绑定')}</Tag>,
    },
    {
      title: t('成员实例'),
      dataIndex: 'instance_count',
      width: 120,
      render: (value: number, record) => (
        <Button type="link" size="small" onClick={() => void openMembers(record)}>
          {t('{{n}} 个', { n: value ?? 0 })}
        </Button>
      ),
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 90,
      render: (value: boolean) => (value ? <Tag color="success">{t('启用')}</Tag> : <Tag>{t('停用')}</Tag>),
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 160, render: (value: string) => formatDateTime(value) },
    {
      title: t('操作'),
      width: compactActions ? 48 : 88,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, show: canWrite, onClick: () => openEdit(record) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: canWrite,
              confirm: { title: t('删除该集群？'), description: t('仅允许删除没有成员实例和任务的空集群。') },
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-clusters-page"
      toolbar={
      <TableToolbar
        title="Agent 集群"
        total={total}
        icon={<ClusterOutlined />}
        gradient="linear-gradient(135deg, #8b5cf6, #2563eb)"
        glow="rgba(139, 92, 246, 0.26)"
        description={t('company_cluster 分组：把逻辑 Agent 实例归组并绑定 dispatcher 策略。')}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void policies.reload() }} loading={loading} />
            {canWrite && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('新建集群')}
              </Button>
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
          dataSource={clusters}
          scroll={{ x: 900 }}
          locale={{ emptyText: <GlassEmpty text="暂无集群" compact /> }}
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
        title={editRecord ? t('编辑集群 · {{name}}', { name: editRecord.name }) : t('新建 Agent 集群')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSubmit()}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label={t('集群名')} rules={[{ required: true, message: t('请输入集群名') }, { max: 80 }]}>
            <Input placeholder={t('如：研发协同集群')} />
          </Form.Item>
          <Form.Item
            name="slug"
            label={t('Kanban Board Slug（可选）')}
            extra={t('非空启用 Kanban 通道，留空使用 Push；有执行中任务时不能切换。')}
            rules={[
              { max: 64, message: t('最长 64 个字符') },
              { pattern: /^[a-z0-9][a-z0-9_-]{0,63}$/, message: t('仅支持小写字母、数字、下划线和短横线，且须以字母或数字开头') },
            ]}
          >
            <Input allowClear placeholder={t('如：research-team')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={3} maxLength={10000} />
          </Form.Item>
          <Form.Item name="policy_id" label={t('dispatcher 策略（可选）')} extra={t('留空则集群不绑定策略。仅 company_cluster 策略可选。')}>
            <Select
              allowClear
              onPopupScroll={policies.loadMore}
              loading={policies.loading}
              options={clusterPolicies.map((p) => ({ value: p.id, label: `${p.name} · R${p.revision}` }))}
              placeholder={t('选择 company_cluster 策略')}
              notFoundContent={t('暂无 company_cluster 策略')}
            />
          </Form.Item>
          <Form.Item name="enabled" label={t('启用')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={memberCluster ? t('集群成员 · {{name}}', { name: memberCluster.name }) : t('集群成员')}
        open={memberOpen}
        onClose={closeMembers}
        width={560}
      >
        <Table
          rowKey="id"
          size="small"
          loading={memberLoading}
          pagination={false}
          columns={[
            { title: t('实例'), dataIndex: 'name', render: (value: string, record: HermesInstance) => <div><div>{value}</div><span className="cell-mono cell-muted">{record.role}</span></div> },
            { title: t('节点'), dataIndex: 'device_id', render: (value: string) => (value ? <span className="cell-mono">{value}</span> : <Tag>{t('未放置')}</Tag>) },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 110,
              render: (value: string) => (
                <Tag>{t(HERMES_INSTANCE_STATUS_LABELS[value as keyof typeof HERMES_INSTANCE_STATUS_LABELS] ?? value)}</Tag>
              ),
            },
          ]}
          dataSource={members}
          locale={{ emptyText: <GlassEmpty text="该集群暂无成员实例" compact /> }}
        />
        <div className="cell-muted" style={{ marginTop: 12 }}>
          {t('实例归属集群在「实例管理」中编辑（cluster_id 字段）。')}
        </div>
      </Drawer>
    </ListPageShell>
  )
}
