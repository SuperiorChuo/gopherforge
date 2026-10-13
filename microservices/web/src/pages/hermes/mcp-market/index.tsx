import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Form, Input, InputNumber, Modal, Radio, Select, Space, Table, Tag , Grid } from 'antd'
import { ApiOutlined, PlusOutlined, ReloadOutlined, EditOutlined, CheckOutlined, StopOutlined, DisconnectOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  approveHermesMcpServer,
  bindClusterMcpServer,
  listClusterMcpServers,
  listHermesClusters,
  listHermesMcpServers,
  rejectHermesMcpServer,
  revokeHermesMcpServer,
  unbindClusterMcpServer,
  updateHermesMcpServer,
  createHermesMcpServer,
  type HermesCluster,
  type HermesMcpServer,
  type HermesMcpServerPayload,
  type HermesMcpServerType,
  type HermesMcpStatus,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { fetchAllHermesPages } from '../admin-contract'
import { useIncrementalList } from '../useIncrementalList'

const STATUS_META: Record<HermesMcpStatus, { label: string; tone: 'success' | 'warning' | 'danger' | 'muted' }> = {
  pending: { label: '待审批', tone: 'warning' },
  approved: { label: '已批准', tone: 'success' },
  rejected: { label: '已拒绝', tone: 'danger' },
  revoked: { label: '已吊销', tone: 'danger' },
}

const TYPE_LABELS: Record<HermesMcpServerType, string> = {
  stdio: '本地命令',
  sse: 'SSE 远程',
  http: 'HTTP 远程',
}

export default function HermesMcpMarketPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:mcp:write')
  const canApprove = hasPerm('ai:hermes:mcp:approve')
  const canBind = hasPerm('ai:hermes:mcp:bind')

  const [loading, setLoading] = useState(false)
  const [servers, setServers] = useState<HermesMcpServer[]>([])
  const [total, setTotal] = useState(0)

  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<HermesMcpServer | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [transport, setTransport] = useState<HermesMcpServerType>('stdio')

  // 集群绑定视图
  const [selectedCluster, setSelectedCluster] = useState<number>()
  const [clusterServers, setClusterServers] = useState<HermesMcpServer[]>([])

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  // 手写状态页的请求代号门控：快速连点翻页时旧响应不得覆盖新页
  const loadGenerationRef = useRef(0)
  const loadData = useCallback(async (p: typeof params) => {
    const generation = loadGenerationRef.current + 1
    loadGenerationRef.current = generation
    setLoading(true)
    try {
      const serverResult = await listHermesMcpServers(p)
      if (generation !== loadGenerationRef.current) return
      setServers(serverResult.list ?? [])
      setTotal(serverResult.total ?? 0)
    } catch {
      // Request layer already reports the server message.
    } finally {
      if (generation === loadGenerationRef.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData(params)
  }, [loadData, params])

  // 待审批聚合：全量统计（逐页真实拉取，不放大 page_size 伪装全量）
  const [pendingCount, setPendingCount] = useState(0)
  const loadStats = useCallback(async () => {
    try {
      const all = await fetchAllHermesPages<HermesMcpServer>((page, pageSize) =>
        listHermesMcpServers({ page, page_size: pageSize }))
      setPendingCount(all.filter((s) => s.status === 'pending').length)
    } catch {
      // 统计失败保留上次值；请求层已提示服务端消息
    }
  }, [])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  // 集群下拉：按页追加加载
  const clusters = useIncrementalList<HermesCluster>(
    useCallback((page, pageSize) => listHermesClusters({ page, page_size: pageSize }), []),
  )

  // 绑定下拉：approved 服务器独立按页追加，不与主表分页纠缠
  const bindServers = useIncrementalList<HermesMcpServer>(
    useCallback((page, pageSize) => listHermesMcpServers({ page, page_size: pageSize }), []),
  )

  const loadClusterServers = useCallback(async (clusterId: number) => {
    try {
      setClusterServers(await listClusterMcpServers(clusterId))
    } catch {
      // Request layer already reports the server message.
    }
  }, [])

  useEffect(() => {
    if (selectedCluster) void loadClusterServers(selectedCluster)
  }, [selectedCluster, loadClusterServers])

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      message.success(ok)
      void loadData(params)
      void loadStats()
      if (selectedCluster) void loadClusterServers(selectedCluster)
    } catch {
      // Request layer already reports the server message.
    }
  }

  const openCreate = () => {
    setEditRecord(null)
    setTransport('stdio')
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: HermesMcpServer) => {
    setEditRecord(record)
    setTransport(record.type)
    form.setFieldsValue({
      name: record.name,
      display_name: record.display_name,
      description: record.description,
      type: record.type,
      command: record.command,
      args: record.args,
      url: record.url,
      timeout: record.timeout || undefined,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const payload: HermesMcpServerPayload = {
        name: values.name,
        display_name: values.display_name ?? '',
        description: values.description ?? '',
        type: values.type,
        command: values.type === 'stdio' ? values.command : '',
        args: values.type === 'stdio' ? (values.args ?? []) : [],
        url: values.type === 'stdio' ? '' : values.url,
        timeout: values.timeout ?? 0,
      }
      if (editRecord) {
        await updateHermesMcpServer(editRecord.id, payload)
        message.success(t('MCP 服务器已更新，等待重审批'))
      } else {
        await createHermesMcpServer(payload)
        message.success(t('MCP 服务器已录入，等待审批'))
      }
      setModalOpen(false)
      void loadData(params)
      void loadStats()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<HermesMcpServer> = [
    {
      title: t('服务器'),
      dataIndex: 'display_name',
      render: (value: string, record) => (
        <div>
          <div>{value || record.name}</div>
          <span className="cell-mono cell-muted">{record.name}</span>
        </div>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 100,
      render: (value: HermesMcpServerType) => <Tag>{t(TYPE_LABELS[value] ?? value)}</Tag>,
    },
    {
      title: t('端点'),
      dataIndex: 'url',
      ellipsis: true,
      render: (value: string, record) =>
        record.type === 'stdio'
          ? <span className="cell-mono cell-muted">{record.command} {record.args?.slice(1).join(' ')}</span>
          : <span className="cell-mono cell-muted">{value || '—'}</span>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: HermesMcpStatus) => {
        const meta = STATUS_META[value]
        return <StatusPill tone={meta.tone} label={meta.label} />
      },
    },
    { title: t('密钥版本'), dataIndex: 'secret_revision', width: 90, render: (value: number) => <span className="cell-mono cell-muted">r{value}</span> },
    { title: t('录入时间'), dataIndex: 'created_at', width: 160, render: (value: string) => formatDateTime(value) },
    {
      title: t('操作'),
      width: compactActions ? 48 : 132,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.display_name || record.name })}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, show: canWrite, onClick: () => openEdit(record) },
            {
              key: 'approve',
              label: t('批准'),
              icon: <CheckOutlined />,
              show: canApprove && record.status === 'pending',
              onClick: () => { void act(() => approveHermesMcpServer(record.id), t('已批准')) },
            },
            {
              key: 'reject',
              label: t('拒绝'),
              icon: <StopOutlined />,
              danger: true,
              show: canApprove && record.status === 'pending',
              onClick: () => { void act(() => rejectHermesMcpServer(record.id), t('已拒绝')) },
            },
            {
              key: 'revoke',
              label: t('吊销'),
              icon: <StopOutlined />,
              danger: true,
              show: canApprove && record.status === 'approved',
              confirm: { title: t('吊销该 MCP 服务器？'), description: t('已绑定集群的该服务器会停止下发。') },
              onClick: () => { void act(() => revokeHermesMcpServer(record.id), t('已吊销')) },
            },
          ]}
        />
      ),
    },
  ]

  const boundColumns: ColumnsType<HermesMcpServer> = [
    { title: t('服务器'), dataIndex: 'display_name', render: (value: string, record) => <div><div>{value || record.name}</div><span className="cell-mono cell-muted">{record.name}</span></div> },
    { title: t('类型'), dataIndex: 'type', width: 100, render: (value: HermesMcpServerType) => <Tag>{t(TYPE_LABELS[value] ?? value)}</Tag> },
    { title: t('状态'), dataIndex: 'status', width: 100, render: (value: HermesMcpStatus) => { const m = STATUS_META[value]; return <StatusPill tone={m.tone} label={m.label} /> } },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      render: (_, record) =>
        canBind ? (
          <TableRowActions
            maxInline={1}
            actions={[
              {
                key: 'unbind',
                label: t('解绑'),
                icon: <DisconnectOutlined />,
                danger: true,
                confirm: t('从该集群解绑？'),
                onClick: () => {
                  if (selectedCluster) void act(() => unbindClusterMcpServer(selectedCluster, record.id), t('已解绑'))
                },
              },
            ]}
          />
        ) : null,
    },
  ]

  const availableToBind = useMemo(
    () => bindServers.items.filter((s) => s.status === 'approved' && !clusterServers.some((b) => b.id === s.id)),
    [bindServers.items, clusterServers],
  )

  return (
    <div className="page-list">
      <TableToolbar
        title="MCP 工具"
        total={total}
        icon={<ApiOutlined />}
        gradient="linear-gradient(135deg, #ec4899, #f59e0b)"
        glow="rgba(236, 72, 153, 0.24)"
        description={t('{{n}} 个待审批 · 员工 Hermes 经批准工具连企业系统', { n: pendingCount })}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => { void loadData(params); void loadStats(); void clusters.reload() }} loading={loading} />
            {canWrite && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('录入 MCP 服务器')}
              </Button>
            )}
          </Space>
        }
      />

      <Card className="list-main-card">
        <Table
          rowKey="id"
          className="list-table"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={servers}
          scroll={{ x: 1000 }}
          locale={{ emptyText: <GlassEmpty text="暂无 MCP 服务器" compact /> }}
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
      </Card>

      <Card className="list-main-card" title={t('集群 MCP 绑定')} style={{ marginTop: 16 }}>
        <Space wrap style={{ marginBottom: 16 }}>
          <Select<number>
            placeholder={t('选择集群')}
            style={{ width: 260 }}
            value={selectedCluster}
            onChange={(value) => setSelectedCluster(value)}
            onPopupScroll={clusters.loadMore}
            loading={clusters.loading}
            options={clusters.items.filter((c) => c.enabled).map((c) => ({ value: c.id, label: c.name }))}
          />
          {canBind && selectedCluster && (
            <Select<number>
              placeholder={t('绑定已批准服务器')}
              style={{ width: 260 }}
              value={undefined}
              onChange={(serverId) => {
                if (serverId) void act(() => bindClusterMcpServer(selectedCluster, serverId), t('已绑定'))
              }}
              onPopupScroll={bindServers.loadMore}
              loading={bindServers.loading}
              options={availableToBind.map((s) => ({ value: s.id, label: `${s.display_name || s.name} · ${t(TYPE_LABELS[s.type])}` }))}
            />
          )}
        </Space>
        {selectedCluster ? (
          <Table
            rowKey="id"
            className="list-table"
            size="small"
            columns={boundColumns}
            dataSource={clusterServers}
            pagination={false}
            locale={{ emptyText: <GlassEmpty text="该集群未绑定 MCP 服务器" compact /> }}
          />
        ) : (
          <div className="cell-muted">{t('选择一个集群查看已绑定的 MCP 服务器。绑定后该集群所有员工 Hermes 都能连这些工具。')}</div>
        )}
      </Card>

      <Modal
        title={editRecord ? t('编辑 MCP 服务器 · {{name}}', { name: editRecord.name }) : t('录入 MCP 服务器')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSubmit()}
        confirmLoading={submitting}
        destroyOnHidden
        width={620}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label={t('服务器名（配置键）')} rules={[{ required: true, message: t('请输入服务器名') }, { pattern: /^[a-z0-9][a-z0-9._-]{0,79}$/, message: t('小写字母数字 + . _ -') }]}>
            <Input placeholder={t('如：company-crm')} disabled={!!editRecord} />
          </Form.Item>
          <Form.Item name="display_name" label={t('显示名')}>
            <Input placeholder={t('如：公司 CRM')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={2} />
          </Form.Item>
          <Form.Item name="type" label={t('传输类型')} initialValue="stdio">
            <Radio.Group onChange={(e) => setTransport(e.target.value)}>
              <Radio.Button value="stdio">{t('本地命令 (stdio)')}</Radio.Button>
              <Radio.Button value="http">HTTP</Radio.Button>
              <Radio.Button value="sse">SSE</Radio.Button>
            </Radio.Group>
          </Form.Item>
          {transport === 'stdio' && (
            <>
              <Form.Item name="command" label={t('命令')} rules={[{ required: true, message: t('请输入命令') }]}>
                <Select
                  options={[
                    { value: 'npx', label: 'npx' },
                    { value: 'uvx', label: 'uvx' },
                    { value: 'python', label: 'python' },
                    { value: 'python3', label: 'python3' },
                  ]}
                  placeholder={t('白名单命令（npx/uvx/python）')}
                />
              </Form.Item>
              <Form.Item name="args" label={t('参数')} extra={t('npx/uvx 需 -y「白名单包」；python 需 -m「白名单模块」')}>
                <Select mode="tags" placeholder={t('如：-y @modelcontextprotocol/server-filesystem')} open={false} />
              </Form.Item>
            </>
          )}
          {transport !== 'stdio' && (
            <Form.Item name="url" label={t('端点 URL')} rules={[{ required: true, message: t('请输入 URL') }, { type: 'url', message: t('URL 格式无效') }]}>
              <Input placeholder={transport === 'sse' ? 'https://.../sse' : 'https://...'} />
            </Form.Item>
          )}
          <Form.Item name="timeout" label={t('超时（秒，可选）')}>
            <InputNumber min={0} style={{ width: 160 }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
