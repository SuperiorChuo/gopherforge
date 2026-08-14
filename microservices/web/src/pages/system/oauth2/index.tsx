import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select, Switch,
  Card, Alert, Tabs, Typography, InputNumber, Tooltip, Row, Col,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined,
  KeyOutlined, StopOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as OAuth2API from '@/api/auth/oauth2'
import type { OAuth2Client, OAuth2AccessToken, OAuth2ClientSaveData } from '@/api/auth/oauth2'
import { CLIENT_TYPE } from '@/api/auth/oauth2'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import './styles.css'

const { Text, Paragraph } = Typography

const GRANT_LABELS: Record<string, string> = {
  authorization_code: '授权码',
  refresh_token: '刷新令牌',
  client_credentials: '客户端凭证',
}

function CompactTagList({
  values,
  color,
  labels,
}: {
  values?: string[]
  color?: string
  labels?: Record<string, string>
}) {
  const { t } = useTranslation()
  const items = values ?? []
  const visible = items.slice(0, 2)
  const remaining = items.slice(2)
  const display = (value: string) => labels?.[value] ?? value
  const remainingLabel = remaining.map(display).join('、')

  if (items.length === 0) return <span className="cell-muted">—</span>

  return (
    <Space size={4} className="oauth2-tag-list">
      {visible.map((value, index) => (
        <Tooltip key={`${value}-${index}`} title={display(value)}>
          <Tag variant="filled" color={color} className="oauth2-list-tag">
            {t(display(value))}
          </Tag>
        </Tooltip>
      ))}
      {remaining.length > 0 && (
        <Tooltip title={remainingLabel}>
          <Tag
            variant="filled"
            className="oauth2-list-tag oauth2-tag-more"
            tabIndex={0}
            aria-label={t('另有 {{n}} 项：{{list}}', { n: remaining.length, list: remainingLabel })}
          >
            +{remaining.length}
          </Tag>
        </Tooltip>
      )}
    </Space>
  )
}

function CopyableCode({ value }: { value: string }) {
  return (
    <Text type="secondary" copyable={{ text: value }} className="oauth2-copyable-code">
      <span className="oauth2-copyable-code-value">{value}</span>
    </Text>
  )
}

// 一次性密钥展示弹窗：创建/重置后仅此一次可见
function SecretModal({ secret, onClose }: { secret: string | null; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Modal open={!!secret} onCancel={onClose} onOk={onClose} title={t('客户端密钥（仅显示一次）')} maskClosable={false}>
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 12 }}
        message={t('请立即复制并妥善保存。关闭后将无法再次查看，只能重置。')}
      />
      <Paragraph
        copyable={{ text: secret ?? '', onCopy: () => message.success(t('已复制')) }}
        code
        className="oauth2-secret-value"
      >
        {secret}
      </Paragraph>
    </Modal>
  )
}

function ClientsTab() {
  const { t } = useTranslation()
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
      message.error(t('获取应用列表失败'))
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
      access_token_format: 'opaque',
      token_rate_per_minute: 0,
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
      access_token_format: record.access_token_format || 'opaque',
      token_rate_per_minute: record.token_rate_per_minute ?? 0,
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
      access_token_format: values.access_token_format || 'opaque',
      token_rate_per_minute: values.token_rate_per_minute ?? 0,
    }
    setSubmitting(true)
    try {
      if (editRecord) {
        await OAuth2API.updateOAuth2Client(editRecord.id, data)
        message.success(t('更新成功'))
      } else {
        const res = await OAuth2API.createOAuth2Client(data)
        message.success(t('创建成功'))
        if (res.client_secret) setSecret(res.client_secret)
      }
      setModalOpen(false)
      fetchList()
    } catch (err) {
      message.error(err instanceof Error ? err.message : t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await OAuth2API.deleteOAuth2Client(id)
      message.success(t('删除成功'))
      fetchList()
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleResetSecret = async (record: OAuth2Client) => {
    try {
      const res = await OAuth2API.resetOAuth2ClientSecret(record.id)
      setSecret(res.client_secret)
      message.success(t('已重置，原有令牌全部失效'))
    } catch (err) {
      message.error(err instanceof Error ? err.message : t('重置失败'))
    }
  }

  const columns: ColumnsType<OAuth2Client> = [
    {
      title: t('应用名称'),
      dataIndex: 'name',
      width: 260,
      ellipsis: true,
      render: (v, r) => (
        <div className="oauth2-client-cell">
          <Text strong className="oauth2-client-name">{v}</Text>
          <CopyableCode value={r.client_id} />
        </div>
      ),
    },
    { title: t('类型'), dataIndex: 'client_type', width: 90, responsive: ['sm'], render: (v) => (
      v === CLIENT_TYPE.PUBLIC ? <Tag variant="filled" color="orange">{t('公开')}</Tag> : <Tag variant="filled" color="blue">{t('机密')}</Tag>
    ) },
    { title: t('授权模式'), dataIndex: 'grant_types', width: 220, responsive: ['md'], render: (v: string[]) => (
      <CompactTagList values={v} labels={GRANT_LABELS} />
    ) },
    { title: 'Scopes', dataIndex: 'scopes', width: 220, responsive: ['lg'], render: (v: string[]) => (
      <CompactTagList values={v} color="geekblue" />
    ) },
    { title: t('状态'), dataIndex: 'status', width: 80, responsive: ['sm'], render: (v) => (
      v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />
    ) },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', responsive: ['lg'], render: (v) => formatDateTime(v) },
    {
      title: t('操作'), key: 'action', width: 132, fixed: 'right', render: (_, record) => (
        <Space size={4} className="table-actions oauth2-row-actions">
          {hasPerm('system:oauth2-client:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={t('编辑 OAuth2 应用')} icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:oauth2-client:reset-secret') && record.client_type === CLIENT_TYPE.CONFIDENTIAL && (
            <Popconfirm title={t('重置密钥？现有令牌将全部失效')} onConfirm={() => handleResetSecret(record)}>
              <Tooltip title={t('重置密钥')}>
                <Button type="text" size="small" aria-label={t('重置 OAuth2 客户端密钥')} icon={<KeyOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
          {hasPerm('system:oauth2-client:delete') && (
            <Popconfirm title={t('删除该应用？令牌与授权将一并清除')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除 OAuth2 应用')} icon={<DeleteOutlined />} />
              </Tooltip>
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
          <Space wrap className="oauth2-toolbar-actions">
            <Input.Search
              allowClear
              placeholder={t('搜索名称 / client_id')}
              className="oauth2-toolbar-search"
              onSearch={(v) => setParams((p) => ({ ...p, page: 1, keyword: v }))}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchList}>{t('刷新')}</Button>
            {hasPerm('system:oauth2-client:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建应用')}</Button>
            )}
          </Space>
        )}
      />
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={list}
        className="oauth2-table"
        rowClassName={(record) => (record.status === 0 ? 'oauth2-row-disabled' : '')}
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: <GlassEmpty text="还没有 OAuth2 应用" compact /> }}
        pagination={{
          current: params.page, pageSize: params.page_size, total,
          showSizeChanger: true, showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams((p) => ({ ...p, page, page_size })),
        }}
      />

      <Modal
        open={modalOpen}
        title={editRecord ? t('编辑应用') : t('新建应用')}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        confirmLoading={submitting}
        width={640}
        maskClosable={false}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label={t('应用名称')} rules={[{ required: true, message: t('请输入应用名称') }]}>
            <Input placeholder={t('如：客户自助门户')} />
          </Form.Item>
          <Form.Item name="client_type" label={t('客户端类型')} rules={[{ required: true }]}>
            <Select
              disabled={!!editRecord}
              options={[
                { value: CLIENT_TYPE.CONFIDENTIAL, label: t('机密客户端（服务端，有密钥）') },
                { value: CLIENT_TYPE.PUBLIC, label: t('公开客户端（SPA/移动端，强制 PKCE，无密钥）') },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="redirect_uris"
            label={t('回调地址（每行一个，需精确匹配）')}
            rules={[{ required: true, message: t('至少填写一个回调地址') }]}
          >
            <Input.TextArea rows={3} placeholder={'https://app.example.com/callback\nhttps://staging.example.com/callback'} />
          </Form.Item>
          <Form.Item name="scopes" label={t('授权范围（scope）')} rules={[{ required: true, message: t('请选择 scope') }]}>
            <Select mode="multiple" options={catalog.scopes.map((s) => ({ value: s, label: s }))} />
          </Form.Item>
          <Form.Item name="grant_types" label={t('授权模式')} rules={[{ required: true, message: t('请选择授权模式') }]}>
            <Select
              mode="multiple"
              options={catalog.grant_types
                .filter((g) => !(clientType === CLIENT_TYPE.PUBLIC && g === 'client_credentials'))
                .map((g) => ({ value: g, label: t(GRANT_LABELS[g] ?? g) }))}
            />
          </Form.Item>
          <Row gutter={16}>
            <Col xs={24} sm={9}>
              <Form.Item name="access_token_ttl" label={t('访问令牌有效期（秒）')}>
                <InputNumber min={60} className="oauth2-full-control" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={9}>
              <Form.Item name="refresh_token_ttl" label={t('刷新令牌有效期（秒）')}>
                <InputNumber min={60} className="oauth2-full-control" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={6}>
              <Form.Item name="auto_approve" label={t('自动授权')} valuePropName="checked" tooltip={t('跳过用户确认页（仅建议一方可信应用）')}>
                <Switch />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={14}>
              <Form.Item
                name="access_token_format"
                label={t('令牌形态')}
                tooltip={t('不透明：每次校验都回授权服务器，吊销立即生效（默认）。JWT：RFC 9068 自包含令牌，资源服务器可用 JWKS 离线验签，但离线验签方在过期前看不到吊销——选 JWT 时请把有效期配短。')}
              >
                <Select
                  className="oauth2-full-control"
                  options={[
                    { value: 'opaque', label: t('不透明串（默认，吊销即时生效）') },
                    { value: 'jwt', label: t('JWT（可离线验签）') },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={10}>
              <Form.Item
                name="token_rate_per_minute"
                label={t('令牌端点配额（次/分钟）')}
                tooltip={t('token 与 introspect 端点按本应用计的每分钟上限；0 表示使用服务端默认值。吊销端点不受限。')}
              >
                <InputNumber min={0} className="oauth2-full-control" placeholder={t('0=默认')} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={2} placeholder={t('展示在授权确认页')} />
          </Form.Item>
          {editRecord && (
            <Form.Item name="status" label={t('状态')} valuePropName="value">
              <Select options={[{ value: 1, label: t('启用') }, { value: 0, label: t('停用（立即吊销所有令牌）') }]} />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <SecretModal secret={secret} onClose={() => setSecret(null)} />
    </>
  )
}

function TokensTab() {
  const { t } = useTranslation()
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
      message.error(t('获取令牌列表失败'))
    } finally {
      setLoading(false)
    }
  }, [params])

  useEffect(() => { fetchList() }, [fetchList])

  const handleRevoke = async (id: number) => {
    try {
      await OAuth2API.revokeOAuth2Token(id)
      message.success(t('已吊销'))
      fetchList()
    } catch {
      message.error(t('吊销失败'))
    }
  }

  const columns: ColumnsType<OAuth2AccessToken> = [
    { title: 'client_id', dataIndex: 'client_id', width: 260, ellipsis: true, render: (v) => <CopyableCode value={v} /> },
    { title: t('用户'), dataIndex: 'username', width: 160, responsive: ['sm'], render: (v) => v || <Text type="secondary">{t('（应用自身）')}</Text> },
    { title: t('授权模式'), dataIndex: 'grant_type', width: 160, responsive: ['md'], render: (v) => t(GRANT_LABELS[v] ?? v) },
    { title: 'Scopes', dataIndex: 'scopes', width: 220, responsive: ['lg'], render: (v: string[]) => (
      <CompactTagList values={v} color="geekblue" />
    ) },
    { title: t('状态'), key: 'state', width: 90, render: (_, r) => {
      if (r.revoked_at) return <StatusPill tone="muted" label="已吊销" />
      if (new Date(r.expires_at).getTime() < Date.now()) return <StatusPill tone="warning" label="已过期" pulse={false} />
      return <StatusPill tone="success" label="有效" />
    } },
    { title: t('过期时间'), dataIndex: 'expires_at', width: 170, className: 'cell-time', responsive: ['md'], render: (v) => formatDateTime(v) },
    {
      title: t('操作'), key: 'action', width: 64, fixed: 'right', render: (_, record) => (
        hasPerm('system:oauth2-token:delete') && !record.revoked_at ? (
          <Popconfirm title={t('吊销该令牌？')} onConfirm={() => handleRevoke(record.id)}>
            <Tooltip title={t('吊销')}>
              <Button type="text" size="small" danger aria-label={t('吊销 OAuth2 令牌')} icon={<StopOutlined />} className="oauth2-token-action" />
            </Tooltip>
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
          <Space wrap className="oauth2-toolbar-actions">
            <Input.Search
              allowClear
              placeholder={t('按 client_id 过滤')}
              className="oauth2-toolbar-search"
              onSearch={(v) => setParams((p) => ({ ...p, page: 1, client_id: v }))}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchList}>{t('刷新')}</Button>
          </Space>
        )}
      />
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={list}
        className="oauth2-table"
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: <GlassEmpty text="暂无签发的令牌" compact /> }}
        pagination={{
          current: params.page, pageSize: params.page_size, total,
          showSizeChanger: true, showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams((p) => ({ ...p, page, page_size })),
        }}
      />
    </>
  )
}

export default function OAuth2Page() {
  const { t } = useTranslation()
  return (
    <Card variant="borderless" className="oauth2-page" styles={{ body: { padding: 0 } }}>
      <Tabs
        defaultActiveKey="clients"
        style={{ padding: '0 16px' }}
        items={[
          { key: 'clients', label: t('应用管理'), children: <div className="oauth2-tab-panel"><ClientsTab /></div> },
          { key: 'tokens', label: t('令牌管理'), children: <div className="oauth2-tab-panel"><TokensTab /></div> },
        ]}
      />
    </Card>
  )
}
