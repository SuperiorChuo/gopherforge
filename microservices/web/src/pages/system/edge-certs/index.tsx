import { useCallback, useMemo, useState } from 'react'
import { Alert, Button, Grid, Modal, Space } from 'antd'
import { PlusOutlined, ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import TableToolbar from '@/components/common/TableToolbar'
import { message } from '@/utils/feedback'
import { usePermission } from '@/hooks/usePermission'
import { useAppSelector } from '@/hooks/store'
import * as API from '@/api/system/edgeCert'
import { CertFormModal } from './CertFormModal'
import { CertList } from './CertList'
import { ExportKeyModal } from './ExportKeyModal'
import { TaskHistoryDrawer } from './TaskHistoryDrawer'
import { buildCapabilityWarnings, type CertRow } from './helpers'
import { useEdgeCerts } from './useEdgeCerts'

export default function EdgeCertsPage() {
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compact = !screens.md
  const totpEnabled = useAppSelector((state) => !!state.auth.userInfo?.totp_enabled)
  const canIssue = hasPerm('system:edge-cert:issue')
  const canDelete = hasPerm('system:edge-cert:delete')
  const canExport = hasPerm('system:edge-cert:export')

  const {
    rows,
    loading,
    loaded,
    loadFailed,
    capabilities,
    capabilitiesReason,
    runningAction,
    taskModal,
    capability,
    fetchData,
    refreshAll,
    enqueue,
    downloadCertificate,
    loadTaskHistory,
    closeTaskHistory,
  } = useEdgeCerts()

  const [createOpen, setCreateOpen] = useState(false)
  const [editingRow, setEditingRow] = useState<CertRow | null>(null)
  const [exportRow, setExportRow] = useState<CertRow | null>(null)
  const [confirmModal, confirmContextHolder] = Modal.useModal()

  const openCreate = useCallback(() => {
    setEditingRow(null)
    setCreateOpen(true)
  }, [])

  const openEdit = useCallback((row: CertRow) => {
    setEditingRow(row)
    setCreateOpen(true)
  }, [])

  const closeCreateModal = useCallback(() => {
    setCreateOpen(false)
    setEditingRow(null)
  }, [])

  const openExport = useCallback((row: CertRow) => {
    setExportRow(row)
  }, [])

  const closeExportModal = useCallback(() => {
    setExportRow(null)
  }, [])

  const confirmDelete = useCallback((row: CertRow) => {
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
  }, [confirmModal, fetchData])

  const handleRetry = useCallback(() => {
    void fetchData()
  }, [fetchData])

  const capabilityWarnings = useMemo(
    () => buildCapabilityWarnings(capabilities, capabilitiesReason, canExport, totpEnabled),
    [canExport, capabilities, capabilitiesReason, totpEnabled],
  )

  return (
    <div className="page-list edge-certs-page">
      {confirmContextHolder}
      <TableToolbar
        title="边缘证书"
        description="签发、部署、线上使用是三个独立状态；本系统不会自动接管 Caddy 等外部 TLS"
        extra={(
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={refreshAll}>
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
          action={<Button size="small" onClick={handleRetry}>重试</Button>}
        />
      )}

      <CertList
        rows={rows}
        loading={loading}
        loaded={loaded}
        loadFailed={loadFailed}
        compact={compact}
        canIssue={canIssue}
        canExport={canExport}
        canDelete={canDelete}
        totpEnabled={totpEnabled}
        runningAction={runningAction}
        getCapability={capability}
        onRetry={handleRetry}
        onEdit={openEdit}
        onEnqueue={enqueue}
        onDownloadCertificate={downloadCertificate}
        onOpenExport={openExport}
        onLoadTaskHistory={loadTaskHistory}
        onDelete={confirmDelete}
      />

      <CertFormModal
        open={createOpen}
        editingRow={editingRow}
        onClose={closeCreateModal}
        onSaved={fetchData}
      />

      <ExportKeyModal
        row={exportRow}
        totpEnabled={totpEnabled}
        onClose={closeExportModal}
      />

      <TaskHistoryDrawer
        taskModal={taskModal}
        compact={compact}
        onClose={closeTaskHistory}
      />
    </div>
  )
}
