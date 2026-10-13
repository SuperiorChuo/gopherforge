import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, Modal, Select, Space, Switch, Table, Tag, Tooltip , Grid } from 'antd'
import { DesktopOutlined, PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  listHermesInstances,
  createHermesInstance,
  updateHermesInstance,
  deleteHermesInstance,
  listHermesDevices,
  listHermesPolicies,
  listHermesClusters,
  type HermesInstance,
  type HermesDevice,
  type HermesPolicy,
  type HermesCluster,
} from '@/api/ai/hermes'
import { getUserList } from '@/api/system/user'
import type { SystemUser } from '@/types'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { fetchAllHermesPages, HERMES_INSTANCE_STATUS_LABELS, instanceFormValues, instancePayload } from '../admin-contract'
import { useIncrementalList } from '../useIncrementalList'
import { stateTone } from '../_shared'

export default function HermesInstancesPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<HermesInstance | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const { hasPerm } = usePermission()

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesInstances(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: instances, total, loading, reload } = useTableQuery({ params, fetcher })

  // 工具栏聚合：running/unassigned 来自全量统计（逐页真实拉取，不放大 page_size）
  const [instanceStats, setInstanceStats] = useState({ running: 0, unassigned: 0 })
  const loadStats = useCallback(async () => {
    try {
      const all = await fetchAllHermesPages<HermesInstance>((page, pageSize) =>
        listHermesInstances({ page, page_size: pageSize }))
      setInstanceStats({
        running: all.filter((item) => item.enabled && item.status === 'running').length,
        unassigned: all.filter((item) => !item.device_id).length,
      })
    } catch {
      // 统计失败保留上次值；请求层已提示服务端消息
    }
  }, [])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  // 辅助选择器：设备/策略/集群按页追加；用户远程搜索 + 按页追加（域外契约独立处理）
  const devices = useIncrementalList<HermesDevice>(
    useCallback((page, pageSize) => listHermesDevices({ page, page_size: pageSize }), []),
  )
  const policies = useIncrementalList<HermesPolicy>(
    useCallback((page, pageSize) => listHermesPolicies({ page, page_size: pageSize }), []),
  )
  const clusters = useIncrementalList<HermesCluster>(
    useCallback((page, pageSize) => listHermesClusters({ page, page_size: pageSize }), []),
  )
  const users = useIncrementalList<SystemUser>(
    useCallback((page, pageSize, keyword) => getUserList({ page, page_size: pageSize, keyword }), []),
  )

  const availableDevices = useMemo(
    () => devices.items.filter((device) => !device.revoked_at),
    [devices.items],
  )
  const companyPolicies = useMemo(
    () => policies.items.filter((policy) => policy.profile === 'company'),
    [policies.items],
  )

  const deviceName = useMemo(() => {
    const map = new Map<string, string>()
    devices.items.forEach((device) => map.set(device.device_id, device.device_name || device.device_id))
    return map
  }, [devices.items])

  const policyName = useMemo(() => {
    const map = new Map<number, string>()
    policies.items.forEach((policy) => map.set(policy.id, `${policy.name} · R${policy.revision}`))
    return map
  }, [policies.items])

  const userName = useMemo(() => {
    const map = new Map<number, string>()
    users.items.forEach((user) => map.set(user.id, user.nickname || user.username))
    return map
  }, [users.items])

  const clusterName = useMemo(() => {
    const map = new Map<number, string>()
    clusters.items.forEach((cluster) => map.set(cluster.id, cluster.name))
    return map
  }, [clusters.items])

  const openModal = (record?: HermesInstance) => {
    setEditRecord(record ?? null)
    form.setFieldsValue(instanceFormValues(record))
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      // 未选节点/策略/集群时显式传空值：后端据此判定「未放置」「回落权益策略」「未归组」
      const payload = instancePayload(values)
      if (editRecord) {
        await updateHermesInstance(editRecord.id, payload)
        message.success(t('实例已更新'))
      } else {
        await createHermesInstance(payload)
        message.success(t('实例已创建'))
      }
      setModalOpen(false)
      void reload()
    } catch {
      // 单实例守卫等业务错误由请求层弹出服务端文案
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteHermesInstance(id)
      message.success(t('实例已删除'))
      void reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const columns: ColumnsType<HermesInstance> = [
    {
      title: t('实例'),
      dataIndex: 'name',
      render: (value: string, record) => (
        <div>
          <div>{value}</div>
          <span className="cell-mono cell-muted">{record.role}</span>
        </div>
      ),
    },
    {
      title: t('负责人'),
      dataIndex: 'owner_user_id',
      width: 140,
      render: (value: number) => userName.get(value) ?? (value ? `#${value}` : t('未指派')),
    },
    {
      title: t('运行节点'),
      dataIndex: 'device_id',
      render: (value: string) =>
        value ? (
          <span className="cell-mono">{deviceName.get(value) ?? value}</span>
        ) : (
          <Tag>{t('未放置')}</Tag>
        ),
    },
    {
      title: t('模型策略'),
      dataIndex: 'policy_id',
      render: (value: number) =>
        value ? (
          policyName.get(value) ?? `#${value}`
        ) : (
          <Tooltip title={t('未单独指定，沿用该负责人的员工权益策略')}>
            <span className="cell-dim">{t('跟随权益')}</span>
          </Tooltip>
        ),
    },
    {
      title: t('Agent 集群'),
      dataIndex: 'cluster_id',
      width: 150,
      render: (value: number) => value ? (clusterName.get(value) ?? `#${value}`) : <Tag>{t('未归组')}</Tag>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 110,
      render: (value: string, record) =>
        record.enabled ? (
          <StatusPill
            tone={stateTone(value === 'running' ? 'active' : value)}
            label={t(HERMES_INSTANCE_STATUS_LABELS[value as keyof typeof HERMES_INSTANCE_STATUS_LABELS] ?? value)}
          />
        ) : (
          <StatusPill tone="muted" label={t('已停用')} pulse={false} />
        ),
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('ai:hermes:instance:write'),
              onClick: () => openModal(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('ai:hermes:instance:write'),
              confirm: {
                title: t('确认删除该实例？'),
                description: t('删除后该节点将回落到员工权益策略'),
              },
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  const running = instanceStats.running
  const unassigned = instanceStats.unassigned

  return (
    <ListPageShell
      className="hermes-instances-page"
      toolbar={
      <TableToolbar
        title="实例管理"
        total={total}
        icon={<DesktopOutlined />}
        gradient="linear-gradient(135deg, #8b5cf6, #2563eb)"
        glow="rgba(139, 92, 246, 0.26)"
        description={t('{{a}} 个运行中 · {{b}} 个未放置到节点', { a: running, b: unassigned })}
        extra={
          <Space wrap>
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void loadStats(); void devices.reload(); void policies.reload(); void clusters.reload(); void users.reload() }} loading={loading} />
            </Tooltip>
            {hasPerm('ai:hermes:instance:write') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>{t('新建实例')}</Button>
            )}
          </Space>
        }
      />}
    >

        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          columns={columns}
          dataSource={instances}
          scroll={{ x: 1080 }}
          locale={{ emptyText: <GlassEmpty text="暂无 Agent 实例，先新建一个并放置到运行节点" compact /> }}
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
        title={editRecord ? t('编辑实例 · {{name}}', { name: editRecord.name }) : t('新建实例')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="name"
            label={t('实例名')}
            rules={[{ required: true, message: t('请输入实例名') }, { max: 80, message: t('最长 80 字') }]}
            tooltip={t('租户内唯一；派发任务时用它指认 Agent')}
          >
            <Input placeholder={t('如：研发助手 A')} />
          </Form.Item>
          <Form.Item name="role" label={t('角色')} rules={[{ max: 40, message: t('最长 40 字') }]} tooltip={t('自定义短文本，如 研发 / 测试 / 文档')}>
            <Input placeholder="default" />
          </Form.Item>
          <Form.Item name="owner_user_id" label={t('负责人')}>
            <Select
              allowClear
              showSearch
              filterOption={false}
              onSearch={(keyword) => void users.reload(keyword)}
              onPopupScroll={users.loadMore}
              loading={users.loading}
              placeholder={t('请选择负责人')}
              options={users.items.map((user) => ({ value: user.id, label: user.nickname || user.username }))}
            />
          </Form.Item>
          <Form.Item
            name="device_id"
            label={t('运行节点')}
            tooltip={t('留空表示先建好不放置。一个节点当前只能有一个启用实例——manager 尚未支持一节点多实例')}
          >
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              onPopupScroll={devices.loadMore}
              loading={devices.loading}
              placeholder={t('留空 = 暂不放置')}
              options={availableDevices.map((device) => ({
                value: device.device_id,
                label: device.device_name || device.device_id,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="policy_id"
            label={t('模型策略')}
            tooltip={t('留空则沿用该负责人的员工权益策略；选定后覆盖之')}
          >
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              onPopupScroll={policies.loadMore}
              loading={policies.loading}
              placeholder={t('留空 = 跟随员工权益')}
              options={companyPolicies.map((policy) => ({
                value: policy.id,
                label: `${policy.name} · R${policy.revision}`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="cluster_id"
            label={t('Agent 集群')}
            tooltip={t('留空表示不参与多 Agent 协作；可随时换组或解除归组')}
          >
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              onPopupScroll={clusters.loadMore}
              loading={clusters.loading}
              placeholder={t('留空 = 未归组')}
              options={clusters.items.map((cluster) => ({
                value: cluster.id,
                label: `${cluster.name}${cluster.slug ? ` · Kanban/${cluster.slug}` : ''}`,
                disabled: !cluster.enabled,
              }))}
            />
          </Form.Item>
          <Form.Item name="enabled" label={t('启用')} valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <div className="form-hint">{t('停用实例会让其所在节点停止运行 Hermes，而不是退回权益策略。')}</div>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
