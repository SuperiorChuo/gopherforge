// 制品审批：录入不可变制品（版本 + 上游 commit + 来源 + SHA-256），并做批准/拒绝/吊销。
// 校验以服务端为准，这里的表单规则只是提前反馈，避免来回一趟。
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tooltip } from 'antd'
import {
  CheckOutlined,
  CheckSquareOutlined,
  CloseOutlined,
  PlusOutlined,
  ReloadOutlined,
  StopOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  approveHermesArtifact,
  createHermesArtifact,
  listHermesArtifacts,
  rejectHermesArtifact,
  revokeHermesArtifact,
  type HermesArtifact,
} from '@/api/ai/hermes'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill, { type StatusTone } from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { useTableQuery } from '@/hooks/useTableQuery'
import { usePagination } from '@/hooks/usePagination'
import { fetchAllHermesPages } from '../admin-contract'

const STATUS_META: Record<string, { label: string; tone: StatusTone }> = {
  pending: { label: '待审批', tone: 'warning' },
  approved: { label: '已批准', tone: 'success' },
  rejected: { label: '已拒绝', tone: 'muted' },
  revoked: { label: '已吊销', tone: 'danger' },
}

const SHA256_RE = /^[0-9a-f]{64}$/
const COMMIT_RE = /^[0-9a-f]{7,64}$/
const MOVING_REFS = ['latest', 'main', 'master', 'head', 'dev']

export default function HermesArtifactsPage() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [open, setOpen] = useState(false)
  const [form] = Form.useForm()
  const kind = Form.useWatch('kind', form)

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesArtifacts(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: artifacts, total, loading, reload } = useTableQuery({ params, fetcher })

  // 待审批聚合：全量统计（逐页真实拉取，不放大 page_size 伪装全量）
  const [pendingCount, setPendingCount] = useState(0)
  const loadStats = useCallback(async () => {
    try {
      const all = await fetchAllHermesPages<HermesArtifact>((page, pageSize) =>
        listHermesArtifacts({ page, page_size: pageSize }))
      setPendingCount(all.filter((item) => item.status === 'pending').length)
    } catch {
      // 统计失败保留上次值；请求层已提示服务端消息
    }
  }, [])
  const pending = pendingCount

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  const act = async (fn: () => Promise<unknown>, okMessage: string) => {
    try {
      await fn()
      message.success(okMessage)
      void reload()
      void loadStats()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const openModal = () => {
    form.setFieldsValue({ kind: 'hermes', platform: 'linux', arch: 'amd64' })
    setOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSaving(true)
    try {
      await createHermesArtifact(values)
      message.success(t('制品已录入，等待审批'))
      setOpen(false)
      await reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const columns: ColumnsType<HermesArtifact> = [
    { title: t('类型'), dataIndex: 'kind', width: 90, render: (value: string) => (value === 'manager' ? 'Manager' : 'Hermes') },
    { title: t('平台'), width: 130, render: (_, record) => `${record.platform} / ${record.arch}` },
    { title: t('版本'), dataIndex: 'version', width: 130 },
    {
      title: t('上游 commit'),
      dataIndex: 'upstream_commit',
      width: 110,
      render: (value: string) => (value ? <span className="cell-mono">{value.slice(0, 10)}</span> : '—'),
    },
    {
      title: t('SHA-256'),
      dataIndex: 'sha256',
      ellipsis: true,
      render: (value: string) => <span className="cell-mono">{value}</span>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: string) => (
        <StatusPill
          tone={STATUS_META[value]?.tone ?? 'muted'}
          label={STATUS_META[value]?.label ?? value}
          pulse={false}
        />
      ),
    },
    {
      title: t('操作'),
      width: 110,
      fixed: 'right',
      render: (_, record) => (
        <Space size={2}>
          {record.status === 'pending' && (
            <>
              <Popconfirm
                title={t('批准此不可变制品？')}
                description={t('批准后策略即可引用该版本下发到员工电脑。')}
                onConfirm={() => void act(() => approveHermesArtifact(record.id), t('制品已批准'))}
              >
                <Tooltip title={t('批准')}>
                  <Button type="text" icon={<CheckOutlined />} />
                </Tooltip>
              </Popconfirm>
              <Popconfirm
                title={t('拒绝此制品？')}
                onConfirm={() => void act(() => rejectHermesArtifact(record.id), t('制品已拒绝'))}
              >
                <Tooltip title={t('拒绝')}>
                  <Button type="text" icon={<CloseOutlined />} />
                </Tooltip>
              </Popconfirm>
            </>
          )}
          {record.status === 'approved' && (
            <Popconfirm
              title={t('吊销此制品？')}
              description={t('仅阻止后续引用与下载；已下发策略仍持有该版本，需另发新策略替换。')}
              onConfirm={() => void act(() => revokeHermesArtifact(record.id), t('制品已吊销'))}
            >
              <Tooltip title={t('吊销')}>
                <Button type="text" danger icon={<StopOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-artifacts-page"
      toolbar={
        <TableToolbar
          title="桥接组件"
        total={total}
        icon={<CheckSquareOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={pending > 0 ? t('{{n}} 个组件待审批', { n: pending }) : t('桥接组件均已完成审批')}
        extra={
          <Space>
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void loadStats() }} loading={loading} />
            </Tooltip>
            <Button type="primary" icon={<PlusOutlined />} onClick={openModal}>
              {t('录入组件')}
            </Button>
          </Space>
        }
      />}
    >
        <Table
          rowKey="id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={artifacts}
          scroll={{ x: 900 }}
          locale={{ emptyText: <GlassEmpty text="暂无 Manager 或 Hermes 桥接组件" compact /> }}
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
        title={t('录入桥接组件')}
        open={open}
        onOk={() => void submit()}
        onCancel={() => setOpen(false)}
        confirmLoading={saving}
        width={620}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="kind" label={t('类型')} rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'hermes', label: t('Hermes 本体') },
                { value: 'manager', label: 'gak-hermes-manager' },
              ]}
            />
          </Form.Item>
          <Space style={{ display: 'flex' }} size={12}>
            <Form.Item name="platform" label={t('平台')} rules={[{ required: true }]} style={{ flex: 1 }}>
              <Select
                options={[
                  { value: 'windows', label: 'windows' },
                  { value: 'darwin', label: 'darwin' },
                  { value: 'linux', label: 'linux' },
                  { value: 'any', label: 'any' },
                ]}
              />
            </Form.Item>
            <Form.Item name="arch" label={t('架构')} rules={[{ required: true }]} style={{ flex: 1 }}>
              <Select
                options={[
                  { value: 'amd64', label: 'amd64' },
                  { value: 'arm64', label: 'arm64' },
                  { value: 'any', label: 'any' },
                ]}
              />
            </Form.Item>
          </Space>
          <Form.Item
            name="version"
            label={t('版本（上游 tag，不可用移动引用）')}
            rules={[
              { required: true, max: 40 },
              {
                validator: (_, value?: string) =>
                  value && MOVING_REFS.includes(value.trim().toLowerCase())
                    ? Promise.reject(new Error(t('不能用 latest / main 这类移动引用，必须是不可变 tag')))
                    : Promise.resolve(),
              },
            ]}
          >
            <Input placeholder="v2026.7.20" />
          </Form.Item>
          <Form.Item
            name="upstream_commit"
            label={kind === 'hermes' ? t('上游 commit（必填）') : t('上游 commit（可选）')}
            rules={[
              { required: kind === 'hermes', message: t('Hermes 本体必须钉住上游 commit') },
              {
                validator: (_, value?: string) =>
                  !value || COMMIT_RE.test(value.trim().toLowerCase())
                    ? Promise.resolve()
                    : Promise.reject(new Error(t('应为 7-64 位十六进制 sha'))),
              },
            ]}
          >
            <Input placeholder="1cd8f8c37e152cc6938a567c6d368e95ad1588ef" />
          </Form.Item>
          <Form.Item
            name="source_url"
            label={t('下载来源（必须 https）')}
            rules={[
              { required: true, max: 1024 },
              {
                validator: (_, value?: string) =>
                  !value || value.trim().startsWith('https://')
                    ? Promise.resolve()
                    : Promise.reject(new Error(t('制品会下载到员工电脑执行，来源必须是 https'))),
              },
            ]}
          >
            <Input placeholder="https://..." />
          </Form.Item>
          <Form.Item
            name="sha256"
            label="SHA-256"
            rules={[
              { required: true },
              {
                validator: (_, value?: string) =>
                  !value || SHA256_RE.test(value.trim().toLowerCase())
                    ? Promise.resolve()
                    : Promise.reject(new Error(t('应为 64 位小写十六进制'))),
              },
            ]}
          >
            <Input placeholder={t('64 位小写十六进制')} />
          </Form.Item>
          <Form.Item name="signature" label={t('签名（可选）')}>
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
