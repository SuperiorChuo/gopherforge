import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Upload , Grid } from 'antd'
import { InboxOutlined, PlusOutlined, ReloadOutlined, CheckOutlined, StopOutlined, DisconnectOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { UploadFile } from 'antd/es/upload/interface'
import {
  approveHermesSkill,
  bindClusterSkill,
  browseHermesSkillHub,
  importHermesSkillHub,
  listClusterSkills,
  listHermesClusters,
  listHermesSkills,
  rejectHermesSkill,
  revokeHermesSkill,
  unbindClusterSkill,
  uploadHermesSkill,
  type HermesCluster,
  type HermesSkill,
  type HermesSkillHubBrowse,
  type HermesSkillStatus,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { usePermission } from '@/hooks/usePermission'
import { usePagination } from '@/hooks/usePagination'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { useIncrementalList } from '../useIncrementalList'
import { fetchAllHermesPages } from '../admin-contract'

const STATUS_META: Record<HermesSkillStatus, { label: string; tone: 'success' | 'warning' | 'danger' | 'muted' }> = {
  pending: { label: '待审批', tone: 'warning' },
  approved: { label: '已批准', tone: 'success' },
  rejected: { label: '已拒绝', tone: 'danger' },
  revoked: { label: '已吊销', tone: 'danger' },
}

export default function HermesSkillsMarketPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:skill:write')
  const canApprove = hasPerm('ai:hermes:skill:approve')
  const canBind = hasPerm('ai:hermes:skill:bind')

  const [loading, setLoading] = useState(false)
  const [skills, setSkills] = useState<HermesSkill[]>([])
  const [total, setTotal] = useState(0)

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  // 手写状态页的请求代号门控：快速连点翻页时旧响应不得覆盖新页
  const loadGenerationRef = useRef(0)
  const loadData = useCallback(async (p: typeof params) => {
    const generation = loadGenerationRef.current + 1
    loadGenerationRef.current = generation
    setLoading(true)
    try {
      const skillResult = await listHermesSkills(p)
      if (generation !== loadGenerationRef.current) return
      setSkills(skillResult.list ?? [])
      setTotal(skillResult.total ?? 0)
    } catch {
      // Request layer already reports the server message.
    } finally {
      if (generation === loadGenerationRef.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData(params)
  }, [loadData, params])

  // 待审批聚合：全量统计（逐页真实拉取，不放大 page_size 伪装全量）
  const [pendingCount, setPendingCount] = useState(0)
  const loadStats = useCallback(async () => {
    try {
      const all = await fetchAllHermesPages<HermesSkill>((page, pageSize) =>
        listHermesSkills({ page, page_size: pageSize }))
      setPendingCount(all.filter((s) => s.status === 'pending').length)
    } catch {
      // 统计失败保留上次值；请求层已提示服务端消息
    }
  }, [])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  // 集群下拉：按页追加加载
  const clusters = useIncrementalList<HermesCluster>(
    useCallback((page, pageSize) => listHermesClusters({ page, page_size: pageSize }), []),
  )

  // 绑定下拉：已审批技能独立按页追加，不与主表分页纠缠
  const bindSkills = useIncrementalList<HermesSkill>(
    useCallback((page, pageSize) => listHermesSkills({ page, page_size: pageSize }), []),
  )

  const [uploadOpen, setUploadOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [fileList, setFileList] = useState<UploadFile[]>([])

  // 集群绑定视图
  const [selectedCluster, setSelectedCluster] = useState<number>()
  const [clusterSkills, setClusterSkills] = useState<HermesSkill[]>([])

  // 官方市场（GitHub 系源）
  const [hubSource, setHubSource] = useState('official')
  const [hubCustomRepo, setHubCustomRepo] = useState('')
  const [hubCustomPath, setHubCustomPath] = useState('')
  const [hubBrowse, setHubBrowse] = useState<HermesSkillHubBrowse | null>(null)
  const [hubLoading, setHubLoading] = useState(false)
  const [hubImporting, setHubImporting] = useState('')

  const browseHub = useCallback(async () => {
    setHubLoading(true)
    try {
      setHubBrowse(
        await browseHermesSkillHub({
          source: hubSource,
          repo: hubSource === 'custom' ? hubCustomRepo : undefined,
          path: hubSource === 'custom' ? hubCustomPath : undefined,
        }),
      )
    } catch {
      // Request layer already reports the server message.
    } finally {
      setHubLoading(false)
    }
  }, [hubSource, hubCustomRepo, hubCustomPath])

  const importHub = async (skillPath: string) => {
    if (!hubBrowse) return
    setHubImporting(skillPath)
    try {
      await importHermesSkillHub({
        source: hubSource,
        custom_repo: hubSource === 'custom' ? hubCustomRepo : undefined,
        custom_path: hubSource === 'custom' ? hubCustomPath : undefined,
        skill_path: skillPath,
        ref: hubBrowse.commit,
      })
      message.success(t('技能已导入，待审批后可绑定/自装'))
      void loadData(params)
      void loadStats()
      void bindSkills.reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setHubImporting('')
    }
  }

  const loadClusterSkills = useCallback(async (clusterId: number) => {
    try {
      setClusterSkills(await listClusterSkills(clusterId))
    } catch {
      // Request layer already reports the server message.
    }
  }, [])

  useEffect(() => {
    if (selectedCluster) void loadClusterSkills(selectedCluster)
  }, [selectedCluster, loadClusterSkills])

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      message.success(ok)
      void loadData(params)
      if (selectedCluster) void loadClusterSkills(selectedCluster)
    } catch {
      // Request layer already reports the server message.
    }
  }

  const openUpload = () => {
    form.resetFields()
    setFileList([])
    setUploadOpen(true)
  }

  const handleUpload = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    if (fileList.length === 0) {
      message.warning(t('请选择技能包文件（tar.gz）'))
      return
    }
    setSubmitting(true)
    try {
      const data = new FormData()
      data.append('name', values.name)
      data.append('display_name', values.display_name ?? '')
      data.append('description', values.description ?? '')
      data.append('version', values.version)
      data.append('signature', values.signature ?? '')
      data.append('file', fileList[0].originFileObj as Blob)
      await uploadHermesSkill(data)
      message.success(t('技能已录入，等待审批'))
      setUploadOpen(false)
      void loadData(params)
      void loadStats()
      void bindSkills.reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<HermesSkill> = [
    {
      title: t('技能'),
      dataIndex: 'display_name',
      render: (value: string, record) => (
        <div>
          <div>{value || record.name}</div>
          <span className="cell-mono cell-muted">{record.name} · v{record.version}</span>
        </div>
      ),
    },
    { title: t('描述'), dataIndex: 'description', ellipsis: true, render: (value: string) => <span className="cell-muted">{value || '—'}</span> },
    {
      title: t('来源'),
      dataIndex: 'source_repo',
      width: 150,
      ellipsis: true,
      render: (value: string, record) =>
        value ? (
          <span className="cell-muted" title={`${value}/${record.source_path ?? ''}@${(record.source_commit ?? '').slice(0, 12)}`}>
            {t('官方市场 · {{name}}', { name: value })}
          </span>
        ) : (
          <span className="cell-muted">{t('手工上传')}</span>
        ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value: HermesSkillStatus) => {
        const meta = STATUS_META[value]
        return <StatusPill tone={meta.tone} label={meta.label} />
      },
    },
    {
      title: t('签名'),
      dataIndex: 'signature_status',
      width: 90,
      render: (value: string) => {
        if (value === 'verified') return <Tag color="success">{t('已验签')}</Tag>
        if (value === 'not_applicable') return <Tag color="purple">{t('官方市场')}</Tag>
        return <Tag color="warning">{t('未签名')}</Tag>
      },
    },
    { title: t('摘要'), dataIndex: 'sha256', width: 90, render: (value: string) => <span className="cell-mono cell-muted">{value.slice(0, 8)}…</span> },
    { title: t('录入时间'), dataIndex: 'created_at', width: 160, render: (value: string) => formatDateTime(value) },
    {
      title: t('操作'),
      width: compactActions ? 48 : 120,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            {
              key: 'approve',
              label: t('批准'),
              icon: <CheckOutlined />,
              show: canApprove && record.status === 'pending',
              onClick: () => { void act(() => approveHermesSkill(record.id), t('技能已批准')) },
            },
            {
              key: 'reject',
              label: t('拒绝'),
              icon: <StopOutlined />,
              danger: true,
              show: canApprove && record.status === 'pending',
              onClick: () => { void act(() => rejectHermesSkill(record.id), t('技能已拒绝')) },
            },
            {
              key: 'revoke',
              label: t('吊销'),
              icon: <StopOutlined />,
              danger: true,
              show: canApprove && record.status === 'approved',
              confirm: { title: t('吊销该技能？'), description: t('已绑定集群的该版本技能会停止下发。') },
              onClick: () => { void act(() => revokeHermesSkill(record.id), t('技能已吊销')) },
            },
          ]}
        />
      ),
    },
  ]

  const boundColumns: ColumnsType<HermesSkill> = [
    { title: t('技能'), dataIndex: 'display_name', render: (value: string, record) => <div><div>{value || record.name}</div><span className="cell-mono cell-muted">{record.name} · v{record.version}</span></div> },
    { title: t('状态'), dataIndex: 'status', width: 100, render: (value: HermesSkillStatus) => { const m = STATUS_META[value]; return <StatusPill tone={m.tone} label={m.label} /> } },
    { title: t('摘要'), dataIndex: 'sha256', width: 90, render: (value: string) => <span className="cell-mono cell-muted">{value.slice(0, 8)}…</span> },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      render: (_, record) =>
        canBind ? (
          <TableRowActions
            maxInline={1}
            actions={[
              {
                key: 'unbind',
                label: t('解绑'),
                icon: <DisconnectOutlined />,
                danger: true,
                confirm: t('从该集群解绑？'),
                onClick: () => {
                  if (selectedCluster) void act(() => unbindClusterSkill(selectedCluster, record.id), t('已解绑'))
                },
              },
            ]}
          />
        ) : null,
    },
  ]

  const availableToBind = useMemo(
    () => bindSkills.items.filter((s) => s.status === 'approved' && !clusterSkills.some((b) => b.id === s.id)),
    [bindSkills.items, clusterSkills],
  )

  return (
    <div className="page-list">
      <TableToolbar
        title="技能市场"
        total={total}
        icon={<InboxOutlined />}
        gradient="linear-gradient(135deg, #0ea5e9, #ec4899)"
        glow="rgba(14, 165, 233, 0.24)"
        description={t('{{n}} 个待审批技能 · 集群绑定后随策略下发到员工 Hermes', { n: pendingCount })}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => { void loadData(params); void loadStats(); void clusters.reload(); void bindSkills.reload() }} loading={loading} />
            {canWrite && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openUpload}>
                {t('上传技能')}
              </Button>
            )}
          </Space>
        }
      />

      <Card className="list-main-card">
        <Table
          rowKey="id"
          className="list-table"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={skills}
          scroll={{ x: 900 }}
          locale={{ emptyText: <GlassEmpty text="暂无技能" compact /> }}
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
      </Card>

      {canWrite && (
        <Card
          className="list-main-card"
          title={t('官方市场')}
          style={{ marginTop: 16 }}
          extra={<span className="cell-muted">{t('从上游 Skills Hub（GitHub 源）导入，导入后仍需审批')}</span>}
        >
          <Space wrap style={{ marginBottom: 16 }}>
            <Select
              value={hubSource}
              onChange={(value) => {
                setHubSource(value)
                setHubBrowse(null)
              }}
              style={{ width: 240 }}
              options={[
                { value: 'official', label: t('Hermes 官方（optional-skills）') },
                { value: 'openai', label: 'OpenAI skills' },
                { value: 'anthropics', label: 'Anthropic skills' },
                { value: 'huggingface', label: 'HuggingFace skills' },
                { value: 'nvidia', label: 'NVIDIA skills' },
                { value: 'custom', label: t('自定义 GitHub 仓') },
              ]}
            />
            {hubSource === 'custom' && (
              <>
                <Input
                  value={hubCustomRepo}
                  onChange={(event) => setHubCustomRepo(event.target.value.trim())}
                  placeholder="owner/repo"
                  style={{ width: 220 }}
                  className="cell-mono"
                />
                <Input
                  value={hubCustomPath}
                  onChange={(event) => setHubCustomPath(event.target.value.trim())}
                  placeholder={t('技能目录前缀（可空）')}
                  style={{ width: 200 }}
                  className="cell-mono"
                />
              </>
            )}
            <Button type="primary" loading={hubLoading} onClick={() => void browseHub()}>
              {t('浏览')}
            </Button>
            {hubBrowse && (
              <span className="cell-muted cell-mono">
                {hubBrowse.repo}@{hubBrowse.commit.slice(0, 12)}
              </span>
            )}
          </Space>
          <Table
            rowKey="path"
            className="list-table"
            size="small"
            loading={hubLoading}
            dataSource={hubBrowse?.entries ?? []}
            pagination={(hubBrowse?.entries.length ?? 0) > 10 ? { pageSize: 10 } : false}
            columns={[
              { title: t('技能目录'), dataIndex: 'name', width: 240, render: (value: string) => <span className="cell-mono">{value}</span> },
              { title: t('路径'), dataIndex: 'path', ellipsis: true, render: (value: string) => <span className="cell-muted cell-mono">{value}</span> },
              {
                title: t('操作'),
                width: 90,
                render: (_, record) => (
                  <Button
                    size="small"
                    type="primary"
                    loading={hubImporting === record.path}
                    disabled={!!hubImporting && hubImporting !== record.path}
                    onClick={() => void importHub(record.path)}
                  >
                    {t('导入')}
                  </Button>
                ),
              },
            ]}
            locale={{ emptyText: <GlassEmpty text="选择源后点「浏览」拉取技能目录" compact /> }}
          />
        </Card>
      )}

      <Card className="list-main-card" title={t('集群技能绑定')} style={{ marginTop: 16 }}>
        <Space wrap style={{ marginBottom: 16 }}>
          <Select<number>
            placeholder={t('选择集群')}
            style={{ width: 260 }}
            value={selectedCluster}
            onChange={(value) => setSelectedCluster(value)}
            onPopupScroll={clusters.loadMore}
            loading={clusters.loading}
            options={clusters.items.filter((c) => c.enabled).map((c) => ({ value: c.id, label: `${c.name}${c.slug ? ` · ${c.slug}` : ''}` }))}
          />
          {canBind && selectedCluster && (
            <Select<number>
              placeholder={t('绑定已审批技能')}
              style={{ width: 260 }}
              value={undefined}
              onChange={(skillId) => {
                if (skillId) void act(() => bindClusterSkill(selectedCluster, skillId), t('技能已绑定'))
              }}
              onPopupScroll={bindSkills.loadMore}
              loading={bindSkills.loading}
              options={availableToBind.map((s) => ({ value: s.id, label: `${s.display_name || s.name} · v${s.version}` }))}
            />
          )}
        </Space>
        {selectedCluster ? (
          <Table
            rowKey="id"
            className="list-table"
            size="small"
            columns={boundColumns}
            dataSource={clusterSkills}
            pagination={false}
            locale={{ emptyText: <GlassEmpty text="该集群未绑定技能" compact /> }}
          />
        ) : (
          <div className="cell-muted">{t('选择一个集群查看已绑定技能。绑定后该集群所有员工 Hermes 都会装上这些技能。')}</div>
        )}
      </Card>

      <Modal
        title={t('上传技能包')}
        open={uploadOpen}
        onCancel={() => setUploadOpen(false)}
        onOk={() => void handleUpload()}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label={t('技能名（目录名）')} rules={[{ required: true, message: t('请输入技能名') }, { pattern: /^[a-z0-9][a-z0-9._-]{0,79}$/, message: t('小写字母数字 + . _ -，会成为目录名') }]}>
            <Input placeholder={t('如：weekly-report')} />
          </Form.Item>
          <Form.Item name="display_name" label={t('显示名')}>
            <Input placeholder={t('如：周报生成')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={3} maxLength={10000} />
          </Form.Item>
          <Form.Item name="version" label={t('版本号')} rules={[{ required: true, message: t('请输入版本号') }, { pattern: /^[A-Za-z0-9][A-Za-z0-9._+-]{0,38}$/, message: t('不可变发布 tag') }]}>
            <Input placeholder={t('如：1.0.0')} />
          </Form.Item>
          <Form.Item name="signature" label={t('签名（可选）')} extra={t('base64 Ed25519 签名，对象为技能包 sha256 十六进制')}>
            <Input.TextArea rows={2} placeholder={t('带配置了签名信任根时，上传必须携带有效签名')} />
          </Form.Item>
          <Form.Item label={t('技能包文件（tar.gz，含 SKILL.md）')} required>
            <Upload.Dragger
              beforeUpload={() => false}
              maxCount={1}
              accept=".tar.gz,.tgz,.tar"
              fileList={fileList}
              onChange={({ fileList: next }) => setFileList(next)}
            >
              <p className="ant-upload-drag-icon"><InboxOutlined /></p>
              <p className="ant-upload-text">{t('点击或拖拽技能包到此处')}</p>
              <p className="ant-upload-hint">{t('tar.gz 包，须含 SKILL.md（YAML frontmatter + markdown 正文）')}</p>
            </Upload.Dragger>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
