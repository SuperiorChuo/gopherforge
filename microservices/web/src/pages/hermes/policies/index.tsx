// 策略管理：发布签名的期望状态（版本、模型、工具白名单、隐私、租约）。
// 由原 /ai/hermes 单页的「策略」Tab 拆出；制品下拉需要制品列表，故一并拉取。
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, Tooltip , Grid } from 'antd'
import { PlusOutlined, ReloadOutlined, SafetyCertificateOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  createHermesPolicy,
  updateHermesPolicy,
  deleteHermesPolicy,
  listHermesArtifacts,
  listHermesPolicies,
  type HermesArtifact,
  type HermesDesiredState,
  type HermesPolicy,
  type HermesPolicyProfile,
} from '@/api/ai/hermes'
import { getEnabledAIModels, type AIModel } from '@/api/ai/model'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { useTableQuery } from '@/hooks/useTableQuery'
import { isPolicyToolsetValid, policyFormValues, policyPayload } from '../admin-contract'
import { useIncrementalList } from '../useIncrementalList'
import { p1Toolsets, p2ClusterToolsets, stateLabels, stateTone } from '../_shared'

export default function HermesPoliciesPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:policy:write')
  const [saving, setSaving] = useState(false)
  const [models, setModels] = useState<AIModel[]>([])
  const [open, setOpen] = useState(false)
  const [editingPolicy, setEditingPolicy] = useState<HermesPolicy | null>(null)
  const [form] = Form.useForm()
  const selectedProfile = Form.useWatch<HermesPolicyProfile>('profile', form) ?? 'company'
  const availableToolsets = useMemo(
    () => selectedProfile === 'company_cluster' ? p2ClusterToolsets : p1Toolsets,
    [selectedProfile],
  )

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesPolicies(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: policies, total, loading, reload } = useTableQuery({ params, fetcher })

  // 制品下拉：按页追加加载，滚动触底拉下一页
  const artifacts = useIncrementalList<HermesArtifact>(
    useCallback((page, pageSize) => listHermesArtifacts({ page, page_size: pageSize }), []),
  )

  const loadModels = useCallback(async () => {
    try {
      setModels(await getEnabledAIModels())
    } catch {
      // Request layer already reports the server message.
    }
  }, [])

  useEffect(() => {
    void loadModels()
  }, [loadModels])

  const openModal = (policy?: HermesPolicy) => {
    if (!canWrite) return
    setEditingPolicy(policy ?? null)
    form.resetFields()
    form.setFieldsValue(policyFormValues(policy, p1Toolsets))
    setOpen(true)
  }

  const submit = async () => {
    if (!canWrite) return
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSaving(true)
    try {
      const base = policyPayload(values, p1Toolsets, p2ClusterToolsets)
      if (editingPolicy) {
        await updateHermesPolicy(editingPolicy.id, base)
        message.success(t('策略已更新'))
      } else {
        await createHermesPolicy(base)
        message.success(t('Hermes 策略已发布'))
      }
      setOpen(false)
      await reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (policy: HermesPolicy) => {
    if (!canWrite) return
    try {
      await deleteHermesPolicy(policy.id)
      message.success(t('策略已删除'))
      await reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const columns: ColumnsType<HermesPolicy> = [
    { title: 'Revision', dataIndex: 'revision', width: 90 },
    { title: t('名称'), dataIndex: 'name' },
    {
      title: t('策略类型'),
      dataIndex: 'profile',
      width: 130,
      render: (value: HermesPolicyProfile) => value === 'company_cluster'
        ? <Tag color="purple">dispatcher</Tag>
        : <Tag color="blue">{t('设备')}</Tag>,
    },
    {
      title: t('期望状态'),
      dataIndex: 'desired_state',
      width: 110,
      render: (value: string) => <StatusPill tone={stateTone(value)} label={stateLabels[value] ?? value} pulse={false} />,
    },
    { title: t('模型'), dataIndex: 'model_name', width: 160, render: (value: string) => value || t('未分配') },
    { title: t('制品 ID'), dataIndex: 'artifact_id', width: 90, render: (value: number) => value || t('未分配') },
    { title: t('租约'), dataIndex: 'lease_seconds', width: 90, render: (value: number) => t('{{n}} 分钟', { n: value / 60 }) },
    { title: t('发布时间'), dataIndex: 'created_at', width: 160, render: formatDateTime },
    {
      title: t('操作'), key: 'action', width: compactActions ? 48 : 88, align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: canWrite,
              onClick: () => openModal(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: canWrite,
              confirm: t('确认删除该策略？'),
              onClick: () => { void handleDelete(record) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-policies-page"
      toolbar={
      <TableToolbar
        title="实例模型"
        total={total}
        icon={<SafetyCertificateOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={policies[0] ? t('当前最新 R{{n}}', { n: policies[0].revision }) : t('尚未发布任何策略')}
        extra={
          <Space>
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void artifacts.reload(); void loadModels() }} loading={loading} />
            </Tooltip>
            {canWrite && (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>
                {t('发布模型策略')}
              </Button>
            )}
          </Space>
        }
      />}
    >

        <Table
          rowKey="id"
          scroll={{ x: 800 }}
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={policies}
          locale={{ emptyText: <GlassEmpty text="暂无 Hermes 策略" compact /> }}
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
        title={editingPolicy ? t('编辑策略') : t('发布新策略')}
        open={open}
        onOk={() => void submit()}
        onCancel={() => setOpen(false)}
        confirmLoading={saving}
        width={620}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }} preserve={false}>
          <Form.Item name="name" label={t('策略名称')} rules={[{ required: true, max: 80 }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="profile"
            label={t('策略类型')}
            rules={[{ required: true }]}
            extra={selectedProfile === 'company_cluster'
              ? t('dispatcher 策略只用于 Agent 集群控制面，不可直接绑定设备或实例。')
              : t('设备策略可绑定权益或实例，并下发到 Hermes Manager。')}
          >
            <Select
              disabled={Boolean(editingPolicy)}
              onChange={(profile: HermesPolicyProfile) => {
                form.setFieldValue('toolsets', profile === 'company_cluster' ? p2ClusterToolsets : p1Toolsets)
              }}
              options={[
                { value: 'company', label: t('设备策略（company）') },
                { value: 'company_cluster', label: t('集群 dispatcher（company_cluster）') },
              ]}
            />
          </Form.Item>
          <Form.Item name="desired_state" label={t('期望状态')} rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'active' satisfies HermesDesiredState, label: t('启用') },
                { value: 'stopped' satisfies HermesDesiredState, label: t('停止') },
                { value: 'revoked' satisfies HermesDesiredState, label: t('吊销') },
              ]}
            />
          </Form.Item>
          <Form.Item name="artifact_id" label={t('Hermes 制品')}>
            <Select
              onPopupScroll={artifacts.loadMore}
              loading={artifacts.loading}
              options={[
                { value: 0, label: t('暂不安装') },
                ...artifacts.items
                  .filter((item) => item.kind === 'hermes' && item.status === 'approved')
                  .map((item) => ({ value: item.id, label: `${item.version} · ${item.platform}/${item.arch}` })),
              ]}
            />
          </Form.Item>
          <Form.Item
            name="model_id"
            label={t('企业模型')}
            extra={t('员工可用哪个模型由策略决定，不由本机配置决定。留空则用服务端全局 AI 配置。')}
          >
            <Select
              allowClear
              placeholder={t('使用全局 AI 配置')}
              options={models.map((m) => ({
                value: m.id,
                label: `${m.display_name || m.name} · ${m.provider}`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="model_name"
            label={t('模型标识（可选）')}
            extra={t('下发给 Hermes 的 model 名称；留空时用上面所选模型的配置名。')}
          >
            <Input placeholder={t('留空自动填充')} />
          </Form.Item>
          <Form.Item
            name="model_base_url"
            label={t('模型代理地址')}
            extra={t('Hermes 调用的入站地址，通常是「网关」/api/v1/ai/hermes/proxy/v1')}
          >
            <Input placeholder="https://.../api/v1/ai/hermes/proxy/v1" />
          </Form.Item>
          <Form.Item name="assistant_template_id" label={t('助手模板 ID')}>
            <Input />
          </Form.Item>
          <Form.Item name="assistant_template_revision" label={t('助手模板 Revision')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="lease_seconds" label={t('策略租约（秒）')} rules={[{ required: true }]}>
            <InputNumber min={300} max={86400} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="enable_knowledge_rag"
            label={t('企业知识库 RAG')}
            valuePropName="checked"
            extra={t('开启后 Hermes 模型代理在转发前检索企业知识库，把相关块注入回答上下文。服务端开关，不发往员工设备。')}
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="toolsets"
            label={t('工具白名单')}
            rules={[
              { required: true, message: t('请选择工具白名单') },
              {
                validator: async (_, value: string[] = []) => {
                  if (!isPolicyToolsetValid(selectedProfile, value, p1Toolsets, p2ClusterToolsets)) {
                    throw new Error(selectedProfile === 'company_cluster'
                      ? t('集群 dispatcher 工具集必须来自允许清单，且 delegation 必选')
                      : t('设备策略必须使用完整的 P1 工具集'))
                  }
                },
              },
            ]}
            extra={selectedProfile === 'company_cluster'
              ? t('delegation 必选；skills 与 mcp 可按需关闭。')
              : t('P1 工具集由 Manager 固定校验，不可裁剪。')}
          >
            <Select
              mode="multiple"
              disabled={selectedProfile === 'company'}
              options={availableToolsets.map((tool) => ({ value: tool, label: tool }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
