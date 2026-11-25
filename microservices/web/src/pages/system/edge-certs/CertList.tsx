import { useMemo } from 'react'
import { Alert, Button, Card, Descriptions, Space, Spin, Table, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import type { EdgeCertCapability, EdgeCertTaskKind } from '@/api/system/edgeCert'
import { CertActionMenu } from './CertActionMenu'
import { CertServingTag, CertStatusTag, CertTaskTag } from './CertStatusTags'
import {
  certificateStatusMeta,
  deploymentStatusMeta,
  isExternal,
  issuanceStatusMeta,
  renewalStatusMeta,
  type CapabilityName,
  type CertRow,
} from './helpers'

export interface CertListProps {
  rows: CertRow[]
  loading: boolean
  loaded: boolean
  loadFailed: boolean
  compact: boolean
  canIssue: boolean
  canExport: boolean
  canDelete: boolean
  totpEnabled: boolean
  runningAction: string | null
  getCapability: (name: CapabilityName) => EdgeCertCapability
  onRetry: () => void
  onEdit: (row: CertRow) => void
  onEnqueue: (row: CertRow, kind: EdgeCertTaskKind) => void
  onDownloadCertificate: (row: CertRow) => void
  onOpenExport: (row: CertRow) => void
  onLoadTaskHistory: (row: CertRow) => void
  onDelete: (row: CertRow) => void
}

export function CertList({
  rows,
  loading,
  loaded,
  loadFailed,
  compact,
  canIssue,
  canExport,
  canDelete,
  totpEnabled,
  runningAction,
  getCapability,
  onRetry,
  onEdit,
  onEnqueue,
  onDownloadCertificate,
  onOpenExport,
  onLoadTaskHistory,
  onDelete,
}: CertListProps) {
  const columns = useMemo<ColumnsType<CertRow>>(() => [
    {
      title: '域名 / 环境',
      key: 'domain',
      width: 220,
      fixed: 'left',
      render: (_, row) => (
        <Space direction="vertical" size={2} style={{ minWidth: 0 }}>
          <Typography.Text strong ellipsis={{ tooltip: row.domain }}>{row.domain}</Typography.Text>
          <Space size={4} wrap>
            {row.is_staging ? <Tag color="warning">Staging · 不受信任</Tag> : <Tag color="blue">Production</Tag>}
            <CertTaskTag task={row.active_task} />
          </Space>
        </Space>
      ),
    },
    {
      title: '签发',
      key: 'certificate',
      width: 145,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <CertStatusTag value={row.issuance.status} meta={issuanceStatusMeta} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            证书：{certificateStatusMeta[row.certificate.status]?.label || row.certificate.status}
            {row.certificate.not_after ? ` · 至 ${formatDateTime(row.certificate.not_after)}` : ''}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '部署',
      key: 'deployment',
      width: 145,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <CertStatusTag value={row.deployment.status} meta={deploymentStatusMeta} />
          <Tooltip title={isExternal(row) ? '由 Caddy/外部网关管理，本系统不会覆盖线上配置' : undefined}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {isExternal(row) ? `${row.deployment.provider || 'external'} · 不接管` : row.deployment.provider}
            </Typography.Text>
          </Tooltip>
        </Space>
      ),
    },
    {
      title: '线上使用',
      key: 'serving',
      width: 165,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <CertServingTag row={row} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {row.serving.checked_at ? `探测 ${formatDateTime(row.serving.checked_at)}` : '尚未探测'}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '续期',
      key: 'renewal',
      width: 165,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <CertStatusTag value={row.renewal.status} meta={renewalStatusMeta} />
          <Tag color={row.renewal.auto_enabled ? 'blue' : undefined}>
            {row.renewal.auto_enabled ? '自动续期已开启' : '自动续期未开启'}
          </Tag>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {row.renewal.renew_at ? `计划 ${formatDateTime(row.renewal.renew_at)}` : '无续期计划'}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '最近异常',
      key: 'error',
      width: 220,
      ellipsis: true,
      render: (_, row) => {
        const text = row.active_task?.error_message || row.serving.error_message
          || row.deployment.last_error || row.renewal.last_error || row.issuance.last_error
        return text ? <Typography.Text type="danger" ellipsis={{ tooltip: text }}>{text}</Typography.Text> : '—'
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 76,
      fixed: 'right',
      align: 'center',
      render: (_, row) => (
        <CertActionMenu
          row={row}
          size="small"
          canIssue={canIssue}
          canExport={canExport}
          canDelete={canDelete}
          totpEnabled={totpEnabled}
          runningAction={runningAction}
          getCapability={getCapability}
          onEdit={onEdit}
          onEnqueue={onEnqueue}
          onDownloadCertificate={onDownloadCertificate}
          onOpenExport={onOpenExport}
          onLoadTaskHistory={onLoadTaskHistory}
          onDelete={onDelete}
        />
      ),
    },
  ], [
    canDelete,
    canExport,
    canIssue,
    getCapability,
    onDelete,
    onDownloadCertificate,
    onEdit,
    onEnqueue,
    onLoadTaskHistory,
    onOpenExport,
    runningAction,
    totpEnabled,
  ])

  if (loadFailed && !loaded) {
    return (
      <Card>
        <GlassEmpty text="证书列表加载失败" />
        <div style={{ textAlign: 'center', marginTop: 12 }}>
          <Button icon={<ReloadOutlined />} onClick={onRetry}>重试</Button>
        </div>
      </Card>
    )
  }

  if (compact) {
    return (
      <Spin spinning={loading}>
        <div aria-busy={loading}>
          {rows.length ? rows.map((row) => (
            <Card
              key={row.id}
              size="small"
              title={<Typography.Text ellipsis={{ tooltip: row.domain }}>{row.domain}</Typography.Text>}
              extra={(
                <CertActionMenu
                  row={row}
                  canIssue={canIssue}
                  canExport={canExport}
                  canDelete={canDelete}
                  totpEnabled={totpEnabled}
                  runningAction={runningAction}
                  getCapability={getCapability}
                  onEdit={onEdit}
                  onEnqueue={onEnqueue}
                  onDownloadCertificate={onDownloadCertificate}
                  onOpenExport={onOpenExport}
                  onLoadTaskHistory={onLoadTaskHistory}
                  onDelete={onDelete}
                />
              )}
              style={{ marginBottom: 12 }}
            >
              <Space wrap style={{ marginBottom: 10 }}>
                {row.is_staging ? <Tag color="warning">Staging · 不受信任</Tag> : <Tag color="blue">Production</Tag>}
                <CertTaskTag task={row.active_task} />
              </Space>
              <Descriptions size="small" column={1} colon={false} labelStyle={{ width: 78 }}>
                <Descriptions.Item label="签发">
                  <CertStatusTag value={row.issuance.status} meta={issuanceStatusMeta} />
                  <Typography.Text type="secondary">
                    {' '}证书：{certificateStatusMeta[row.certificate.status]?.label || row.certificate.status}
                  </Typography.Text>
                </Descriptions.Item>
                <Descriptions.Item label="部署">
                  <CertStatusTag value={row.deployment.status} meta={deploymentStatusMeta} />
                  {isExternal(row) && <Typography.Text type="secondary"> Caddy/外部托管，不接管</Typography.Text>}
                </Descriptions.Item>
                <Descriptions.Item label="线上使用">
                  <CertServingTag row={row} />
                </Descriptions.Item>
                <Descriptions.Item label="证书到期">
                  {row.certificate.not_after ? formatDateTime(row.certificate.not_after) : '—'}
                </Descriptions.Item>
                <Descriptions.Item label="续期计划">
                  {row.renewal.renew_at ? formatDateTime(row.renewal.renew_at) : '未开启'}
                </Descriptions.Item>
              </Descriptions>
              {(row.serving.error_message || row.deployment.last_error || row.issuance.last_error) && (
                <Alert
                  type="error"
                  showIcon
                  style={{ marginTop: 10 }}
                  message={row.serving.error_message || row.deployment.last_error || row.issuance.last_error}
                />
              )}
            </Card>
          )) : <Card><GlassEmpty text={loading ? '正在加载证书' : '暂无证书域名'} /></Card>}
        </div>
      </Spin>
    )
  }

  return (
    <Table
      rowKey="id"
      loading={loading}
      columns={columns}
      dataSource={rows}
      pagination={false}
      locale={{ emptyText: <GlassEmpty text="暂无证书域名" /> }}
      scroll={{ x: 1140 }}
    />
  )
}
