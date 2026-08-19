import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Drawer, Input, Popconfirm, Skeleton, Space, Tag, Typography } from 'antd'
import { ArrowLeftOutlined, FormOutlined, SaveOutlined, SendOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { getUserList } from '@/api/system/user'
import { getRoleList } from '@/api/system/role'
import type { SystemRole, SystemUser } from '@/types'
import {
  BPM_ASSIGNEE_TYPE_META,
  BPM_DEFINITION_STATUS_META,
  chainToFlow,
  createDefaultFlowSchema,
  findNodeById,
  flowToChain,
  formFieldLabels,
  getDefinition,
  publishDefinition,
  updateDefinition,
  updateNodeById,
  validateChain,
  validateFormSchema,
  type AnyNode,
  type ApprovalNode,
  type AssigneeRule,
  type BpmDefinition,
  type BpmFormSchema,
  type ConditionNode,
  type DesignerBranch,
  type StartNode,
} from '@/api/bpm'
import StatusPill from '@/components/common/StatusPill'
import { useLocale } from '@/i18n/LocaleContext'
import BranchConfigPanel from './BranchConfigPanel'
import ChainView, { type Selection } from './ChainView'
import FormSchemaEditor from './FormSchemaEditor'
import NodeConfigPanel from './NodeConfigPanel'

const { Text } = Typography

interface FlowDesignerProps {
  definitionId: number
  /** 非 draft 版本只读查看 */
  readOnly?: boolean
  onBack: () => void
}

export default function FlowDesigner({ definitionId, readOnly = false, onBack }: FlowDesignerProps) {
  const { t } = useTranslation()
  const { locale } = useLocale()
  const [def, setDef] = useState<BpmDefinition | null>(null)
  const [chain, setChain] = useState<AnyNode[]>([])
  // 表单构建器（M1）：流程表单 Schema，随定义保存；发布时前后端双重校验
  const [formSchema, setFormSchema] = useState<BpmFormSchema | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [defName, setDefName] = useState('')
  const [selection, setSelection] = useState<Selection | null>(null)
  const [users, setUsers] = useState<SystemUser[]>([])
  const [roles, setRoles] = useState<SystemRole[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [publishErrors, setPublishErrors] = useState<string[]>([])

  useEffect(() => {
    let alive = true
    setLoading(true)
    getDefinition(definitionId)
      .then((d) => {
        if (!alive) return
        setDef(d)
        setDefName(d.name)
        setFormSchema(d.form_schema ?? null)
        setChain(flowToChain(d.node_tree ?? createDefaultFlowSchema(d.biz_type)))
      })
      .catch(() => {
        // 加载失败提示已由拦截器弹出，返回列表
        if (alive) onBack()
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [definitionId])

  useEffect(() => {
    getUserList({ page: 1, page_size: 500 })
      .then((r) => setUsers(r.list ?? []))
      .catch(() => {})
    getRoleList({ page: 1, page_size: 200 })
      .then((r) => setRoles(r.list ?? []))
      .catch(() => {})
  }, [])

  const userNameOf = useMemo(() => {
    const map = new Map<number, string>()
    users.forEach((u) => map.set(u.id, u.nickname || u.username))
    return (id: number) => map.get(id) || t('用户 #{{n}}', { n: id })
  }, [users, locale])

  const roleNameOf = useMemo(() => {
    const map = new Map<number, string>()
    roles.forEach((r) => map.set(r.id, r.name))
    return (id: number) => map.get(id) || t('角色 #{{n}}', { n: id })
  }, [roles, locale])

  // ---- 选中对象解析（节点在整树上按 id 定位） ----

  const selectedNode = selection?.kind === 'node' ? findNodeById(chain, selection.id) : null
  const selectedCond =
    selection?.kind === 'branch' ? (findNodeById(chain, selection.condId) as ConditionNode | null) : null
  const selectedBranch =
    selection?.kind === 'branch' && selectedCond?.type === 'condition'
      ? ((selectedCond.branches ?? []).find((b) => b.id === selection.branchId) as
          | DesignerBranch
          | undefined) ?? null
      : null

  const updateNode = (id: string, patch: Partial<AnyNode>) => {
    setChain((prev) => updateNodeById(prev, id, patch))
  }

  const updateBranchMeta = (condId: string, branchId: string, patch: Partial<DesignerBranch>) => {
    setChain((prev) => {
      const cond = findNodeById(prev, condId)
      if (!cond || cond.type !== 'condition') return prev
      const branches = (cond.branches ?? []).map((b) => (b.id === branchId ? { ...b, ...patch } : b))
      return updateNodeById(prev, condId, { branches } as Partial<AnyNode>)
    })
  }

  // ---- 摘要 ----

  const ruleWhoText = (rule?: AssigneeRule): string => {
    if (!rule?.type) return ''
    if (rule.type === 'users') {
      const names = (rule.userIds ?? []).map(userNameOf)
      return names.slice(0, 3).join('、') + (names.length > 3 ? t(' 等 {{n}} 人', { n: names.length }) : '')
    }
    if (rule.type === 'roles') {
      const names = (rule.roleIds ?? []).map(roleNameOf)
      return names.slice(0, 3).join('、') + (names.length > 3 ? t(' 等 {{n}} 个角色', { n: names.length }) : '')
    }
    if (rule.type === 'dept_leader') {
      return (rule.deptLeaderBase ?? 'initiator') === 'form_field'
        ? t('按表单字段「{{field}}」取部门主管', { field: rule.deptFormField || t('未填') })
        : t('发起人所在部门的主管')
    }
    if (rule.type === 'self_select') return t('发起时由发起人指定')
    return ''
  }

  const assigneeSummary = (node: ApprovalNode): ReactNode => {
    const rule = node.assignee
    return (
      <Space direction="vertical" size={2}>
        <span>
          <Text type="secondary">{t(BPM_ASSIGNEE_TYPE_META[rule?.type] ?? '未配置')}：</Text>
          {ruleWhoText(rule) || '-'}
        </span>
        <Space size={6} wrap>
          <Tag>
            {node.multiMode === 'AND' ? t('会签') : node.multiMode === 'SEQ' ? t('依次') : t('或签')}
          </Tag>
          {node.timeoutHours ? (
            <Tag color={node.timeoutAction === 'auto_pass' ? 'green' : node.timeoutAction === 'auto_reject' ? 'red' : 'gold'}>
              {node.timeoutHours}h{' '}
              {node.timeoutAction === 'auto_pass' ? t('超时自动通过') : node.timeoutAction === 'auto_reject' ? t('超时自动拒绝') : t('超时提醒')}
            </Tag>
          ) : null}
          {node.onReject === 'back_to_start' ? <Tag color="orange">{t('拒绝退回发起人')}</Tag> : null}
          {node.allowBackPrev ? <Tag color="cyan">{t('可退回上一节点')}</Tag> : null}
        </Space>
      </Space>
    )
  }

  const nodeSummary = (node: AnyNode): ReactNode => {
    if (node.type === 'start') {
      const fields = (node as StartNode).formFields ?? []
      return (
        <span>
          <Text type="secondary">{t('表单字段：')}</Text>
          {fields.length ? fields.map((f) => <Tag key={f} className="cell-mono">{f}</Tag>) : t('无')}
        </span>
      )
    }
    if (node.type === 'approval') return assigneeSummary(node)
    if (node.type === 'cc') {
      const rule = node.targets
      return (
        <span>
          <Text type="secondary">{t('抄送给（{{type}}）：', { type: t(BPM_ASSIGNEE_TYPE_META[rule?.type] ?? '未配置') })}</Text>
          {ruleWhoText(rule) || '-'}
        </span>
      )
    }
    if (node.type === 'condition') {
      const names = (node.branches ?? []).map((b) => b.name || t('未命名'))
      return (
        <span>
          <Text type="secondary">{t('排他分支（从上到下取第一个命中）：')}</Text>
          {names.join(' / ')}
        </span>
      )
    }
    return <Text type="secondary">{t('未知节点类型，保存时原样保留')}</Text>
  }

  const startFormFields = formSchema?.fields?.length
    ? formSchema.fields.map((f) => f.key)
    : chain[0]?.type === 'start'
      ? ((chain[0] as StartNode).formFields ?? [])
      : []
  const fieldLabels = formFieldLabels(formSchema)

  // ---- 保存 / 发布 ----

  const save = async (quiet = false): Promise<boolean> => {
    let nodeTree
    try {
      nodeTree = chainToFlow(chain, def?.node_tree?.version ?? 1)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('节点树结构异常'))
      return false
    }
    setSaving(true)
    try {
      await updateDefinition(definitionId, {
        name: defName.trim() || def?.name,
        node_tree: nodeTree,
        form_schema: formSchema,
      })
      if (!quiet) message.success(t('草稿已保存'))
      return true
    } catch {
      // 后端/网络错误已由拦截器统一提示
      return false
    } finally {
      setSaving(false)
    }
  }

  const publish = async () => {
    const errors = [...validateFormSchema(formSchema), ...validateChain(chain)]
    if (errors.length) {
      setPublishErrors(errors)
      message.warning(t('存在未完成的节点配置，请先修正'))
      return
    }
    setPublishErrors([])
    setPublishing(true)
    try {
      if (!(await save(true))) return
      await publishDefinition(definitionId)
      message.success(t('已发布，该版本立即生效'))
      onBack()
    } catch {
      // 后端 Schema 校验失败等，拦截器已提示
    } finally {
      setPublishing(false)
    }
  }

  // ---- 渲染 ----

  if (loading) {
    return (
      <Card className="list-main-card" bordered={false}>
        <Skeleton active paragraph={{ rows: 8 }} />
      </Card>
    )
  }

  const statusMeta = def ? BPM_DEFINITION_STATUS_META[def.status] : undefined
  const drawerTitle =
    selection?.kind === 'branch'
      ? t('分支配置：{{name}}', { name: selectedBranch?.name || t('未命名') })
      : selectedNode
        ? t('节点配置：{{name}}', { name: selectedNode.name || t('未命名') })
        : t('配置')

  return (
    <Card className="list-main-card" bordered={false}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: 12,
          marginBottom: 8,
        }}
      >
        <Space size={10} wrap>
          <Button icon={<ArrowLeftOutlined />} onClick={onBack}>
            {t('返回列表')}
          </Button>
          {readOnly ? (
            <Text strong style={{ fontSize: 16 }}>{defName}</Text>
          ) : (
            <Input
              value={defName}
              onChange={(e) => setDefName(e.target.value)}
              style={{ width: 220 }}
              maxLength={128}
              placeholder={t('流程名称')}
            />
          )}
          {def && <Tag className="cell-mono">{def.key}</Tag>}
          {def && <Tag>v{def.version}</Tag>}
          {statusMeta && <StatusPill tone={statusMeta.tone} label={statusMeta.label} />}
        </Space>
        {!readOnly && (
          <Space wrap>
            <Button icon={<FormOutlined />} onClick={() => setFormOpen(true)}>
              {t('表单设计')}{formSchema?.fields?.length ? `（${formSchema.fields.length}）` : ''}
            </Button>
            <Button icon={<SaveOutlined />} loading={saving} onClick={() => void save()}>
              {t('保存草稿')}
            </Button>
            <Popconfirm
              title={t('发布该版本？')}
              description={t('发布后立即生效，同一 key 的旧生效版本将自动归档')}
              onConfirm={() => void publish()}
            >
              <Button type="primary" icon={<SendOutlined />} loading={publishing}>
                {t('发布')}
              </Button>
            </Popconfirm>
          </Space>
        )}
      </div>

      {publishErrors.length > 0 && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
          message={t('发布前请修正以下问题')}
          description={
            <ul style={{ margin: 0, paddingInlineStart: 18 }}>
              {publishErrors.map((err, i) => (
                <li key={i}>{err}</li>
              ))}
            </ul>
          }
        />
      )}

      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          padding: '24px 0 48px',
          background:
            'radial-gradient(rgba(128, 128, 128, 0.12) 1px, transparent 1px) 0 0 / 16px 16px',
          borderRadius: 12,
        }}
      >
        <ChainView
          chain={chain}
          onChange={setChain}
          readOnly={readOnly}
          formFields={startFormFields}
          selection={selection}
          onSelect={setSelection}
          nodeSummary={nodeSummary}
        />
        <div
          style={{
            width: 120,
            textAlign: 'center',
            padding: '8px 0',
            borderRadius: 20,
            background: 'rgba(128, 128, 128, 0.12)',
            color: 'var(--text-secondary)',
            fontSize: 13,
          }}
        >
          {t('流程结束')}
        </div>
      </div>

      <Drawer
        title={drawerTitle}
        open={!!(selectedNode || selectedBranch)}
        onClose={() => setSelection(null)}
        width={440}
        destroyOnHidden
      >
        {selection?.kind === 'branch' && selectedCond && selectedBranch ? (
          <BranchConfigPanel
            branch={selectedBranch}
            readOnly={readOnly}
            formFields={startFormFields}
            fieldLabels={fieldLabels}
            onChange={(patch) => updateBranchMeta(selectedCond.id, selectedBranch.id, patch)}
          />
        ) : selectedNode ? (
          <NodeConfigPanel
            node={selectedNode}
            readOnly={readOnly}
            users={users}
            roles={roles}
            formFields={startFormFields}
            schemaFields={formSchema?.fields}
            userNameOf={userNameOf}
            roleNameOf={roleNameOf}
            onChange={(patch) => updateNode(selectedNode.id, patch)}
          />
        ) : null}
      </Drawer>

      <Drawer
        title={t('表单设计（流程表单模式）')}
        open={formOpen}
        onClose={() => setFormOpen(false)}
        width={520}
        destroyOnHidden
      >
        <FormSchemaEditor
          value={formSchema}
          readOnly={readOnly}
          onChange={setFormSchema}
        />
      </Drawer>
    </Card>
  )
}
