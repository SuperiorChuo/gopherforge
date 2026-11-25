import { Tag } from 'antd'
import type { EdgeCertTask } from '@/api/system/edgeCert'
import {
  isExternal,
  servingStatusMeta,
  taskKindMeta,
  taskStatusMeta,
  type CertRow,
  type StatusMetaItem,
} from './helpers'

interface CertStatusTagProps {
  value: string | undefined
  meta: Record<string, StatusMetaItem>
}

export function CertStatusTag({ value, meta }: CertStatusTagProps) {
  const key = value || 'unknown'
  const item = meta[key] || { label: key }
  return <Tag color={item.color}>{item.label}</Tag>
}

interface CertTaskTagProps {
  task?: EdgeCertTask | null
}

export function CertTaskTag({ task }: CertTaskTagProps) {
  if (!task) return null
  return (
    <Tag color={taskStatusMeta[task.status]?.color || 'default'}>
      {taskKindMeta[task.kind]?.label || task.kind} · {taskStatusMeta[task.status]?.label || task.status}
    </Tag>
  )
}

interface CertServingTagProps {
  row: CertRow
}

export function CertServingTag({ row }: CertServingTagProps) {
  if (['healthy', 'matching'].includes(row.serving.status)) {
    return isExternal(row)
      ? <Tag color="success">外部证书正常</Tag>
      : <Tag color="success">本系统证书线上使用中</Tag>
  }
  return <CertStatusTag value={row.serving.status} meta={servingStatusMeta} />
}
