import { useCallback, useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select, Switch,
  Card, Alert, Tabs, Typography, InputNumber,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined,
  KeyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as OAuth2API from '@/api/oauth2'
import type { OAuth2Client, OAuth2AccessToken, OAuth2ClientSaveData } from '@/api/oauth2'
import { CLIENT_TYPE } from '@/api/oauth2'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

const { Text, Paragraph } = Typography

const GRANT_LABELS: Record<string, string> = {
  authorization_code: '授权码',
  refresh_token: '刷新令牌',
  client_credentials: '客户端凭证',
}

// 一次性密钥展示弹窗：创建/重置后仅此一次可见
function SecretModal({ secret, onClose }: { secret: string | null; onClose: () => void }) {
  return (
    <Modal open={!!secret} onCancel={onClose} onOk={onClose} title="客户端密钥（仅显示一次）" maskClosable={false}>
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 12 }}
        message="请立即复制并妥善保存。关闭后将无法再次查看，只能重置。"
      />
      <Paragraph copyable={{ text: secret ?? '', onCopy: () => message.success('已复制') }} code>
        {secret}
      </Paragraph>
    </Modal>
  )
}

function ClientsTab() {
  const { hasPerm } = usePermission()
  const [list, setList] = useState<OAuth2Client[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState({ page: 1, page_size: 10, keyword: '' })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<OAuth2Client | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [catalog, setCatalog] = useState<{ scopes: string[]; grant_types: string[] }>({ scopes: [], grant_types: [] })
  const [form] = Form.useForm()
  const clientType = Form.useWatch('client_type', form)

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const res = await OAuth2API.getOAuth2ClientList(params)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取应用列表失败')
    } finally {
      setLoading(false)
    }
  }, [params])

  useEffect(() => { fetchList() }, [fetchList])
  useEffect(() => {
    OAuth2API.getOAuth2Catalog().then(setCatalog).catch(() => undefined)
  }, [])

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    form.setFieldsValue({
      client_type: CLIENT_TYPE.CONFIDENTIAL,
      grant_types: ['authorization_code', 'refresh_token'],
      scopes: ['profile'],
      access_token_ttl: 3600,
      refresh_token_ttl: 2592000,
      auto_approve: false,
      status: 1,
    })
    setModalOpen(true)
  }

  const openEdit = (record: OAuth2Client) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      logo: record.logo,
      description: record.description,
      client_type: record.client_type,
      redirect_uris: record.redirect_uris.join('\n'),
      scopes: record.scopes,
      grant_types: record.grant_types,
      access_token_ttl: record.access_token_ttl,
      refresh_token_ttl: record.refresh_token_ttl,
      auto_approve: record.auto_approve,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    const uris = String(values.redirect_uris || '')
      .split('\n')
      .map((s: string) => s.trim())
      .filter(Boolean)
    const data: OAuth2ClientSaveData = {
      name: values.name,
      logo: values.logo || '',
      description: values.description || '',
      client_type: values.client_type,
      redirect_uris: uris,
      scopes: values.scopes || [],
      grant_types: values.grant_types || [],
      access_token_ttl: values.access_token_ttl,
      refresh_token_ttl: values.refresh_token_ttl,
      auto_approve: values.auto_approve,
      status: values.status,
    }
    setSubmitting(true)
    try {
      if (editRecord) {
        await OAuth2API.updateOAuth2Client(editRecord.id, data)
        message.success('更新成功')
      } else {
        const res = await OAuth2API.createOAuth2Client(data)
        message.success('创建成功')
        if (res.client_secret) setSecret(res.client_secret)
      }
      setModalOpen(false)
      fetchList()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await OAuth2API.deleteOAuth2Client(id)
      message.success('删除成功')
      fetchList()
    } catch {
      message.error('删除失败')
    }
  }

  const handleResetSecret = async (record: OAuth2Client) => {
    try {
      const res = await OAuth2API.resetOAuth2ClientSecret(record.id)
      setSecret(res.client_secret)
      message.success('已重置，原有令牌全部失效')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重置失败')
    }
  }

  const columns: ColumnsType<OAuth2Client> = [
    { title: '应用名称', dataIndex: 'name', render: (v, r) => (
      <Space direction="vertical" size={0}>
        <Text strong>{v}</Text>
        <Text type="secondary" copyable={{ text: r.client_id }} style={{ fontSize: 12 }}>{r.client_id}</Text>
      </Space>
    ) },
    { title: '类型', dataIndex: 'client_type', width: 90, render: (v) => (
      v === CLIENT_TYPE.PUBLIC ? <Tag color="orange">公开</Tag> : <Tag color="blue">机密</Tag>
    ) },
    { title: '授权模式', dataIndex: 'grant_types', render: (v: string[]) => (
      <Space wrap size={4}>{v.map((g) => <Tag key={g}>{GRANT_LABELS[g] || g}</Tag>)}</Space>
    ) },
    { title: 'Scopes', dataIndex: 'scopes', render: (v: string[]) => (
      <Space wrap size={4}>{v.map((s) => <Tag key={s} color="geekblue">{s}</Tag>)}</Space>
    ) },
    { title: '状态', dataIndex: 'status', width: 80, render: (v) => (
      v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>
    ) },
    { title: '创建时间', dataIndex: 'created_at', width: 170, render: (v) => formatDateTime(v) },
    {
      title: '操作', key: 'action', width: 220, fixed: 'right', render: (_, record) => (
        <Space size={4}>
          {hasPerm('system:oauth2-client:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:oauth2-client:reset-secret') && record.client_type === CLIENT_TYPE.CONFIDENTIAL && (
            <Popconfirm title="重置密钥？现有令牌将全部失效" onConfirm={() => handleResetSecret(record)}>
              <Button type="link" size="small" icon={<KeyOutlined />}>重置密钥</Button>
            </Popconfirm>
          )}
          {hasPerm('system:oauth2-client:delete') && (
            <Popconfirm title="删除该应用？令牌与授权将一并清除" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <TableToolbar
        title="OAuth2 应用"
        total={total}
        extra={(
          <Space wrap>
            <Input.Search
              allowClear
              placeholder="搜索名称 / client_id"
              style={{ width: 220 }}
              onSearch={(v) => setParams((p) => ({ ...p, page: 1, keyword: v }))}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchList}>刷新</Button>
            {hasPerm('system:oauth2-client:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建应用</Button>
            )}
          </Space>
        )}
      />
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={list}
        scroll={{ x: 900 }}
        locale={{ emptyText: <GlassEmpty text="还没有 OAuth2 应用" compact /> }}
        pagination={{
          current: params.page, pageSize: params.page_size, total,
          showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          onChange: (page, page_size) => setParams((p) => ({ ...p, page, page_size })),
        }}
      />

      <Modal
        open={modalOpen}
        title={editRecord ? '编辑应用' : '新建应用'}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        confirmLoading={submitting}
        width={640}
        maskClosable={false}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="应用名称" rules={[{ required: true, message: '请输入应用名称' }]}>
            <Input placeholder="如：客户自助门户" />
          </Form.Item>
          <Form.Item name="client_type" label="客户端类型" rules={[{ required: true }]}>
            <Select
              disabled={!!editRecord}
              options={[
                { value: CLIENT_TYPE.CONFIDENTIAL, label: '机密客户端（服务端，有密钥）' },
                { value: CLIENT_TYPE.PUBLIC, label: '公开客户端（SPA/移动端，强制 PKCE，无密钥）' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="redirect_uris"
            label="回调地址（每行一个，需精确匹配）"
            rules={[{ required: true, message: '至少填写一个回调地址' }]}
          >
            <Input.TextArea rows={3} placeholder={'https://app.example.com/callback\nhttps://staging.example.com/callback'} />
          </Form.Item>
          <Form.Item name="scopes" label="授权范围（scope）" rules={[{ required: true, message: '请选择 scope' }]}>
            <Select mode="multiple" options={catalog.scopes.map((s) => ({ value: s, label: s }))} />
          </Form.Item>
          <Form.Item name="grant_types" label="授权模式" rules={[{ required: true, message: '请选择授权模式' }]}>
            <Select
              mode="multiple"
              options={catalog.grant_types
                .filter((g) => !(clientType === CLIENT_TYPE.PUBLIC && g === 'client_credentials'))
                .map((g) => ({ value: g, label: GRANT_LABELS[g] || g }))}
            />
          </Form.Item>
          <Space size="large" style={{ display: 'flex' }}>
            <Form.Item name="access_token_ttl" label="访问令牌有效期（秒）">
              <InputNumber min={60} style={{ width: 160 }} />
            </Form.Item>
            <Form.Item name="refresh_token_ttl" label="刷新令牌有效期（秒）">
              <InputNumber min={60} style={{ width: 160 }} />
            </Form.Item>
            <Form.Item name="auto_approve" label="自动授权" valuePropName="checked" tooltip="跳过用户确认页（仅建议一方可信应用）">
              <Switch />
            </Form.Item>
          </Space>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="展示在授权确认页" />
          </Form.Item>
          {editRecord && (
            <Form.Item name="status" label="状态" valuePropName="value">
              <Select options={[{ value: 1, label: '启用' }, { value: 0, label: '停用（立即吊销所有令牌）' }]} />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <SecretModal secret={secret} onClose={() => setSecret(null)} />
    </>
  )
}

function TokensTab() {
  const { hasPerm } = usePermission()
  const [list, setList] = useState<OAuth2AccessToken[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState({ page: 1, page_size: 10, client_id: '' })

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const res = await OAuth2API.getOAuth2TokenList(params)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取令牌列表失败')
    } finally {
      setLoading(false)
    }
  }, [params])

  useEffect(() => { fetchList() }, [fetchList])

  const handleRevoke = async (id: number) => {
    try {
      await OAuth2API.revokeOAuth2Token(id)
      message.success('已吊销')
      fetchList()
    } catch {
      message.error('吊销失败')
    }
  }

  const columns: ColumnsType<OAuth2AccessToken> = [
    { title: 'client_id', dataIndex: 'client_id', render: (v) => <Text copyable style={{ fontSize: 12 }}>{v}</Text> },
    { title: '用户', dataIndex: 'username', render: (v) => v || <Text type="secondary">（应用自身）</Text> },
    { title: '授权模式', dataIndex: 'grant_type', width: 140, render: (v) => GRANT_LABELS[v] || v },
    { title: 'Scopes', dataIndex: 'scopes', render: (v: string[]) => (
      <Space wrap size={4}>{(v || []).map((s) => <Tag key={s} color="geekblue">{s}</Tag>)}</Space>
    ) },
    { title: '状态', key: 'state', width: 90, render: (_, r) => {
      if (r.revoked_at) return <Tag>已吊销</Tag>
      if (new Date(r.expires_at).getTime() < Date.now()) return <Tag color="orange">已过期</Tag>
      return <Tag color="green">有效</Tag>
    } },
    { title: '过期时间', dataIndex: 'expires_at', width: 170, render: (v) => formatDateTime(v) },
    {
      title: '操作', key: 'action', width: 100, fixed: 'right', render: (_, record) => (
        hasPerm('system:oauth2-token:delete') && !record.revoked_at ? (
          <Popconfirm title="吊销该令牌？" onConfirm={() => handleRevoke(record.id)}>
            <Button type="link" size="small" danger>吊销</Button>
          </Popconfirm>
        ) : null
      ),
    },
  ]

  return (
    <>
      <TableToolbar
        title="已签发令牌"
        total={total}
        extra={(
          <Space wrap>
            <Input.Search
              allowClear
              placeholder="按 client_id 过滤"
              style={{ width: 240 }}
              onSearch={(v) => setParams((p) => ({ ...p, page: 1, client_id: v }))}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchList}>刷新</Button>
          </Space>
        )}
      />
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={list}
        scroll={{ x: 800 }}
        locale={{ emptyText: <GlassEmpty text="暂无签发的令牌" compact /> }}
        pagination={{
          current: params.page, pageSize: params.page_size, total,
          showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          onChange: (page, page_size) => setParams((p) => ({ ...p, page, page_size })),
        }}
      />
    </>
  )
}

export default function OAuth2Page() {
  return (
    <Card variant="borderless" styles={{ body: { padding: 0 } }}>
      <Tabs
        defaultActiveKey="clients"
        style={{ padding: '0 16px' }}
        items={[
          { key: 'clients', label: '应用管理', children: <div style={{ paddingBottom: 16 }}><ClientsTab /></div> },
          { key: 'tokens', label: '令牌管理', children: <div style={{ paddingBottom: 16 }}><TokensTab /></div> },
        ]}
      />
    </Card>
  )
}
