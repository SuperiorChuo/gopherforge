import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Card, Descriptions, Drawer, Dropdown, Form, Grid, Input, List,
  Modal, Select, Space, Spin, Switch, Table, Tag, Tooltip, Typography, type MenuProps,
} from 'antd'
import {
  CloudDownloadOutlined, CloudServerOutlined, DeleteOutlined, ExportOutlined,
  HistoryOutlined, MoreOutlined, PlusOutlined, ReloadOutlined,
  SafetyCertificateOutlined, SearchOutlined, SyncOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { usePermission } from '@/hooks/usePermission'
import { useAppSelector } from '@/hooks/store'
import * as API from '@/api/system/edgeCert'
import type {
  EdgeCert, EdgeCertCapabilities, EdgeCertCapability, EdgeCertTask,
  EdgeCertTaskKind, EdgeCertTaskStatus,
} from '@/api/system/edgeCert'

type CertRow = EdgeCert & {
  certificate: API.EdgeCertCertificateState
  issuance: API.EdgeCertIssuanceState
  deployment: API.EdgeCertDeploymentState
  renewal: API.EdgeCertRenewalState
  serving: API.EdgeCertServingState
}

type TaskModalState = { row: CertRow; tasks: EdgeCertTask[]; loading: boolean } | null

const terminalTaskStatuses = new Set<EdgeCertTaskStatus>(['succeeded', 'failed'])

const taskKindMeta: Record<EdgeCertTaskKind, { label: string; submitted: string }> = {
  issue: { label: '签发', submitted: '签发任务已提交' },
  renew: { label: '续期', submitted: '续期任务已提交' },
  deploy: { label: '部署', submitted: '部署任务已提交' },
  probe: { label: '探测', submitted: '线上探测任务已提交' },
}

const taskStatusMeta: Record<EdgeCertTaskStatus, { label: string; color?: string }> = {
  queued: { label: '排队中', color: 'default' },
  running: { label: '执行中', color: 'processing' },
  succeeded: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
}

const certificateStatusMeta: Record<string, { label: string; color?: string }> = {
  none: { label: '未签发' },
  draft: { label: '未签发' },
  pending: { label: '签发中', color: 'processing' },
  issued: { label: '已签发', color: 'success' },
  valid: { label: '有效', color: 'success' },
  expiring: { label: '30 天内到期', color: 'warning' },
  failed: { label: '签发失败', color: 'error' },
  expired: { label: '已过期', color: 'warning' },
  missing: { label: '无证书' },
}

const issuanceStatusMeta: Record<string, { label: string; color?: string }> = {
  idle: { label: '未提交' },
  draft: { label: '未提交' },
  queued: { label: '等待签发' },
  pending: { label: '签发中', color: 'processing' },
  running: { label: '签发中', color: 'processing' },
  succeeded: { label: '签发成功', color: 'success' },
  issued: { label: '签发成功', color: 'success' },
  failed: { label: '签发失败', color: 'error' },
}

const deploymentStatusMeta: Record<string, { label: string; color?: string }> = {
  external: { label: '外部托管', color: 'blue' },
  not_deployed: { label: '未部署' },
  not_installed: { label: '未部署' },
  queued: { label: '等待部署', color: 'default' },
  pending: { label: '等待部署', color: 'default' },
  running: { label: '部署中', color: 'processing' },
  installed: { label: '已部署', color: 'success' },
  deployed: { label: '已部署', color: 'success' },
  failed: { label: '部署失败', color: 'error' },
}

const renewalStatusMeta: Record<string, { label: string; color?: string }> = {
  disabled: { label: '自动续期未开启' },
  awaiting_certificate: { label: '等待证书签发' },
  scheduled: { label: '已安排续期', color: 'blue' },
  due: { label: '需要续期', color: 'warning' },
  idle: { label: '续期空闲' },
  queued: { label: '等待续期' },
  running: { label: '续期中', color: 'processing' },
  completed: { label: '最近续期成功', color: 'success' },
  succeeded: { label: '最近续期成功', color: 'success' },
  failed: { label: '最近续期失败', color: 'error' },
}

const servingStatusMeta: Record<string, { label: string; color?: string }> = {
  unchecked: { label: '未探测' },
  unknown: { label: '未探测' },
  healthy: { label: '线上匹配', color: 'success' },
  matching: { label: '线上匹配', color: 'success' },
  mismatch: { label: '线上不匹配', color: 'warning' },
  unreachable: { label: '无法连接', color: 'error' },
  invalid: { label: '线上证书无效', color: 'error' },
  failed: { label: '探测失败', color: 'error' },
}

const capabilityOff: EdgeCertCapability = { enabled: false, reason: '服务端未声明此能力' }

function readError(error: unknown, fallback: string) {
  if (!error || typeof error !== 'object') return fallback
  const candidate = error as {
    message?: string
    response?: { data?: { message?: string } }
  }
  return candidate.response?.data?.message || candidate.message || fallback
}

function normalizeRow(row: EdgeCert): CertRow {
  const certificateStatus = row.certificate?.status || row.status || (row.has_cert ? 'issued' : 'draft')
  return {
    ...row,
    certificate: row.certificate ?? {
      status: certificateStatus,
      has_certificate: !!row.has_cert,
      not_before: row.not_before,
      not_after: row.not_after,
    },
    issuance: row.issuance ?? { status: certificateStatus, last_error: row.last_error },
    deployment: row.deployment ?? {
      mode: row.deployment_mode || 'external',
      provider: row.deployment_provider || 'external',
      status: row.deployment_status || 'external',
    },
    renewal: row.renewal ?? {
      status: row.last_renewal_at ? 'completed' : 'idle',
      auto_enabled: !!row.auto_renew_enabled,
      renew_at: row.renew_at,
      last_renewal_at: row.last_renewal_at,
    },
    serving: row.serving ?? {
      status: row.serving_status || 'unchecked',
      not_after: row.serving_not_after,
      issuer: row.serving_issuer,
      checked_at: row.serving_checked_at,
      error_code: row.serving_error_code,
      error_message: row.serving_error_message,
    },
  }
}

function statusTag(
  value: string | undefined,
  meta: Record<string, { label: string; color?: string }>,
) {
  const key = value || 'unknown'
  const item = meta[key] || { label: key }
  return <Tag color={item.color}>{item.label}</Tag>
}

function taskTag(task?: EdgeCertTask | null) {
  if (!task) return null
  return (
    <Tag color={taskStatusMeta[task.status]?.color || 'default'}>
      {taskKindMeta[task.kind]?.label || task.kind} · {taskStatusMeta[task.status]?.label || task.status}
    </Tag>
  )
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } finally {
    URL.revokeObjectURL(url)
  }
}

export default function EdgeCertsPage() {
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compact = !screens.md
  const totpEnabled = useAppSelector((state) => !!state.auth.userInfo?.totp_enabled)
  const canIssue = hasPerm('system:edge-cert:issue')
  const canDelete = hasPerm('system:edge-cert:delete')
  const canExport = hasPerm('system:edge-cert:export')

  const [rows, setRows] = useState<CertRow[]>([])
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [capabilities, setCapabilities] = useState<EdgeCertCapabilities | null>(null)
  const [capabilitiesReason, setCapabilitiesReason] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editingRow, setEditingRow] = useState<CertRow | null>(null)
  const [creating, setCreating] = useState(false)
  const [runningAction, setRunningAction] = useState<string | null>(null)
  const [exportRow, setExportRow] = useState<CertRow | null>(null)
  const [exporting, setExporting] = useState(false)
  const [taskModal, setTaskModal] = useState<TaskModalState>(null)
  const [createForm] = Form.useForm()
  const [exportForm] = Form.useForm()
  const createDeploymentMode = Form.useWatch('deployment_mode', createForm)
  const [confirmModal, confirmContextHolder] = Modal.useModal()
  const listRequest = useRef(0)
  const activePolls = useRef(new Map<number, number>())

  const capability = useCallback((name: keyof Pick<EdgeCertCapabilities, 'issue' | 'renew' | 'deploy' | 'probe' | 'export'>) => (
    capabilities?.[name] ?? { ...capabilityOff, reason: capabilitiesReason || capabilityOff.reason }
  ), [capabilities, capabilitiesReason])

  const fetchData = useCallback(async (quiet = false) => {
    const requestId = ++listRequest.current
    if (!quiet) setLoading(true)
    try {
      const list = await API.listEdgeCerts()
      if (requestId !== listRequest.current) return
      setRows((Array.isArray(list) ? list : []).map(normalizeRow))
      setLoaded(true)
      setLoadFailed(false)
    } catch {
      if (requestId !== listRequest.current) return
      setLoadFailed(true)
      if (!quiet) message.error('加载边缘证书失败')
    } finally {
      if (requestId === listRequest.current && !quiet) setLoading(false)
    }
  }, [])

  const fetchCapabilities = useCallback(async () => {
    try {
      const result = await API.getEdgeCertCapabilities()
      setCapabilities(result)
      setCapabilitiesReason('')
    } catch (error) {
      setCapabilities(null)
      setCapabilitiesReason(readError(error, '能力探测失败，危险操作已关闭'))
    }
  }, [])

  const pollTask = useCallback((certificateId: number, initial: EdgeCertTask) => {
    const existing = activePolls.current.get(certificateId)
    if (existing) window.clearInterval(existing)

    let task = initial
    let failureCount = 0
    const poll = async () => {
      if (terminalTaskStatuses.has(task.status)) return
      try {
        task = await API.getEdgeCertTask(certificateId, task.id)
        failureCount = 0
        setRows((current) => current.map((row) => (
          row.id === certificateId ? { ...row, active_task: task } : row
        )))
        setTaskModal((current) => current?.row.id === certificateId ? {
          ...current,
          tasks: current.tasks.map((item) => item.id === task.id ? task : item),
        } : current)
        if (!terminalTaskStatuses.has(task.status)) return

        const timer = activePolls.current.get(certificateId)
        if (timer) window.clearInterval(timer)
        activePolls.current.delete(certificateId)
        setRunningAction(null)
        if (task.status === 'succeeded') {
          message.success(`${taskKindMeta[task.kind]?.label || '任务'}已完成`)
        } else {
          message.error(task.error_message || `${taskKindMeta[task.kind]?.label || '任务'}失败`)
        }
        await fetchData(true)
      } catch {
        failureCount += 1
        if (failureCount < 3) return
        const timer = activePolls.current.get(certificateId)
        if (timer) window.clearInterval(timer)
        activePolls.current.delete(certificateId)
        setRunningAction(null)
        message.warning('任务状态暂时无法刷新，请稍后手动刷新')
      }
    }

    if (terminalTaskStatuses.has(task.status)) return
    const timer = window.setInterval(() => void poll(), 2_000)
    activePolls.current.set(certificateId, timer)
    void poll()
  }, [fetchData])

  useEffect(() => {
    void Promise.all([fetchData(), fetchCapabilities()])
  }, [fetchCapabilities, fetchData])

  useEffect(() => {
    rows.forEach((row) => {
      if (row.active_task && !terminalTaskStatuses.has(row.active_task.status) && !activePolls.current.has(row.id)) {
        pollTask(row.id, row.active_task)
      }
    })
  }, [pollTask, rows])

  useEffect(() => () => {
    activePolls.current.forEach((timer) => window.clearInterval(timer))
    activePolls.current.clear()
  }, [])

  const submitCreate = async () => {
    const values = await createForm.validateFields().catch(() => null)
    if (!values) return
    setCreating(true)
    try {
      await API.createEdgeCert({
        domain: String(values.domain).trim().toLowerCase(),
        email: String(values.email).trim(),
        is_staging: !!values.is_staging,
        deployment_mode: values.deployment_mode,
        deployment_provider: values.deployment_provider,
        auto_renew_enabled: !!values.auto_renew_enabled,
      })
      message.success(editingRow ? '证书管理配置已更新' : '域名已保存，尚未签发或部署')
      setCreateOpen(false)
      setEditingRow(null)
      createForm.resetFields()
      await fetchData()
    } catch {
      message.error('保存域名失败')
    } finally {
      setCreating(false)
    }
  }

  const openCreate = () => {
    setEditingRow(null)
    createForm.setFieldsValue({
      domain: '',
      email: '',
      is_staging: true,
      deployment_mode: 'external',
      deployment_provider: 'caddy',
      auto_renew_enabled: false,
    })
    setCreateOpen(true)
  }

  const openEdit = (row: CertRow) => {
    setEditingRow(row)
    createForm.setFieldsValue({
      domain: row.domain,
      email: row.email,
      is_staging: row.is_staging,
      deployment_mode: row.deployment.mode === 'traefik_file' ? 'traefik_file' : 'external',
      deployment_provider: row.deployment.provider || 'external',
      auto_renew_enabled: row.renewal.auto_enabled,
    })
    setCreateOpen(true)
  }

  const enqueue = async (row: CertRow, kind: EdgeCertTaskKind) => {
    const key = `${row.id}:${kind}`
    setRunningAction(key)
    try {
      const result = kind === 'issue' ? await API.issueEdgeCert(row.id)
        : kind === 'renew' ? await API.renewEdgeCert(row.id)
          : kind === 'deploy' ? await API.deployEdgeCert(row.id)
            : await API.probeEdgeCert(row.id)
      const task = result.task
      setRows((current) => current.map((item) => item.id === row.id ? { ...item, active_task: task } : item))
      message.success(result.reused ? '已有任务正在执行，已继续跟踪' : taskKindMeta[kind].submitted)
      setRunningAction(null)
      pollTask(row.id, task)
    } catch (error) {
      setRunningAction(null)
      message.error(readError(error, `${taskKindMeta[kind].label}任务提交失败`))
    }
  }

  const downloadCertificate = async (row: CertRow) => {
    setRunningAction(`${row.id}:certificate`)
    try {
      const blob = await API.downloadEdgeCertificate(row.id)
      saveBlob(blob, `${row.domain}.fullchain.pem`)
      message.success('公钥证书链已下载（不含私钥）')
    } catch {
      message.error('证书链下载失败')
    } finally {
      setRunningAction(null)
    }
  }

  const openExport = (row: CertRow) => {
    exportForm.resetFields()
    setExportRow(row)
  }

  const submitExport = async () => {
    if (!exportRow) return
    const values = await exportForm.validateFields().catch(() => null)
    if (!values) return
    setExporting(true)
    try {
      const proofResult = await API.requestEdgeCertExportStepUp({
        current_password: values.current_password,
        totp_code: values.totp_code || undefined,
        certificate_id: exportRow.id,
      })
      const proof = proofResult.proof || proofResult.step_up_proof
      if (!proof) throw new Error('服务端未返回二次认证凭证')
      const blob = await API.exportEdgeCertPrivateKey(exportRow.id, {
        step_up_proof: proof,
        confirm_domain: String(values.confirm_domain).trim().toLowerCase(),
      })
      saveBlob(blob, `${exportRow.domain}.private-key.pem`)
      message.success('私钥已导出；请离线保管，使用后及时清理下载文件')
      setExportRow(null)
      exportForm.resetFields()
    } catch (error) {
      message.error(readError(error, '私钥导出失败'))
    } finally {
      setExporting(false)
    }
  }

  const loadTaskHistory = async (row: CertRow) => {
    setTaskModal({ row, tasks: [], loading: true })
    try {
      const result = await API.listEdgeCertTasks(row.id)
      const tasks = Array.isArray(result) ? result : result.list ?? []
      setTaskModal({ row, tasks, loading: false })
    } catch {
      setTaskModal({ row, tasks: [], loading: false })
      message.error('任务记录加载失败')
    }
  }

  const confirmDelete = (row: CertRow) => {
    confirmModal.confirm({
      title: `删除 ${row.domain}？`,
      content: '只删除本系统记录；外部 Caddy/网关上正在使用的证书不会被接管或删除。此操作无法恢复。',
      okText: '删除记录',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        await API.deleteEdgeCert(row.id)
        message.success('证书记录已删除')
        await fetchData()
      },
    })
  }

  const hasActiveTask = (row: CertRow) => !!row.active_task && !terminalTaskStatuses.has(row.active_task.status)
  const isServing = (row: CertRow) => ['healthy', 'matching'].includes(row.serving.status)
  const isInUse = (row: CertRow) => row.serving.managed_certificate_in_use === true
    || (!isExternal(row) && isServing(row))
    || ['installed', 'deployed'].includes(row.deployment.status)
  const hasCertificate = (row: CertRow) => row.certificate.has_certificate
    || ['issued', 'valid'].includes(row.certificate.status)
  const isExternal = (row: CertRow) => row.deployment.mode === 'external'
    || row.deployment.provider === 'external'
    || row.deployment.provider === 'caddy'
  const servingTag = (row: CertRow) => {
    if (['healthy', 'matching'].includes(row.serving.status)) {
      return isExternal(row)
        ? <Tag color="success">外部证书正常</Tag>
        : <Tag color="success">本系统证书线上使用中</Tag>
    }
    return statusTag(row.serving.status, servingStatusMeta)
  }

  const actionMenu = (row: CertRow): MenuProps['items'] => {
    const active = hasActiveTask(row)
    const items: MenuProps['items'] = []
    const issueCap = capability('issue')
    const renewCap = capability('renew')
    const deployCap = capability('deploy')
    const probeCap = capability('probe')
    const exportCap = capability('export')

    if (canIssue) {
      items.push({
        key: 'config', icon: <SafetyCertificateOutlined />, label: '编辑管理方式',
        disabled: active,
        title: active ? '任务执行期间不可修改配置' : undefined,
        onClick: () => openEdit(row),
      })
      items.push({
        key: 'issue', icon: <ThunderboltOutlined />, label: '提交签发',
        disabled: active || !issueCap.enabled,
        title: issueCap.enabled ? undefined : issueCap.reason,
        onClick: () => void enqueue(row, 'issue'),
      })
      items.push({
        key: 'renew', icon: <SyncOutlined />, label: '提交续期',
        disabled: active || !hasCertificate(row) || !renewCap.enabled,
        title: renewCap.enabled ? undefined : renewCap.reason,
        onClick: () => void enqueue(row, 'renew'),
      })
      items.push({
        key: 'deploy', icon: <CloudServerOutlined />, label: isExternal(row) ? '外部托管，不接管部署' : '提交部署',
        disabled: active || !hasCertificate(row) || !deployCap.enabled || isExternal(row),
        title: isExternal(row) ? 'Caddy/外部网关仍是证书来源，本系统不会覆盖' : deployCap.reason,
        onClick: () => void enqueue(row, 'deploy'),
      })
      items.push({
        key: 'probe', icon: <SearchOutlined />, label: '探测线上证书',
        disabled: active || !probeCap.enabled,
        title: probeCap.enabled ? undefined : probeCap.reason,
        onClick: () => void enqueue(row, 'probe'),
      })
    }
    if (hasCertificate(row)) {
      items.push({
        key: 'certificate', icon: <CloudDownloadOutlined />, label: '下载证书链（不含私钥）',
        disabled: runningAction === `${row.id}:certificate`,
        onClick: () => void downloadCertificate(row),
      })
    }
    if (canExport && hasCertificate(row)) {
      const exportDisabledReason = !totpEnabled
        ? '导出私钥前必须先在个人安全设置中启用两步验证（TOTP）'
        : exportCap.reason
      items.push({
        key: 'export', danger: true, icon: <ExportOutlined />, label: '二次认证后导出私钥',
        disabled: active || !totpEnabled || !exportCap.enabled,
        title: active ? '任务执行期间不可导出私钥' : exportDisabledReason,
        onClick: () => openExport(row),
      })
    }
    items.push({
      key: 'history', icon: <HistoryOutlined />, label: '任务记录',
      onClick: () => void loadTaskHistory(row),
    })
    if (canDelete) {
      items.push({ type: 'divider' })
      items.push({
        key: 'delete', danger: true, icon: <DeleteOutlined />, label: '删除记录',
        disabled: active || isInUse(row),
        title: active ? '任务执行期间不可删除' : isInUse(row) ? '证书已部署或线上正在使用，需先切换服务证书' : undefined,
        onClick: () => confirmDelete(row),
      })
    }
    return items
  }

  const columns = useMemo<ColumnsType<CertRow>>(() => [
    {
      title: '域名 / 环境', key: 'domain', width: 220, fixed: 'left',
      render: (_, row) => (
        <Space direction="vertical" size={2} style={{ minWidth: 0 }}>
          <Typography.Text strong ellipsis={{ tooltip: row.domain }}>{row.domain}</Typography.Text>
          <Space size={4} wrap>
            {row.is_staging ? <Tag color="warning">Staging · 不受信任</Tag> : <Tag color="blue">Production</Tag>}
            {taskTag(row.active_task)}
          </Space>
        </Space>
      ),
    },
    {
      title: '签发', key: 'certificate', width: 145,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          {statusTag(row.issuance.status, issuanceStatusMeta)}
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            证书：{certificateStatusMeta[row.certificate.status]?.label || row.certificate.status}
            {row.certificate.not_after ? ` · 至 ${formatDateTime(row.certificate.not_after)}` : ''}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '部署', key: 'deployment', width: 145,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          {statusTag(row.deployment.status, deploymentStatusMeta)}
          <Tooltip title={isExternal(row) ? '由 Caddy/外部网关管理，本系统不会覆盖线上配置' : undefined}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {isExternal(row) ? `${row.deployment.provider || 'external'} · 不接管` : row.deployment.provider}
            </Typography.Text>
          </Tooltip>
        </Space>
      ),
    },
    {
      title: '线上使用', key: 'serving', width: 165,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          {servingTag(row)}
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {row.serving.checked_at ? `探测 ${formatDateTime(row.serving.checked_at)}` : '尚未探测'}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '续期', key: 'renewal', width: 165,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          {statusTag(row.renewal.status, renewalStatusMeta)}
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
      title: '最近异常', key: 'error', width: 220, ellipsis: true,
      render: (_, row) => {
        const text = row.active_task?.error_message || row.serving.error_message
          || row.deployment.last_error || row.renewal.last_error || row.issuance.last_error
        return text ? <Typography.Text type="danger" ellipsis={{ tooltip: text }}>{text}</Typography.Text> : '—'
      },
    },
    {
      title: '操作', key: 'actions', width: 76, fixed: 'right', align: 'center',
      render: (_, row) => (
        <Dropdown menu={{ items: actionMenu(row) }} trigger={['click']}>
          <Button size="small" type="text" icon={<MoreOutlined />} aria-label={`管理 ${row.domain}`} />
        </Dropdown>
      ),
    },
  // actionMenu intentionally reflects the latest capability and task state.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [capabilities, capabilitiesReason, runningAction, canDelete, canExport, canIssue, totpEnabled])

  const capabilityWarnings = useMemo(() => {
    const warnings = capabilities ? (['issue', 'renew', 'deploy', 'probe', 'export'] as const)
      .map((name) => capabilities[name] ?? capabilityOff)
      .filter((value) => !value.enabled && value.reason)
      .map((value) => value.reason as string)
      .filter((value, index, all) => all.indexOf(value) === index)
      : capabilitiesReason ? [capabilitiesReason] : []
    if (canExport && !totpEnabled) {
      warnings.push('私钥导出已锁定：请先在个人安全设置中启用两步验证（TOTP）')
    }
    return warnings
  }, [canExport, capabilities, capabilitiesReason, totpEnabled])

  const rowCard = (row: CertRow) => (
    <Card
      key={row.id}
      size="small"
      title={<Typography.Text ellipsis={{ tooltip: row.domain }}>{row.domain}</Typography.Text>}
      extra={(
        <Dropdown menu={{ items: actionMenu(row) }} trigger={['click']}>
          <Button type="text" icon={<MoreOutlined />} aria-label={`管理 ${row.domain}`} />
        </Dropdown>
      )}
      style={{ marginBottom: 12 }}
    >
      <Space wrap style={{ marginBottom: 10 }}>
        {row.is_staging ? <Tag color="warning">Staging · 不受信任</Tag> : <Tag color="blue">Production</Tag>}
        {taskTag(row.active_task)}
      </Space>
      <Descriptions size="small" column={1} colon={false} labelStyle={{ width: 78 }}>
        <Descriptions.Item label="签发">
          {statusTag(row.issuance.status, issuanceStatusMeta)}
          <Typography.Text type="secondary">
            {' '}证书：{certificateStatusMeta[row.certificate.status]?.label || row.certificate.status}
          </Typography.Text>
        </Descriptions.Item>
        <Descriptions.Item label="部署">
          {statusTag(row.deployment.status, deploymentStatusMeta)}
          {isExternal(row) && <Typography.Text type="secondary"> Caddy/外部托管，不接管</Typography.Text>}
        </Descriptions.Item>
        <Descriptions.Item label="线上使用">{servingTag(row)}</Descriptions.Item>
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
  )

  return (
    <div className="page-list edge-certs-page">
      {confirmContextHolder}
      <TableToolbar
        title="边缘证书"
        description="签发、部署、线上使用是三个独立状态；本系统不会自动接管 Caddy 等外部 TLS"
        extra={(
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => void Promise.all([fetchData(), fetchCapabilities()])}>
              刷新
            </Button>
            {canIssue && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
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
        style={{ marginBottom: 12 }}
        message="签发不等于已经上线"
        description="证书签发成功，只代表 PEM 已安全保存。只有部署状态成功且线上探测匹配，才能确认访客实际使用了该证书。若显示“外部托管”，Caddy/外部网关仍是线上证书来源，本系统只管理记录和探测，不会覆盖它。"
      />
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: capabilityWarnings.length ? 8 : 16 }}
        message="Staging 仅用于流程验证"
        description="Staging 证书不受浏览器信任，不可用于正式流量。测试通过后应新建或切换为 Production，再重新签发、部署和探测。"
      />
      {capabilityWarnings.length > 0 && (
        <Alert
          type="warning"
          showIcon
          closable
          style={{ marginBottom: 16 }}
          message="部分能力当前不可用"
          description={capabilityWarnings.join('；')}
        />
      )}
      {loadFailed && loaded && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="刷新失败，当前显示上次成功加载的数据"
          action={<Button size="small" onClick={() => void fetchData()}>重试</Button>}
        />
      )}

      {loadFailed && !loaded ? (
        <Card>
          <GlassEmpty text="证书列表加载失败" />
          <div style={{ textAlign: 'center', marginTop: 12 }}>
            <Button icon={<ReloadOutlined />} onClick={() => void fetchData()}>重试</Button>
          </div>
        </Card>
      ) : compact ? (
        <Spin spinning={loading}>
          <div aria-busy={loading}>
            {rows.length ? rows.map(rowCard) : <Card><GlassEmpty text={loading ? '正在加载证书' : '暂无证书域名'} /></Card>}
          </div>
        </Spin>
      ) : (
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={rows}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无证书域名" /> }}
          scroll={{ x: 1140 }}
        />
      )}

      <Modal
        title={editingRow ? '编辑证书管理配置' : '添加域名'}
        open={createOpen}
        onCancel={() => { setCreateOpen(false); setEditingRow(null); createForm.resetFields() }}
        onOk={() => void submitCreate()}
        confirmLoading={creating}
        okText="保存域名"
        destroyOnHidden
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="保存后不会自动签发或修改网关"
        />
        <Form
          form={createForm}
          layout="vertical"
          initialValues={{
            is_staging: true,
            deployment_mode: 'external',
            deployment_provider: 'caddy',
            auto_renew_enabled: false,
          }}
        >
          <Form.Item
            name="domain"
            label="域名"
            normalize={(value) => String(value || '').trim().toLowerCase()}
            rules={[
              { required: true, message: '请输入域名' },
              { pattern: /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/, message: '请输入单个完整域名，不含协议、端口或路径' },
            ]}
          >
            <Input placeholder="如 admin.example.com" autoComplete="off" disabled={!!editingRow} />
          </Form.Item>
          <Form.Item name="email" label="ACME 通知邮箱" rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}>
            <Input placeholder="用于 CA 账号与到期通知" autoComplete="email" />
          </Form.Item>
          <Form.Item name="is_staging" label="使用 Staging（建议先验证）" valuePropName="checked">
            <Switch
              checkedChildren="测试"
              unCheckedChildren="正式"
              disabled={createDeploymentMode === 'traefik_file' || (!!editingRow && hasCertificate(editingRow))}
            />
          </Form.Item>
          <Form.Item
            name="deployment_mode"
            label="证书部署方式"
            rules={[{ required: true, message: '请选择证书部署方式' }]}
          >
            <Select
              options={[
                { value: 'external', label: '外部托管（Caddy / CDN）' },
                { value: 'traefik_file', label: '由本系统部署到 Traefik' },
              ]}
              onChange={(value) => {
                if (value === 'traefik_file') {
                  createForm.setFieldsValue({
                    is_staging: false,
                    deployment_provider: 'traefik',
                    auto_renew_enabled: true,
                  })
                } else {
                  createForm.setFieldsValue({
                    deployment_provider: 'caddy',
                    auto_renew_enabled: false,
                  })
                }
              }}
            />
          </Form.Item>
          {createDeploymentMode === 'traefik_file' ? (
            <>
              <Alert
                type="warning"
                showIcon
                style={{ marginBottom: 16 }}
                message="本系统将写入 Traefik 动态证书配置"
                description="仅用于 DNS 与流量确实指向本机 Traefik 的生产域名；当前由 Caddy 托管的域名不要选择此项。"
              />
              <Form.Item name="auto_renew_enabled" label="自动续期" valuePropName="checked">
                <Switch checkedChildren="开启" unCheckedChildren="关闭" />
              </Form.Item>
            </>
          ) : (
            <Form.Item name="deployment_provider" label="外部 TLS 提供方" rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'caddy', label: 'Caddy' },
                  { value: 'cdn', label: 'CDN / 云网关' },
                  { value: 'other', label: '其他外部提供方' },
                  { value: 'external', label: '未指定' },
                ]}
              />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <Modal
        title="导出证书私钥"
        open={!!exportRow}
        onCancel={() => { setExportRow(null); exportForm.resetFields() }}
        onOk={() => void submitExport()}
        okText="认证并导出私钥"
        okButtonProps={{ danger: true }}
        confirmLoading={exporting}
        maskClosable={false}
        keyboard={!exporting}
        destroyOnHidden
      >
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message="私钥一旦泄露，任何人都可能冒充此域名"
          description="此操作会被审计。请仅在离线迁移或故障恢复时导出；不要通过聊天、工单或邮件传输。"
        />
        <Form form={exportForm} layout="vertical" autoComplete="off">
          <Form.Item
            name="current_password"
            label="当前登录密码"
            rules={[{ required: true, message: '请输入当前登录密码' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          {totpEnabled && (
            <Form.Item
              name="totp_code"
              label="TOTP 动态验证码"
              rules={[
                { required: true, message: '请输入动态验证码' },
                { pattern: /^\d{6}$/, message: '请输入 6 位数字验证码' },
              ]}
            >
              <Input inputMode="numeric" maxLength={6} autoComplete="one-time-code" placeholder="6 位数字" />
            </Form.Item>
          )}
          <Form.Item
            name="confirm_domain"
            label={<>输入完整域名确认：<Typography.Text code>{exportRow?.domain}</Typography.Text></>}
            dependencies={[]}
            rules={[
              { required: true, message: '请输入完整域名' },
              {
                validator: (_, value) => String(value || '').trim().toLowerCase() === exportRow?.domain
                  ? Promise.resolve()
                  : Promise.reject(new Error('域名不一致')),
              },
            ]}
          >
            <Input autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={taskModal ? `${taskModal.row.domain} · 任务记录` : '任务记录'}
        open={!!taskModal}
        onClose={() => setTaskModal(null)}
        width={compact ? '100%' : 560}
      >
        <List
          loading={taskModal?.loading}
          dataSource={taskModal?.tasks ?? []}
          locale={{ emptyText: <GlassEmpty text="暂无任务记录" /> }}
          renderItem={(task) => (
            <List.Item>
              <List.Item.Meta
                title={(
                  <Space wrap>
                    <Typography.Text strong>{taskKindMeta[task.kind]?.label || task.kind}</Typography.Text>
                    {statusTag(task.status, taskStatusMeta)}
                    {task.environment === 'staging' && <Tag color="warning">Staging</Tag>}
                  </Space>
                )}
                description={(
                  <Space direction="vertical" size={2}>
                    <Typography.Text type="secondary">
                      提交：{formatDateTime(task.created_at)}
                      {task.finished_at ? ` · 完成：${formatDateTime(task.finished_at)}` : ''}
                    </Typography.Text>
                    {task.step && <Typography.Text type="secondary">阶段：{task.step}</Typography.Text>}
                    {task.error_message && <Typography.Text type="danger">{task.error_message}</Typography.Text>}
                    {task.error_hint && <Typography.Text type="secondary">建议：{task.error_hint}</Typography.Text>}
                  </Space>
                )}
              />
            </List.Item>
          )}
        />
      </Drawer>
    </div>
  )
}
