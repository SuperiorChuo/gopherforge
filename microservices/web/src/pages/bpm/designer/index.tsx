import { Fragment, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Dropdown,
  Input,
  InputNumber,
  Popconfirm,
  Radio,
  Segmented,
  Select,
  Skeleton,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd'
import {
  ArrowDownOutlined,
  ArrowLeftOutlined,
  ArrowUpOutlined,
  AuditOutlined,
  CaretDownOutlined,
  DeleteOutlined,
  ForkOutlined,
  MailOutlined,
  PlusOutlined,
  SaveOutlined,
  SendOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { getUserList } from '@/api/system/user'
import { getRoleList } from '@/api/system/role'
import type { SystemRole, SystemUser } from '@/types'
import {
  BPM_ASSIGNEE_TYPE_META,
  BPM_CONDITION_OP_META,
  BPM_DEFINITION_STATUS_META,
  BPM_DEPT_LEADER_BASE_META,
  BPM_EMPTY_FALLBACK_META,
  BPM_FORM_FIELD_LABELS,
  BPM_MULTI_MODE_META,
  BPM_ON_REJECT_META,
  chainToFlow,
  createConditionNode,
  createDefaultFlowSchema,
  draftToExpr,
  exprSummary,
  exprToDraft,
  findNodeById,
  flowToChain,
  genNodeId,
  getDefinition,
  publishDefinition,
  updateDefinition,
  updateNodeById,
  validateApprovalNode,
  validateCcNode,
  validateChain,
  validateConditionNode,
  type AnyNode,
  type ApprovalNode,
  type AssigneeRule,
  type BpmDefinition,
  type CcNode,
  type ConditionLeafOp,
  type ConditionNode,
  type DesignerBranch,
  type StartNode,
} from '@/api/bpm'
import StatusPill from '@/components/StatusPill'

const { Text } = Typography

// ---------------------------------------------------------------------
// 视觉常量：纵向卡片流（仿钉钉简版），纯 div + border 连线，不引入画布库。
// M3：条件分支渲染为横向分叉的分支列（每列 = 分支卡片 + 子卡片流），
// 列尾自动汇合回主流。
// ---------------------------------------------------------------------

const CARD_WIDTH = 340

const HEADER_GRADIENTS: Record<string, string> = {
  start: 'linear-gradient(135deg, #38bdf8, #0284c7)',
  approval: 'linear-gradient(135deg, #fb923c, #ea580c)',
  cc: 'linear-gradient(135deg, #34d399, #059669)',
  condition: 'linear-gradient(135deg, #a78bfa, #7c3aed)',
}

const connectorLineStyle: CSSProperties = {
  width: 2,
  height: 18,
  background: 'rgba(128, 128, 128, 0.35)',
}

function cardStyle(selected: boolean, invalid: boolean): CSSProperties {
  return {
    width: CARD_WIDTH,
    borderRadius: 10,
    overflow: 'hidden',
    cursor: 'pointer',
    background: 'var(--ant-color-bg-container, #fff)',
    boxShadow: selected
      ? '0 0 0 2px #1677ff, 0 4px 12px rgba(22, 119, 255, 0.2)'
      : invalid
        ? '0 0 0 2px #ff4d4f, 0 2px 8px rgba(255, 77, 79, 0.15)'
        : '0 1px 4px rgba(0, 0, 0, 0.1)',
  }
}

// ---------------------------------------------------------------------
// 选中态：节点 或 条件分支（决定右侧抽屉展示哪个配置面板）
// ---------------------------------------------------------------------

type Selection =
  | { kind: 'node'; id: string }
  | { kind: 'branch'; condId: string; branchId: string }

// ---------------------------------------------------------------------
// 卡片间连接器：竖线 + 「+」按钮 + 箭头
// ---------------------------------------------------------------------

function Connector({ onAdd }: { onAdd?: (type: 'approval' | 'cc' | 'condition') => void }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      <div style={connectorLineStyle} />
      {onAdd && (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'approval', icon: <AuditOutlined />, label: '添加审批人' },
              { key: 'cc', icon: <MailOutlined />, label: '添加抄送人' },
              { key: 'condition', icon: <ForkOutlined />, label: '添加条件分支' },
            ],
            onClick: ({ key }) => onAdd(key as 'approval' | 'cc' | 'condition'),
          }}
        >
          <Button shape="circle" size="small" icon={<PlusOutlined />} />
        </Dropdown>
      )}
      <div style={connectorLineStyle} />
      <CaretDownOutlined style={{ color: 'rgba(128,128,128,0.5)', marginTop: -6, fontSize: 14 }} />
    </div>
  )
}

// ---------------------------------------------------------------------
// 单张节点卡片
// ---------------------------------------------------------------------

interface NodeCardProps {
  node: AnyNode
  selected: boolean
  invalidText: string
  summary: ReactNode
  readOnly: boolean
  canUp: boolean
  canDown: boolean
  onClick: () => void
  onMoveUp: () => void
  onMoveDown: () => void
  onRemove: () => void
}

function NodeCard({
  node,
  selected,
  invalidText,
  summary,
  readOnly,
  canUp,
  canDown,
  onClick,
  onMoveUp,
  onMoveDown,
  onRemove,
}: NodeCardProps) {
  const icon =
    node.type === 'start' ? (
      <UserOutlined />
    ) : node.type === 'cc' ? (
      <MailOutlined />
    ) : node.type === 'condition' ? (
      <ForkOutlined />
    ) : (
      <AuditOutlined />
    )
  const removeTitle =
    node.type === 'cc'
      ? '删除该抄送节点？'
      : node.type === 'condition'
        ? '删除该条件分支（含分支内全部节点）？'
        : '删除该审批节点？'
  return (
    <div style={cardStyle(selected, !!invalidText)} onClick={onClick}>
      <div
        style={{
          background: HEADER_GRADIENTS[node.type] ?? HEADER_GRADIENTS.approval,
          color: '#fff',
          padding: '6px 12px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          minHeight: 34,
        }}
      >
        <Space size={6}>
          {icon}
          <span style={{ fontWeight: 600 }}>{node.name || '未命名节点'}</span>
        </Space>
        {!readOnly && node.type !== 'start' && (
          <Space size={0} onClick={(e) => e.stopPropagation()}>
            <Button
              type="text"
              size="small"
              style={{ color: '#fff' }}
              icon={<ArrowUpOutlined />}
              disabled={!canUp}
              onClick={onMoveUp}
            />
            <Button
              type="text"
              size="small"
              style={{ color: '#fff' }}
              icon={<ArrowDownOutlined />}
              disabled={!canDown}
              onClick={onMoveDown}
            />
            <Popconfirm title={removeTitle} onConfirm={onRemove}>
              <Button type="text" size="small" style={{ color: '#fff' }} icon={<DeleteOutlined />} />
            </Popconfirm>
          </Space>
        )}
      </div>
      <div style={{ padding: '10px 12px', fontSize: 13, lineHeight: 1.6 }}>
        {invalidText ? <Text type="danger">{invalidText}</Text> : summary}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------
// 分支卡片（分支列头）：名称 + 优先级 + 条件摘要
// ---------------------------------------------------------------------

function BranchCard({
  branch,
  index,
  selected,
  invalidText,
  readOnly,
  canRemove,
  onClick,
  onRemove,
}: {
  branch: DesignerBranch
  index: number
  selected: boolean
  invalidText: string
  readOnly: boolean
  canRemove: boolean
  onClick: () => void
  onRemove: () => void
}) {
  const isDefault = !branch.expr
  return (
    <div style={{ ...cardStyle(selected, !!invalidText), width: CARD_WIDTH - 40 }} onClick={onClick}>
      <div
        style={{
          padding: '6px 12px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          minHeight: 32,
          borderBottom: '1px solid rgba(128,128,128,0.15)',
        }}
      >
        <Space size={6}>
          <Tag color={isDefault ? 'default' : 'purple'} style={{ marginInlineEnd: 0 }}>
            {isDefault ? '默认' : `优先级 ${index + 1}`}
          </Tag>
          <span style={{ fontWeight: 600, fontSize: 13 }}>{branch.name || '未命名分支'}</span>
        </Space>
        {!readOnly && canRemove && (
          <span onClick={(e) => e.stopPropagation()}>
            <Popconfirm title="删除该分支（含分支内全部节点）？" onConfirm={onRemove}>
              <Button type="text" size="small" icon={<DeleteOutlined />} />
            </Popconfirm>
          </span>
        )}
      </div>
      <div style={{ padding: '8px 12px', fontSize: 12, lineHeight: 1.6 }}>
        {invalidText ? (
          <Text type="danger">{invalidText}</Text>
        ) : (
          <Text type="secondary">{exprSummary(branch.expr)}</Text>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------
// 递归卡片流：一条链（主链 / 分支子链）的节点渲染与结构编辑
// ---------------------------------------------------------------------

interface ChainViewProps {
  chain: AnyNode[]
  onChange: (next: AnyNode[]) => void
  readOnly: boolean
  /** 分支子链为 true：链头渲染添加连接器、节点可移到 0 号位 */
  isBranch?: boolean
  formFields: string[]
  selection: Selection | null
  onSelect: (sel: Selection) => void
  nodeSummary: (node: AnyNode) => ReactNode
}

function newNodeOf(type: 'approval' | 'cc' | 'condition'): AnyNode {
  if (type === 'approval') {
    return {
      id: genNodeId(),
      name: '审批节点',
      type: 'approval',
      assignee: { type: 'users', userIds: [] },
      multiMode: 'OR',
      onReject: 'reject',
      next: null,
    } satisfies ApprovalNode
  }
  if (type === 'cc') {
    return {
      id: genNodeId(),
      name: '抄送',
      type: 'cc',
      targets: { type: 'users', userIds: [] },
      next: null,
    } satisfies CcNode
  }
  return createConditionNode()
}

function ChainView({
  chain,
  onChange,
  readOnly,
  isBranch,
  formFields,
  selection,
  onSelect,
  nodeSummary,
}: ChainViewProps) {
  const minIndex = isBranch ? 0 : 1 // 主链 0 号位是 start，不可移不可删

  const insertAt = (index: number, type: 'approval' | 'cc' | 'condition') => {
    const node = newNodeOf(type)
    const next = [...chain]
    next.splice(index, 0, node)
    onChange(next)
    onSelect({ kind: 'node', id: node.id })
  }

  const removeAt = (index: number) => {
    onChange(chain.filter((_, i) => i !== index))
  }

  const moveAt = (index: number, dir: -1 | 1) => {
    const target = index + dir
    if (index < minIndex || target < minIndex || target >= chain.length) return
    const next = [...chain]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }

  const invalidTextOf = (node: AnyNode): string => {
    if (readOnly) return ''
    if (node.type === 'approval') return validateApprovalNode(node, formFields)
    if (node.type === 'cc') return validateCcNode(node)
    if (node.type === 'condition') {
      // 卡片只标节点级问题；分支级问题标在分支卡片上
      if (!node.name?.trim()) return '节点名称不能为空'
      return ''
    }
    return ''
  }

  const branchInvalidText = (cond: ConditionNode, b: DesignerBranch): string => {
    if (readOnly) return ''
    const err = validateConditionNode(cond, formFields)
    // 粗粒度归属：默认分支缺失/位置问题标到默认分支，条件行问题标到对应分支
    if (err && err.includes(`「${b.name}」`)) return err
    if (err && !b.expr && (err.includes('默认') || err.includes('分支'))) return err
    return ''
  }

  const updateBranches = (condId: string, branches: DesignerBranch[]) => {
    onChange(
      chain.map((n) =>
        n.id === condId && n.type === 'condition' ? ({ ...n, branches } as AnyNode) : n,
      ),
    )
  }

  const renderCondition = (cond: ConditionNode) => {
    const branches = (cond.branches ?? []) as DesignerBranch[]
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'stretch',
          gap: 16,
          padding: '4px 8px',
          maxWidth: 'min(96vw, 1400px)',
          overflowX: 'auto',
        }}
      >
        {branches.map((b, bi) => (
          <div
            key={b.id}
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              minWidth: CARD_WIDTH - 16,
              padding: '12px 10px 16px',
              borderRadius: 12,
              background: 'rgba(128, 128, 128, 0.05)',
              border: '1px dashed rgba(128, 128, 128, 0.28)',
            }}
          >
            <BranchCard
              branch={b}
              index={bi}
              selected={
                selection?.kind === 'branch' &&
                selection.condId === cond.id &&
                selection.branchId === b.id
              }
              invalidText={branchInvalidText(cond, b)}
              readOnly={readOnly}
              canRemove={!!b.expr && branches.length > 2}
              onClick={() => onSelect({ kind: 'branch', condId: cond.id, branchId: b.id })}
              onRemove={() => updateBranches(cond.id, branches.filter((x) => x.id !== b.id))}
            />
            <ChainView
              chain={b.chain ?? []}
              onChange={(sub) =>
                updateBranches(
                  cond.id,
                  branches.map((x) => (x.id === b.id ? { ...x, chain: sub } : x)),
                )
              }
              readOnly={readOnly}
              isBranch
              formFields={formFields}
              selection={selection}
              onSelect={onSelect}
              nodeSummary={nodeSummary}
            />
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4 }}>
              ↓ 汇合
            </Text>
          </div>
        ))}
        {!readOnly && (
          <div style={{ display: 'flex', alignItems: 'flex-start', paddingTop: 12 }}>
            <Button
              icon={<PlusOutlined />}
              size="small"
              onClick={() => {
                const nb: DesignerBranch = {
                  id: genNodeId(),
                  name: `条件 ${branches.length}`,
                  expr: { op: 'and', items: [] },
                  next: null,
                  chain: [],
                }
                // 插到默认分支之前（默认恒在末尾）
                const next = [...branches]
                next.splice(Math.max(branches.length - 1, 0), 0, nb)
                updateBranches(cond.id, next)
                onSelect({ kind: 'branch', condId: cond.id, branchId: nb.id })
              }}
            >
              添加分支
            </Button>
          </div>
        )}
      </div>
    )
  }

  return (
    <>
      {isBranch && (
        <Connector onAdd={readOnly ? undefined : (type) => insertAt(0, type)} />
      )}
      {chain.map((node, index) => (
        <Fragment key={node.id}>
          <NodeCard
            node={node}
            selected={selection?.kind === 'node' && selection.id === node.id}
            invalidText={invalidTextOf(node)}
            summary={nodeSummary(node)}
            readOnly={readOnly}
            canUp={index > minIndex}
            canDown={index >= minIndex && index < chain.length - 1}
            onClick={() => onSelect({ kind: 'node', id: node.id })}
            onMoveUp={() => moveAt(index, -1)}
            onMoveDown={() => moveAt(index, 1)}
            onRemove={() => removeAt(index)}
          />
          {node.type === 'condition' && renderCondition(node)}
          <Connector onAdd={readOnly ? undefined : (type) => insertAt(index + 1, type)} />
        </Fragment>
      ))}
      {isBranch && chain.length === 0 && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          空分支：直通汇合点
        </Text>
      )}
    </>
  )
}

// ---------------------------------------------------------------------
// 设计器主体（作为流程定义页的子组件使用，不占用独立路由）
// ---------------------------------------------------------------------

interface FlowDesignerProps {
  definitionId: number
  /** 非 draft 版本只读查看 */
  readOnly?: boolean
  onBack: () => void
}

export default function FlowDesigner({ definitionId, readOnly = false, onBack }: FlowDesignerProps) {
  const [def, setDef] = useState<BpmDefinition | null>(null)
  const [chain, setChain] = useState<AnyNode[]>([])
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
    return (id: number) => map.get(id) || `用户 #${id}`
  }, [users])

  const roleNameOf = useMemo(() => {
    const map = new Map<number, string>()
    roles.forEach((r) => map.set(r.id, r.name))
    return (id: number) => map.get(id) || `角色 #${id}`
  }, [roles])

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
      return names.slice(0, 3).join('、') + (names.length > 3 ? ` 等 ${names.length} 人` : '')
    }
    if (rule.type === 'roles') {
      const names = (rule.roleIds ?? []).map(roleNameOf)
      return names.slice(0, 3).join('、') + (names.length > 3 ? ` 等 ${names.length} 个角色` : '')
    }
    if (rule.type === 'dept_leader') {
      return (rule.deptLeaderBase ?? 'initiator') === 'form_field'
        ? `按表单字段「${rule.deptFormField || '未填'}」取部门主管`
        : '发起人所在部门的主管'
    }
    if (rule.type === 'self_select') return '发起时由发起人指定'
    return ''
  }

  const assigneeSummary = (node: ApprovalNode): ReactNode => {
    const rule = node.assignee
    return (
      <Space direction="vertical" size={2}>
        <span>
          <Text type="secondary">{BPM_ASSIGNEE_TYPE_META[rule?.type] ?? '未配置'}：</Text>
          {ruleWhoText(rule) || '-'}
        </span>
        <Space size={6} wrap>
          <Tag>
            {node.multiMode === 'AND' ? '会签' : node.multiMode === 'SEQ' ? '依次' : '或签'}
          </Tag>
          {node.timeoutHours ? <Tag color="gold">{node.timeoutHours}h 超时提醒</Tag> : null}
          {node.onReject === 'back_to_start' ? <Tag color="orange">拒绝退回发起人</Tag> : null}
          {node.allowBackPrev ? <Tag color="cyan">可退回上一节点</Tag> : null}
        </Space>
      </Space>
    )
  }

  const nodeSummary = (node: AnyNode): ReactNode => {
    if (node.type === 'start') {
      const fields = (node as StartNode).formFields ?? []
      return (
        <span>
          <Text type="secondary">表单字段：</Text>
          {fields.length ? fields.map((f) => <Tag key={f} className="cell-mono">{f}</Tag>) : '无'}
        </span>
      )
    }
    if (node.type === 'approval') return assigneeSummary(node)
    if (node.type === 'cc') {
      const rule = node.targets
      return (
        <span>
          <Text type="secondary">抄送给（{BPM_ASSIGNEE_TYPE_META[rule?.type] ?? '未配置'}）：</Text>
          {ruleWhoText(rule) || '-'}
        </span>
      )
    }
    if (node.type === 'condition') {
      const names = (node.branches ?? []).map((b) => b.name || '未命名')
      return (
        <span>
          <Text type="secondary">排他分支（从上到下取第一个命中）：</Text>
          {names.join(' / ')}
        </span>
      )
    }
    return <Text type="secondary">未知节点类型，保存时原样保留</Text>
  }

  const startFormFields = chain[0]?.type === 'start' ? ((chain[0] as StartNode).formFields ?? []) : []

  // ---- 保存 / 发布 ----

  const save = async (quiet = false): Promise<boolean> => {
    let nodeTree
    try {
      nodeTree = chainToFlow(chain, def?.node_tree?.version ?? 1)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '节点树结构异常')
      return false
    }
    setSaving(true)
    try {
      await updateDefinition(definitionId, { name: defName.trim() || def?.name, node_tree: nodeTree })
      if (!quiet) message.success('草稿已保存')
      return true
    } catch {
      // 后端/网络错误已由拦截器统一提示
      return false
    } finally {
      setSaving(false)
    }
  }

  const publish = async () => {
    const errors = validateChain(chain)
    if (errors.length) {
      setPublishErrors(errors)
      message.warning('存在未完成的节点配置，请先修正')
      return
    }
    setPublishErrors([])
    setPublishing(true)
    try {
      if (!(await save(true))) return
      await publishDefinition(definitionId)
      message.success('已发布，该版本立即生效')
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
      ? `分支配置：${selectedBranch?.name || '未命名'}`
      : selectedNode
        ? `节点配置：${selectedNode.name || '未命名'}`
        : '配置'

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
            返回列表
          </Button>
          {readOnly ? (
            <Text strong style={{ fontSize: 16 }}>{defName}</Text>
          ) : (
            <Input
              value={defName}
              onChange={(e) => setDefName(e.target.value)}
              style={{ width: 220 }}
              maxLength={128}
              placeholder="流程名称"
            />
          )}
          {def && <Tag className="cell-mono">{def.key}</Tag>}
          {def && <Tag>v{def.version}</Tag>}
          {statusMeta && <StatusPill tone={statusMeta.tone} label={statusMeta.label} />}
        </Space>
        {!readOnly && (
          <Space wrap>
            <Button icon={<SaveOutlined />} loading={saving} onClick={() => void save()}>
              保存草稿
            </Button>
            <Popconfirm
              title="发布该版本？"
              description="发布后立即生效，同一 key 的旧生效版本将自动归档"
              onConfirm={() => void publish()}
            >
              <Button type="primary" icon={<SendOutlined />} loading={publishing}>
                发布
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
          message="发布前请修正以下问题"
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
            color: 'var(--ant-color-text-secondary, #888)',
            fontSize: 13,
          }}
        >
          流程结束
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
            onChange={(patch) => updateBranchMeta(selectedCond.id, selectedBranch.id, patch)}
          />
        ) : selectedNode ? (
          <NodeConfigPanel
            node={selectedNode}
            readOnly={readOnly}
            users={users}
            roles={roles}
            formFields={startFormFields}
            userNameOf={userNameOf}
            roleNameOf={roleNameOf}
            onChange={(patch) => updateNode(selectedNode.id, patch)}
          />
        ) : null}
      </Drawer>
    </Card>
  )
}

// ---------------------------------------------------------------------
// 分支配置面板（右侧抽屉，M3）：分支名 + 条件行编辑器（一层 AND/OR）
// ---------------------------------------------------------------------

const LEAF_OPS = Object.keys(BPM_CONDITION_OP_META) as ConditionLeafOp[]

function BranchConfigPanel({
  branch,
  readOnly,
  formFields,
  onChange,
}: {
  branch: DesignerBranch
  readOnly: boolean
  formFields: string[]
  onChange: (patch: Partial<DesignerBranch>) => void
}) {
  const isDefault = !branch.expr
  const draft = exprToDraft(branch.expr)

  const commitRows = (logic: 'and' | 'or', rows: typeof draft.rows) => {
    onChange({ expr: draftToExpr({ logic, rows }) ?? { op: 'and', items: [] } })
  }

  const fieldOptions = formFields.map((f) => ({
    value: f,
    label: BPM_FORM_FIELD_LABELS[f] ? `${BPM_FORM_FIELD_LABELS[f]}（${f}）` : f,
  }))

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <div>
        <Text type="secondary">分支名称</Text>
        <Input
          style={{ marginTop: 4 }}
          value={branch.name}
          disabled={readOnly}
          maxLength={64}
          placeholder="如：金额 ≥ 10 万"
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </div>
      {isDefault ? (
        <Alert
          type="info"
          showIcon
          message="默认兜底分支"
          description="其余条件分支均未命中时进入此分支；默认分支不可配置条件、不可删除。"
        />
      ) : (
        <>
          <div>
            <Text type="secondary">多条件组合方式</Text>
            <div style={{ marginTop: 6 }}>
              <Segmented
                block
                disabled={readOnly}
                value={draft.logic}
                options={[
                  { label: '且（全部满足）', value: 'and' },
                  { label: '或（任一满足）', value: 'or' },
                ]}
                onChange={(v) => commitRows(v as 'and' | 'or', draft.rows)}
              />
            </div>
          </div>
          <div>
            <Text type="secondary">条件（按发起表单快照求值；金额字段单位为分）</Text>
            <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 6 }}>
              {draft.rows.map((row, ri) => (
                <Space.Compact key={ri} block>
                  <Select
                    style={{ width: '42%' }}
                    disabled={readOnly}
                    placeholder="字段"
                    value={row.field || undefined}
                    options={fieldOptions}
                    onChange={(v: string) => {
                      const rows = draft.rows.map((r, i) => (i === ri ? { ...r, field: v } : r))
                      commitRows(draft.logic, rows)
                    }}
                  />
                  <Select
                    style={{ width: '30%' }}
                    disabled={readOnly}
                    value={row.op}
                    options={LEAF_OPS.map((op) => ({ value: op, label: BPM_CONDITION_OP_META[op] }))}
                    onChange={(v: ConditionLeafOp) => {
                      const rows = draft.rows.map((r, i) => (i === ri ? { ...r, op: v } : r))
                      commitRows(draft.logic, rows)
                    }}
                  />
                  <Input
                    style={{ width: '28%' }}
                    disabled={readOnly}
                    placeholder={row.op === 'in' ? '多值逗号分隔' : '值'}
                    value={row.value}
                    onChange={(e) => {
                      const rows = draft.rows.map((r, i) =>
                        i === ri ? { ...r, value: e.target.value } : r,
                      )
                      commitRows(draft.logic, rows)
                    }}
                  />
                  {!readOnly && (
                    <Button
                      icon={<DeleteOutlined />}
                      onClick={() => commitRows(draft.logic, draft.rows.filter((_, i) => i !== ri))}
                    />
                  )}
                </Space.Compact>
              ))}
              {!readOnly && (
                <Button
                  type="dashed"
                  block
                  icon={<PlusOutlined />}
                  onClick={() =>
                    commitRows(draft.logic, [
                      ...draft.rows,
                      { field: formFields[0] ?? '', op: 'gte', value: '' },
                    ])
                  }
                >
                  添加条件行
                </Button>
              )}
              {!draft.rows.length && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  尚未配置条件；发布前必须至少一行完整条件
                </Text>
              )}
            </Space>
          </div>
        </>
      )}
    </Space>
  )
}

// ---------------------------------------------------------------------
// 节点配置面板（右侧抽屉）
// ---------------------------------------------------------------------

interface NodeConfigPanelProps {
  node: AnyNode
  readOnly: boolean
  users: SystemUser[]
  roles: SystemRole[]
  /** 发起节点声明的表单字段（dept_leader form_field 的字段名候选） */
  formFields: string[]
  userNameOf: (id: number) => string
  roleNameOf: (id: number) => string
  onChange: (patch: Partial<AnyNode>) => void
}

function NodeConfigPanel({
  node,
  readOnly,
  users,
  roles,
  formFields,
  userNameOf,
  roleNameOf,
  onChange,
}: NodeConfigPanelProps) {
  if (node.type === 'start') {
    const fields = (node as StartNode).formFields ?? []
    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="发起节点"
          description="表单由业务方自持，表单字段按业务类型预置，作展示与条件求值声明。"
        />
        <div>
          <Text type="secondary">节点名称</Text>
          <Input
            style={{ marginTop: 4 }}
            value={node.name}
            disabled={readOnly}
            maxLength={128}
            onChange={(e) => onChange({ name: e.target.value })}
          />
        </div>
        <div>
          <Text type="secondary">表单字段声明（只读）</Text>
          <div style={{ marginTop: 6 }}>
            {fields.length ? (
              fields.map((f) => (
                <Tag key={f} className="cell-mono">
                  {f}
                </Tag>
              ))
            ) : (
              <Text type="secondary">无</Text>
            )}
          </div>
        </div>
      </Space>
    )
  }

  if (node.type === 'condition') {
    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="条件分支节点（排他）"
          description="按发起表单快照从上到下求值各分支条件，进入第一个命中的分支；全不命中走默认分支。点击下方分支卡片配置各分支的条件。"
        />
        <div>
          <Text type="secondary">节点名称</Text>
          <Input
            style={{ marginTop: 4 }}
            value={node.name}
            disabled={readOnly}
            maxLength={128}
            placeholder="如：金额分流"
            onChange={(e) => onChange({ name: e.target.value })}
          />
        </div>
      </Space>
    )
  }

  if (node.type === 'cc') {
    const cc = node
    const rule = cc.targets ?? { type: 'users' as const }
    if (readOnly) {
      return (
        <Descriptions
          column={1}
          size="small"
          bordered
          items={[
            { key: 'name', label: '节点名称', children: cc.name || '-' },
            {
              key: 'type',
              label: '抄送对象规则',
              children: BPM_ASSIGNEE_TYPE_META[rule.type] ?? rule.type,
            },
            {
              key: 'who',
              label: '抄送对象',
              children:
                rule.type === 'users'
                  ? (rule.userIds ?? []).map(userNameOf).join('、') || '-'
                  : rule.type === 'roles'
                    ? (rule.roleIds ?? []).map(roleNameOf).join('、') || '-'
                    : '-',
            },
          ]}
        />
      )
    }
    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="抄送节点"
          description="流程流转到此处时给抄送对象落抄送记录并发站内信，不阻塞流程推进。"
        />
        <div>
          <Text type="secondary">节点名称</Text>
          <Input
            style={{ marginTop: 4 }}
            value={cc.name}
            maxLength={128}
            placeholder="如：抄送财务"
            onChange={(e) => onChange({ name: e.target.value })}
          />
        </div>
        <div>
          <Text type="secondary">抄送对象规则（仅支持用户 / 角色）</Text>
          <div style={{ marginTop: 6 }}>
            <Segmented
              block
              value={rule.type === 'roles' ? 'roles' : 'users'}
              options={[
                { label: '指定用户', value: 'users' },
                { label: '指定角色', value: 'roles' },
              ]}
              onChange={(v) => onChange({ targets: { ...rule, type: v as 'users' | 'roles' } })}
            />
          </div>
        </div>
        {rule.type !== 'roles' && (
          <div>
            <Text type="secondary">抄送用户（多选）</Text>
            <Select
              mode="multiple"
              showSearch
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 4 }}
              placeholder="选择抄送用户"
              value={rule.userIds ?? []}
              options={users.map((u) => ({ value: u.id, label: u.nickname || u.username }))}
              onChange={(v: number[]) => onChange({ targets: { ...rule, type: 'users', userIds: v } })}
            />
          </div>
        )}
        {rule.type === 'roles' && (
          <div>
            <Text type="secondary">抄送角色（该角色下所有用户均收到抄送）</Text>
            <Select
              mode="multiple"
              showSearch
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 4 }}
              placeholder="选择抄送角色"
              value={rule.roleIds ?? []}
              options={roles.map((r) => ({ value: r.id, label: r.name }))}
              onChange={(v: number[]) => onChange({ targets: { ...rule, type: 'roles', roleIds: v } })}
            />
          </div>
        )}
      </Space>
    )
  }

  if (node.type !== 'approval') {
    return (
      <Alert
        type="warning"
        showIcon
        message="暂不支持编辑"
        description="简版设计器暂不支持编辑此类型节点；该节点将在保存时原样保留。"
      />
    )
  }

  const approval = node
  const rule = approval.assignee ?? { type: 'users' as const }

  if (readOnly) {
    return (
      <Descriptions
        column={1}
        size="small"
        bordered
        items={[
          { key: 'name', label: '节点名称', children: approval.name || '-' },
          {
            key: 'assignee',
            label: '审批人规则',
            children: BPM_ASSIGNEE_TYPE_META[rule.type] ?? rule.type,
          },
          {
            key: 'who',
            label: '审批人',
            children:
              rule.type === 'users'
                ? (rule.userIds ?? []).map(userNameOf).join('、') || '-'
                : rule.type === 'roles'
                  ? (rule.roleIds ?? []).map(roleNameOf).join('、') || '-'
                  : rule.type === 'dept_leader'
                    ? (rule.deptLeaderBase ?? 'initiator') === 'form_field'
                      ? `表单字段「${rule.deptFormField || '-'}」指定部门的主管`
                      : '发起人所在部门的主管'
                    : '发起时指定',
          },
          ...(rule.type === 'dept_leader'
            ? [
                {
                  key: 'fallback',
                  label: '主管空缺兜底',
                  children: `${BPM_EMPTY_FALLBACK_META[rule.emptyFallback ?? 'suspend'] ?? rule.emptyFallback}${
                    rule.emptyFallback === 'to_users'
                      ? `：${(rule.fallbackUserIds ?? []).map(userNameOf).join('、') || '-'}`
                      : ''
                  }`,
                },
              ]
            : []),
          {
            key: 'mode',
            label: '多人模式',
            children: BPM_MULTI_MODE_META[approval.multiMode] ?? approval.multiMode,
          },
          {
            key: 'onReject',
            label: '拒绝后走向',
            children: BPM_ON_REJECT_META[approval.onReject ?? 'reject'] ?? approval.onReject,
          },
          {
            key: 'backPrev',
            label: '允许退回上一节点',
            children: approval.allowBackPrev ? '允许' : '不允许',
          },
          {
            key: 'timeout',
            label: '超时提醒',
            children: approval.timeoutHours ? `${approval.timeoutHours} 小时` : '不提醒',
          },
        ]}
      />
    )
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <div>
        <Text type="secondary">节点名称</Text>
        <Input
          style={{ marginTop: 4 }}
          value={approval.name}
          maxLength={128}
          placeholder="如：部门经理审批"
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </div>

      <div>
        <Text type="secondary">审批人规则</Text>
        <div style={{ marginTop: 6 }}>
          <Segmented
            block
            value={rule.type}
            options={[
              { label: '指定用户', value: 'users' },
              { label: '指定角色', value: 'roles' },
              { label: '部门主管', value: 'dept_leader' },
              { label: '发起人自选', value: 'self_select' },
            ]}
            onChange={(v) => {
              const type = v as AssigneeRule['type']
              // 切到部门主管时补默认：基准=发起人部门，空结果兜底=挂起（R4 默认从严）
              onChange({
                assignee:
                  type === 'dept_leader'
                    ? {
                        ...rule,
                        type,
                        deptLeaderBase: rule.deptLeaderBase ?? 'initiator',
                        emptyFallback: rule.emptyFallback ?? 'suspend',
                      }
                    : { ...rule, type },
              })
            }}
          />
        </div>
      </div>

      {rule.type === 'users' && (
        <div>
          <Text type="secondary">审批用户（多选；「依次」模式按此顺序逐个审批）</Text>
          <Select
            mode="multiple"
            showSearch
            optionFilterProp="label"
            style={{ width: '100%', marginTop: 4 }}
            placeholder="选择审批用户"
            value={rule.userIds ?? []}
            options={users.map((u) => ({ value: u.id, label: u.nickname || u.username }))}
            onChange={(v: number[]) => onChange({ assignee: { ...rule, userIds: v } })}
          />
        </div>
      )}

      {rule.type === 'roles' && (
        <div>
          <Text type="secondary">审批角色（该角色下所有用户为候选审批人）</Text>
          <Select
            mode="multiple"
            showSearch
            optionFilterProp="label"
            style={{ width: '100%', marginTop: 4 }}
            placeholder="选择审批角色"
            value={rule.roleIds ?? []}
            options={roles.map((r) => ({ value: r.id, label: r.name }))}
            onChange={(v: number[]) => onChange({ assignee: { ...rule, roleIds: v } })}
          />
        </div>
      )}

      {rule.type === 'dept_leader' && (
        <>
          <div>
            <Text type="secondary">主管取自谁的部门（基准）</Text>
            <div style={{ marginTop: 6 }}>
              <Segmented
                block
                value={rule.deptLeaderBase ?? 'initiator'}
                options={[
                  { label: BPM_DEPT_LEADER_BASE_META.initiator, value: 'initiator' },
                  { label: BPM_DEPT_LEADER_BASE_META.form_field, value: 'form_field' },
                ]}
                onChange={(v) =>
                  onChange({
                    assignee: { ...rule, deptLeaderBase: v as 'initiator' | 'form_field' },
                  })
                }
              />
            </div>
          </div>
          {(rule.deptLeaderBase ?? 'initiator') === 'form_field' && (
            <div>
              <Text type="secondary">部门来源字段名（表单快照中存部门 ID 的字段）</Text>
              {formFields.length ? (
                <Select
                  showSearch
                  allowClear
                  style={{ width: '100%', marginTop: 4 }}
                  placeholder="选择表单字段"
                  value={rule.deptFormField || undefined}
                  options={formFields.map((f) => ({ value: f, label: f }))}
                  onChange={(v?: string) =>
                    onChange({ assignee: { ...rule, deptFormField: v || undefined } })
                  }
                />
              ) : (
                <Input
                  style={{ marginTop: 4 }}
                  className="cell-mono"
                  placeholder="如 department_id"
                  maxLength={64}
                  value={rule.deptFormField ?? ''}
                  onChange={(e) =>
                    onChange({ assignee: { ...rule, deptFormField: e.target.value } })
                  }
                />
              )}
            </div>
          )}
          <div>
            <Text type="secondary">找不到主管时的兜底（空结果处理）</Text>
            <div style={{ marginTop: 6 }}>
              <Radio.Group
                value={rule.emptyFallback ?? 'suspend'}
                onChange={(e) => onChange({ assignee: { ...rule, emptyFallback: e.target.value } })}
                options={[
                  { label: BPM_EMPTY_FALLBACK_META.auto_pass, value: 'auto_pass' },
                  { label: BPM_EMPTY_FALLBACK_META.to_users, value: 'to_users' },
                  { label: BPM_EMPTY_FALLBACK_META.suspend, value: 'suspend' },
                ]}
              />
            </div>
          </div>
          {rule.emptyFallback === 'to_users' && (
            <div>
              <Text type="secondary">兜底审批人（多选）</Text>
              <Select
                mode="multiple"
                showSearch
                optionFilterProp="label"
                style={{ width: '100%', marginTop: 4 }}
                placeholder="主管空缺时转交这些人审批"
                value={rule.fallbackUserIds ?? []}
                options={users.map((u) => ({ value: u.id, label: u.nickname || u.username }))}
                onChange={(v: number[]) => onChange({ assignee: { ...rule, fallbackUserIds: v } })}
              />
            </div>
          )}
        </>
      )}

      {rule.type === 'self_select' && (
        <Alert
          type="info"
          showIcon
          message="发起时由发起人指定审批人；仅允许配置在紧邻发起节点的第一个审批节点上。"
        />
      )}

      <div>
        <Text type="secondary">多人审批模式</Text>
        <div style={{ marginTop: 6 }}>
          <Radio.Group
            value={approval.multiMode}
            onChange={(e) => onChange({ multiMode: e.target.value })}
            options={[
              { label: BPM_MULTI_MODE_META.AND, value: 'AND' },
              { label: BPM_MULTI_MODE_META.OR, value: 'OR' },
              { label: BPM_MULTI_MODE_META.SEQ, value: 'SEQ' },
            ]}
          />
        </div>
        {approval.multiMode === 'SEQ' && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
            依次审批：同一时刻只有一个待办，前一人同意后流转给下一人；任一人拒绝即节点拒绝
          </Text>
        )}
      </div>

      <div>
        <Text type="secondary">拒绝后走向</Text>
        <div style={{ marginTop: 6 }}>
          <Segmented
            block
            value={approval.onReject ?? 'reject'}
            options={[
              { label: '结束流程', value: 'reject' },
              { label: '退回发起人', value: 'back_to_start' },
            ]}
            onChange={(v) => onChange({ onReject: v as 'reject' | 'back_to_start' })}
          />
        </div>
        {approval.onReject === 'back_to_start' && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
            拒绝时不结束流程，退回发起人修改后可重新提交
          </Text>
        )}
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <Text type="secondary">允许退回上一节点</Text>
          <Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
            开启后审批人可将任务退回上一审批节点重审（按实际流转路径回溯）
          </Text>
        </div>
        <Switch
          checked={!!approval.allowBackPrev}
          onChange={(checked) => onChange({ allowBackPrev: checked || undefined })}
        />
      </div>

      <div>
        <Text type="secondary">超时提醒（小时，留空不提醒）</Text>
        <InputNumber
          style={{ width: '100%', marginTop: 4 }}
          min={1}
          precision={0}
          placeholder="如 24"
          value={approval.timeoutHours ?? null}
          onChange={(v) => onChange({ timeoutHours: v ?? undefined })}
        />
      </div>
    </Space>
  )
}
