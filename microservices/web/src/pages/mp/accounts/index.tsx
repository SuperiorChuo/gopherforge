import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, Modal, Select, Space, Switch, Table, Tag, Typography, Grid } from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, WechatOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
// icons
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  createMpAccount, deleteMpAccount, listMpAccounts, updateMpAccount,
  MP_ENCRYPT_META, MP_TYPE_META, type MpAccount, type MpAccountPayload,
} from '@/api/mp'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import './index.css'

const { Text } = Typography

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

export default function MpAccountsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('social:write')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [editing, setEditing] = useState<MpAccount | null>(null)
  const [creating, setCreating] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const editorSequence = useRef(0)

  const fetchList = useCallback((p: SearchParams) => listMpAccounts({ ...p }), [])
  const onLoadError = useCallback(() => message.error(t('获取公众号账号失败')), [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  function openEditor(row: MpAccount | null) {
    if (saving || !canWrite) return
    editorSequence.current += 1
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
      app_id: row?.app_id || '',
      name: row?.name || '',
      type: row?.type || 'service',
      gh_id: row?.gh_id || '',
      token: '',
      secret_ref: row?.secret_ref || '',
      aes_key_ref: row?.aes_key_ref || '',
      encrypt_mode: row?.encrypt_mode || 'plain',
      auto_publish: row?.auto_publish ?? false,
    })
  }

  function closeEditor() {
    if (saving) return
    editorSequence.current += 1
    setEditing(null)
    setCreating(false)
  }

  async function onSave() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    const body: MpAccountPayload = {
      app_id: values.app_id,
      name: values.name,
      type: values.type,
      gh_id: values.gh_id || '',
      token: values.token || undefined,
      secret_ref: values.secret_ref || '',
      aes_key_ref: values.aes_key_ref || '',
      encrypt_mode: values.encrypt_mode,
      auto_publish: !!values.auto_publish,
    }
    const sequence = editorSequence.current
    setSaving(true)
    try {
      if (editing) {
        await updateMpAccount(editing.id, body)
        message.success(t('已保存'))
      } else {
        await createMpAccount(body)
        message.success(t('账号已创建'))
      }
      if (sequence !== editorSequence.current) return
      editorSequence.current += 1
      setEditing(null)
      setCreating(false)
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      setSaving(false)
    }
  }

  function onDelete(row: MpAccount) {
    Modal.confirm({
      title: t('删除账号「{{name}}」', { name: row.name }),
      content: t('删除后该公众号的配置将被移除，回调将不再生效。此操作不可恢复。'),
      okText: t('删除'),
      okButtonProps: { danger: true },
      cancelText: t('取消'),
      async onOk() {
        try {
          await deleteMpAccount(row.id)
          message.success(t('已删除'))
          void reload()
        } catch (e: unknown) {
          message.error(e instanceof Error ? e.message : t('删除失败'))
        }
      },
    })
  }

  const columns: ColumnsType<MpAccount> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: 'AppID', dataIndex: 'app_id', width: 200, render: (v: string) => <span className="cell-mono">{v || '—'}</span> },
    { title: t('名称'), dataIndex: 'name', ellipsis: true },
    {
      title: t('类型'), dataIndex: 'type', width: 100,
      render: (v: string) => {
        const meta = MP_TYPE_META[v] ?? { label: v, color: 'default' }
        return <Tag color={meta.color} variant="filled">{t(meta.label)}</Tag>
      },
    },
    {
      title: t('加密模式'), dataIndex: 'encrypt_mode', width: 110,
      render: (v: string) => {
        const meta = MP_ENCRYPT_META[v] ?? { label: v, color: 'default' }
        return <Tag color={meta.color} variant="filled">{t(meta.label)}</Tag>
      },
    },
    {
      title: t('自动发布'), dataIndex: 'auto_publish', width: 100,
      render: (v: boolean) =>
        v ? <StatusPill tone="success" label="已开启" pulse={false} /> : <StatusPill tone="muted" label="关闭" />,
    },
    {
      title: t('状态'), dataIndex: 'status', width: 90,
      render: (v: string) =>
        v === 'active' ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 165, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'), width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => canWrite ? (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEditor(row) },
            { key: 'delete', label: t('删除'), icon: <DeleteOutlined />, danger: true, onClick: () => onDelete(row) },
          ]}
        />
      ) : <span className="cell-muted">—</span>,
    },
  ]

  return (
    <ListPageShell
      className="mp-accounts-page"
      filter={
        <ListFilterForm
          form={searchForm}
          initialValues={params}
          onFinish={({ keyword }) => setParams({ ...params, page: 1, keyword })}
        >
          <Form.Item name="keyword">
            <Input placeholder={t('名称 / AppID')} prefix={<SearchOutlined />} allowClear style={{ width: 240 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.setFieldsValue({ keyword: undefined }); setParams({ page: 1, page_size: 10 }) }}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      }
      toolbar={
        <TableToolbar
          title="公众号账号"
          total={total}
          icon={<WechatOutlined />}
          gradient="linear-gradient(135deg, #34d399, #059669)"
          glow="rgba(5, 150, 105, 0.4)"
          description="微信公众号接入配置、回调校验与自动发布开关"
          extra={
            <Space wrap>
              {canWrite && <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>{t('新建账号')}</Button>}
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无公众号账号" compact /> }}
          scroll={{ x: 1080 }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />

      <Modal
        title={editing ? t('编辑账号 #{{id}}', { id: editing.id }) : t('新建公众号账号')}
        open={creating || !!editing}
        onCancel={closeEditor}
        onOk={() => void onSave()}
        confirmLoading={saving}
        closable={!saving}
        mask={{ closable: !saving }}
        keyboard={!saving}
        cancelButtonProps={{ disabled: saving }}
        width={560}
        destroyOnHidden
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
          {t('secret_ref / aes_key_ref 填写的是环境变量名或配置项 key（不是明文密钥）；token 是公众号后台配置的回调校验 Token。')}
        </Text>
        <Form form={form} layout="vertical" disabled={saving}>
          <Form.Item name="app_id" label="AppID" rules={[{ required: true, message: t('请输入 AppID') }]}>
            <Input placeholder="wx0123456789abcdef" />
          </Form.Item>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
            <Input placeholder={t('公众号显示名称')} />
          </Form.Item>
          <div className="mp-account-form-grid">
            <Form.Item name="type" label={t('类型')} rules={[{ required: true }]}>
              <Select options={Object.entries(MP_TYPE_META).map(([value, m]) => ({ value, label: t(m.label) }))} />
            </Form.Item>
            <Form.Item name="gh_id" label={t('原始 ID（gh_id）')}>
              <Input placeholder="gh_xxxxxxxx" />
            </Form.Item>
          </div>
          <Form.Item name="token" label={t('回调校验 Token')} extra={t('用于服务器配置的 URL 校验；编辑时留空表示不修改')}>
            <Input.Password placeholder={editing ? t('留空则不修改') : t('与公众号后台一致')} autoComplete="new-password" />
          </Form.Item>
          <div className="mp-account-form-grid">
            <Form.Item name="secret_ref" label={t('Secret 引用（secret_ref）')}>
              <Input placeholder={t('如 MP_APP_SECRET')} />
            </Form.Item>
            <Form.Item name="aes_key_ref" label={t('AES Key 引用（aes_key_ref）')}>
              <Input placeholder={t('如 MP_AES_KEY')} />
            </Form.Item>
          </div>
          <Form.Item name="encrypt_mode" label={t('加密模式')} rules={[{ required: true }]}>
            <Select options={Object.entries(MP_ENCRYPT_META).map(([value, m]) => ({ value, label: t(m.label) }))} />
          </Form.Item>
          <Form.Item name="auto_publish" label={t('自动发布')} valuePropName="checked" extra={t('AI 生成文章审核通过后是否自动群发')}>
            <Switch checkedChildren={t('开')} unCheckedChildren={t('关')} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
