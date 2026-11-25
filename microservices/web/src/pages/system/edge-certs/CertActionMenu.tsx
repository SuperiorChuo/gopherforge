import { Button, Dropdown, type MenuProps } from 'antd'
import {
  CloudDownloadOutlined,
  CloudServerOutlined,
  DeleteOutlined,
  ExportOutlined,
  HistoryOutlined,
  MoreOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SyncOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type { EdgeCertCapability, EdgeCertTaskKind } from '@/api/system/edgeCert'
import {
  hasActiveTask,
  hasCertificate,
  isExternal,
  isInUse,
  type CapabilityName,
  type CertRow,
} from './helpers'

export interface CertActionMenuProps {
  row: CertRow
  size?: 'small' | 'middle' | 'large'
  canIssue: boolean
  canExport: boolean
  canDelete: boolean
  totpEnabled: boolean
  runningAction: string | null
  getCapability: (name: CapabilityName) => EdgeCertCapability
  onEdit: (row: CertRow) => void
  onEnqueue: (row: CertRow, kind: EdgeCertTaskKind) => void
  onDownloadCertificate: (row: CertRow) => void
  onOpenExport: (row: CertRow) => void
  onLoadTaskHistory: (row: CertRow) => void
  onDelete: (row: CertRow) => void
}

export function CertActionMenu({
  row,
  size,
  canIssue,
  canExport,
  canDelete,
  totpEnabled,
  runningAction,
  getCapability,
  onEdit,
  onEnqueue,
  onDownloadCertificate,
  onOpenExport,
  onLoadTaskHistory,
  onDelete,
}: CertActionMenuProps) {
  const active = hasActiveTask(row)
  const items: MenuProps['items'] = []
  const issueCap = getCapability('issue')
  const renewCap = getCapability('renew')
  const deployCap = getCapability('deploy')
  const probeCap = getCapability('probe')
  const exportCap = getCapability('export')

  if (canIssue) {
    items.push({
      key: 'config',
      icon: <SafetyCertificateOutlined />,
      label: '编辑管理方式',
      disabled: active,
      title: active ? '任务执行期间不可修改配置' : undefined,
      onClick: () => onEdit(row),
    })
    items.push({
      key: 'issue',
      icon: <ThunderboltOutlined />,
      label: '提交签发',
      disabled: active || !issueCap.enabled,
      title: issueCap.enabled ? undefined : issueCap.reason,
      onClick: () => onEnqueue(row, 'issue'),
    })
    items.push({
      key: 'renew',
      icon: <SyncOutlined />,
      label: '提交续期',
      disabled: active || !hasCertificate(row) || !renewCap.enabled,
      title: renewCap.enabled ? undefined : renewCap.reason,
      onClick: () => onEnqueue(row, 'renew'),
    })
    items.push({
      key: 'deploy',
      icon: <CloudServerOutlined />,
      label: isExternal(row) ? '外部托管，不接管部署' : '提交部署',
      disabled: active || !hasCertificate(row) || !deployCap.enabled || isExternal(row),
      title: isExternal(row) ? 'Caddy/外部网关仍是证书来源，本系统不会覆盖' : deployCap.reason,
      onClick: () => onEnqueue(row, 'deploy'),
    })
    items.push({
      key: 'probe',
      icon: <SearchOutlined />,
      label: '探测线上证书',
      disabled: active || !probeCap.enabled,
      title: probeCap.enabled ? undefined : probeCap.reason,
      onClick: () => onEnqueue(row, 'probe'),
    })
  }
  if (hasCertificate(row)) {
    items.push({
      key: 'certificate',
      icon: <CloudDownloadOutlined />,
      label: '下载证书链（不含私钥）',
      disabled: runningAction === `${row.id}:certificate`,
      onClick: () => onDownloadCertificate(row),
    })
  }
  if (canExport && hasCertificate(row)) {
    const exportDisabledReason = !totpEnabled
      ? '导出私钥前必须先在个人安全设置中启用两步验证（TOTP）'
      : exportCap.reason
    items.push({
      key: 'export',
      danger: true,
      icon: <ExportOutlined />,
      label: '二次认证后导出私钥',
      disabled: active || !totpEnabled || !exportCap.enabled,
      title: active ? '任务执行期间不可导出私钥' : exportDisabledReason,
      onClick: () => onOpenExport(row),
    })
  }
  items.push({
    key: 'history',
    icon: <HistoryOutlined />,
    label: '任务记录',
    onClick: () => onLoadTaskHistory(row),
  })
  if (canDelete) {
    items.push({ type: 'divider' })
    items.push({
      key: 'delete',
      danger: true,
      icon: <DeleteOutlined />,
      label: '删除记录',
      disabled: active || isInUse(row),
      title: active
        ? '任务执行期间不可删除'
        : isInUse(row)
          ? '证书已部署或线上正在使用，需先切换服务证书'
          : undefined,
      onClick: () => onDelete(row),
    })
  }

  return (
    <Dropdown menu={{ items }} trigger={['click']}>
      <Button size={size} type="text" icon={<MoreOutlined />} aria-label={`管理 ${row.domain}`} />
    </Dropdown>
  )
}
