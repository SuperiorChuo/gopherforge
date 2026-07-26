import { useMemo, useState } from 'react'
import { Alert, Button, Input, Modal, Select, Space, Tabs, Tag, Tooltip, Typography } from 'antd'
import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  CloseCircleOutlined,
  SaveOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import type { CodegenArtifact, CodegenCapabilities, CodegenPlan } from '@/api/codegen'

type Props = {
  plan: CodegenPlan
  capabilities: CodegenCapabilities | null
  loading: boolean
  onDownload: () => Promise<void>
  onWrite: (confirmation: string) => Promise<void>
}

type ArtifactGroup = { key: string; label: string; artifacts: CodegenArtifact[] }

function artifactStatus(artifact: CodegenArtifact) {
  if (artifact.status === 'ready') return <Tag icon={<CheckCircleOutlined />} color="success">就绪</Tag>
  if (artifact.status === 'conflict') return <Tag icon={<WarningOutlined />} color="warning">冲突</Tag>
  return <Tag icon={<CloseCircleOutlined />} color="error">无效</Tag>
}

function ArtifactViewer({ artifacts }: { artifacts: CodegenArtifact[] }) {
  const [selectedPath, setSelectedPath] = useState(artifacts[0]?.path)
  const selected = artifacts.find((artifact) => artifact.path === selectedPath) || artifacts[0]
  if (!selected) return null
  return (
    <div style={{ minWidth: 0 }}>
      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 12 }} wrap>
        <Select
          value={selected.path}
          options={artifacts.map((artifact) => ({ label: artifact.path, value: artifact.path }))}
          onChange={setSelectedPath}
          style={{ width: 620, maxWidth: '100%' }}
        />
        {artifactStatus(selected)}
      </Space>
      <pre style={{
        height: 420,
        maxHeight: '55vh',
        overflow: 'auto',
        background: 'rgba(2, 6, 23, 0.72)',
        color: '#e2e8f0',
        border: '1px solid rgba(148, 163, 184, 0.18)',
        padding: 16,
        borderRadius: 8,
        fontSize: 13,
        lineHeight: 1.55,
        whiteSpace: 'pre',
      }}>
        {selected.operation === 'patch' ? selected.diff : selected.content}
      </pre>
    </div>
  )
}

export default function PlanPreview({ plan, capabilities, loading, onDownload, onWrite }: Props) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const groups = useMemo<ArtifactGroup[]>(() => {
    const conflicts = plan.artifacts.filter((artifact) => artifact.status !== 'ready')
    return [
      { key: 'create', label: `新增文件 ${plan.artifacts.filter((artifact) => artifact.operation === 'create' && artifact.status === 'ready').length}`, artifacts: plan.artifacts.filter((artifact) => artifact.operation === 'create' && artifact.status === 'ready') },
      { key: 'patch', label: `接入修改 ${plan.artifacts.filter((artifact) => artifact.operation === 'patch' && artifact.status === 'ready').length}`, artifacts: plan.artifacts.filter((artifact) => artifact.operation === 'patch' && artifact.status === 'ready') },
      { key: 'conflict', label: `冲突 ${conflicts.length}`, artifacts: conflicts },
    ].filter((group) => group.artifacts.length > 0)
  }, [plan.artifacts])

  const blocked = plan.diagnostics.some((diagnostic) => diagnostic.severity === 'error') ||
    plan.artifacts.some((artifact) => artifact.status !== 'ready')
  const canDownload = Boolean(capabilities?.download_enabled) && !blocked
  const canWrite = Boolean(capabilities?.write_enabled) && !blocked

  return (
    <div style={{ minWidth: 0 }}>
      {plan.diagnostics.map((diagnostic, index) => (
        <Alert
          key={`${diagnostic.code}-${diagnostic.path || index}`}
          type={diagnostic.severity === 'error' ? 'error' : 'warning'}
          showIcon
          message={diagnostic.message}
          description={diagnostic.path}
          style={{ marginBottom: 12 }}
        />
      ))}

      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 12 }} wrap>
        <Typography.Text type="secondary">
          摘要 <Typography.Text code copyable>{plan.digest}</Typography.Text>
        </Typography.Text>
        <Space wrap>
          <Tooltip title={!canDownload ? '计划存在冲突或当前环境未开放下载' : undefined}>
            <span>
              <Button disabled={!canDownload} loading={loading} icon={<CloudDownloadOutlined />} onClick={() => void onDownload()}>
                下载 ZIP
              </Button>
            </span>
          </Tooltip>
          <Tooltip title={!canWrite ? '当前环境未开放仓库写入或计划存在冲突' : undefined}>
            <span>
              <Button type="primary" disabled={!canWrite} loading={loading} icon={<SaveOutlined />} onClick={() => setConfirmOpen(true)}>
                写入仓库
              </Button>
            </span>
          </Tooltip>
        </Space>
      </Space>

      <Tabs
        items={groups.map((group) => ({
          key: group.key,
          label: group.label,
          children: <ArtifactViewer artifacts={group.artifacts} />,
        }))}
      />

      <Modal
        title="确认写入仓库"
        open={confirmOpen}
        okText="确认写入"
        cancelText="取消"
        confirmLoading={loading}
        okButtonProps={{ disabled: confirmation !== plan.request.module }}
        onCancel={() => { setConfirmOpen(false); setConfirmation('') }}
        onOk={async () => {
          await onWrite(confirmation)
          setConfirmOpen(false)
          setConfirmation('')
        }}
      >
        <Typography.Paragraph type="secondary">
          输入模块名 <Typography.Text code>{plan.request.module}</Typography.Text> 以确认本次写入。
        </Typography.Paragraph>
        <Input
          autoFocus
          aria-label="确认模块名"
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
        />
      </Modal>
    </div>
  )
}
