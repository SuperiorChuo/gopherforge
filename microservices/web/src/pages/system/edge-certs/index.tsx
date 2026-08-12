import { useCallback, useEffect, useState } from 'react'
import {
  Alert, Button, Form, Input, Modal, Space, Switch, Table, Tag, Typography,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, ReloadOutlined, SafetyCertificateOutlined,
  CloudDownloadOutlined, ThunderboltOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import * as API from '@/api/system/edgeCert'
import type { EdgeCert } from '@/api/system/edgeCert'

const statusColor: Record<string, string> = {
  draft: 'default',
  pending: 'processing',
  issued: 'success',
  failed: 'error',
  expired: 'warning',
}

const statusLabel: Record<string, string> = {
  draft: '草稿',
  pending: '申请中',
  issued: '已签发',
  failed: '失败',
  expired: '已过期',
}

export default function EdgeCertsPage() {
  const { hasPerm } = usePermission()
  const canIssue = hasPerm('system:edge-cert:issue')
  const canDelete = hasPerm('system:edge-cert:delete')

  const [list, setList] = useState<EdgeCert[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [issuingId, setIssuingId] = useState<number | null>(null)
  const [form] = Form.useForm()

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const data = await API.listEdgeCerts()
      setList(Array.isArray(data) ? data : [])
    } catch {
      message.error('加载边缘证书失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetchList() }, [fetchList])

  const onCreate = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      await API.createEdgeCert({
        domain: String(values.domain).trim().toLowerCase(),
        email: String(values.email).trim(),
        is_staging: !!values.is_staging,
      })
      message.success('已保存，可点击「申请证书」')
      setModalOpen(false)
      form.resetFields()
      await fetchList()
    } catch (e: unknown) {
      if (e && typeof e === 'object' && 'errorFields' in e) return
      message.error('保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const onIssue = async (row: EdgeCert) => {
    setIssuingId(row.id)
    try {
      await API.issueEdgeCert(row.id)
      message.success(`已签发 ${row.domain}`)
      await fetchList()
    } catch {
      message.error('申请失败：请确认域名 A 记录指向本机网关 80 端口，并查看行内错误信息')
      await fetchList()
    } finally {
      setIssuingId(null)
    }
  }

  const onDownload = async (row: EdgeCert) => {
    try {
      const data = await API.downloadEdgeCert(row.id)
      const blob = new Blob(
        [`# ${data.domain}\n# fullchain\n${data.fullchain_pem}\n# private key\n${data.private_key_pem}\n`],
        { type: 'text/plain;charset=utf-8' },
      )
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${data.domain}.pem.txt`
      a.click()
      URL.revokeObjectURL(url)
      message.success('已下载（含私钥，请妥善保管）')
    } catch {
      message.error('下载失败')
    }
  }

  const onDelete = async (row: EdgeCert) => {
    try {
      await API.deleteEdgeCert(row.id)
      message.success('已删除')
      await fetchList()
    } catch {
      message.error('删除失败')
    }
  }

  const columns: ColumnsType<EdgeCert> = [
    { title: '域名', dataIndex: 'domain', key: 'domain', ellipsis: true },
    { title: '邮箱', dataIndex: 'email', key: 'email', width: 200, ellipsis: true },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 100,
      render: (s: string) => <Tag color={statusColor[s] || 'default'}>{statusLabel[s] || s}</Tag>,
    },
    {
      title: '环境', dataIndex: 'is_staging', key: 'is_staging', width: 100,
      render: (v: boolean) => (v ? <Tag>Staging</Tag> : <Tag color="blue">Production</Tag>),
    },
    {
      title: '有效期至', dataIndex: 'not_after', key: 'not_after', width: 170,
      render: (v?: string) => (v ? formatDateTime(v) : '—'),
    },
    {
      title: '错误', dataIndex: 'last_error', key: 'last_error', ellipsis: true,
      render: (v?: string) => v || '—',
    },
    {
      title: '操作', key: 'actions', width: 220, fixed: 'right',
      render: (_, row) => (
        <Space size={4}>
          {canIssue && (
            <Button
              type="link"
              size="small"
              icon={<ThunderboltOutlined />}
              loading={issuingId === row.id}
              onClick={() => void onIssue(row)}
            >
              申请证书
            </Button>
          )}
          {canIssue && row.has_cert && (
            <Button type="link" size="small" icon={<CloudDownloadOutlined />} onClick={() => void onDownload(row)}>
              下载
            </Button>
          )}
          {canDelete && (
            <Button type="link" size="small" danger icon={<DeleteOutlined />} onClick={() => void onDelete(row)}>
              删除
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <TableToolbar
        title="边缘证书"
        description="Let's Encrypt 免费证书（HTTP-01）· 用于网关/域名 HTTPS，不是服务间 gRPC mTLS"
        extra={(
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => void fetchList()}>刷新</Button>
            {canIssue && (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
                添加域名
              </Button>
            )}
          </Space>
        )}
      />

      <Alert
        type="info"
        showIcon
        icon={<SafetyCertificateOutlined />}
        style={{ marginBottom: 16 }}
        message="使用前请确认"
        description={(
          <Typography.Paragraph style={{ marginBottom: 0 }}>
            1. 域名 A/AAAA 记录指向本机网关公网 IP（HTTP 80 可达）；
            2. 网关已把 <code>/.well-known/acme-challenge</code> 转到 system-service；
            3. 首次建议勾选 Staging 验证流程，再关 Staging 申请生产证；
            4. 签发后可下载 PEM 挂到 Traefik file 提供方，或设置服务环境变量
            {' '}
            <code>EDGE_CERT_DIR</code>
            {' '}
            自动落盘。
          </Typography.Paragraph>
        )}
      />

      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={list}
        pagination={false}
        locale={{ emptyText: <GlassEmpty text="暂无证书域名" /> }}
        scroll={{ x: 960 }}
      />

      <Modal
        title="添加域名"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void onCreate()}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form form={form} layout="vertical" initialValues={{ is_staging: true }}>
          <Form.Item
            name="domain"
            label="域名"
            rules={[
              { required: true, message: '请输入域名' },
              { pattern: /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/, message: '域名格式不正确' },
            ]}
          >
            <Input placeholder="如 admin.example.com" />
          </Form.Item>
          <Form.Item
            name="email"
            label="ACME 通知邮箱"
            rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}
          >
            <Input placeholder="用于 Let's Encrypt 账号与到期通知" />
          </Form.Item>
          <Form.Item name="is_staging" label="使用 Staging（测试）" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
