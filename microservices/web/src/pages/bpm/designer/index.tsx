import { Fragment, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  InputNumber,
  Popconfirm,
  Radio,
  Segmented,
  Select,
  Skeleton,
  Space,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import {
  ArrowDownOutlined,
  ArrowLeftOutlined,
  ArrowUpOutlined,
  AuditOutlined,
  CaretDownOutlined,
  DeleteOutlined,
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
  BPM_DEFINITION_STATUS_META,
  BPM_MULTI_MODE_META,
  chainToFlow,
  createDefaultFlowSchema,
  flowToChain,
  genNodeId,
  getDefinition,
  publishDefinition,
  updateDefinition,
  validateApprovalNode,
  validateChain,
  type AnyNode,
  type ApprovalNode,
  type BpmDefinition,
  type StartNode,
} from '@/api/bpm'
import StatusPill from '@/components/StatusPill'

const { Text } = Typography

// ---------------------------------------------------------------------
// 视觉常量：纵向卡片流（仿钉钉简版），纯 div + border 连线，不引入画布库
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
// 卡片间连接器：竖线 + 「+」按钮 + 箭头
// ---------------------------------------------------------------------

function Connector({ onAdd }: { onAdd?: () => void }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      <div style={connectorLineStyle} />
      {onAdd && (
        <Tooltip title="添加审批节点">
          <Button shape="circle" size="small" icon={<PlusOutlined />} onClick={onAdd} />
        </Tooltip>
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
  const icon = node.type === 'start' ? <UserOutlined /> : <AuditOutlined />
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
        {!readOnly && node.type === 'approval' && (
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
            <Popconfirm title="删除该审批节点？" onConfirm={onRemove}>
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
  const [selectedId, setSelectedId] = useState<string | null>(null)
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

  const selected = chain.find((n) => n.id === selectedId) ?? null

  // ---- 链上编辑操作（start 恒在下标 0，不可移不可删）----

  const updateNode = (id: string, patch: Partial<AnyNode>) => {
    setChain((prev) => prev.map((n) => (n.id === id ? ({ ...n, ...patch } as AnyNode) : n)))
  }

  const addApprovalAfter = (index: number) => {
    const node: ApprovalNode = {
      id: genNodeId(),
      name: '审批节点',
      type: 'approval',
      assignee: { type: 'users', userIds: [] },
      multiMode: 'OR',
      onReject: 'reject',
      next: null,
    }
    setChain((prev) => {
      const next = [...prev]
      next.splice(index + 1, 0, node)
      return next
    })
    setSelectedId(node.id)
  }

  const removeNode = (index: number) => {
    setChain((prev) => {
      const removed = prev[index]
      if (removed && removed.id === selectedId) setSelectedId(null)
      return prev.filter((_, i) => i !== index)
    })
  }

  const moveNode = (index: number, dir: -1 | 1) => {
    setChain((prev) => {
      const target = index + dir
      if (index <= 0 || target <= 0 || target >= prev.length) return prev
      const next = [...prev]
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })
  }

  // ---- 摘要与校验 ----

  const assigneeSummary = (node: ApprovalNode): ReactNode => {
    const rule = node.assignee
    let who = ''
    if (rule?.type === 'users') {
      const names = (rule.userIds ?? []).map(userNameOf)
      who = names.slice(0, 3).join('、') + (names.length > 3 ? ` 等 ${names.length} 人` : '')
    } else if (rule?.type === 'roles') {
      const names = (rule.roleIds ?? []).map(roleNameOf)
      who = names.slice(0, 3).join('、') + (names.length > 3 ? ` 等 ${names.length} 个角色` : '')
    } else if (rule?.type === 'self_select') {
      who = '发起时由发起人指定'
    }
    return (
      <Space direction="vertical" size={2}>
        <span>
          <Text type="secondary">{BPM_ASSIGNEE_TYPE_META[rule?.type] ?? '未配置'}：</Text>
          {who || '-'}
        </span>
        <Space size={6} wrap>
          <Tag>{node.multiMode === 'AND' ? '会签' : node.multiMode === 'SEQ' ? '依次' : '或签'}</Tag>
          {node.timeoutHours ? <Tag color="gold">{node.timeoutHours}h 超时提醒</Tag> : null}
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
    return <Text type="secondary">M1 设计器暂不支持编辑此类型节点，保存时原样保留</Text>
  }

  const invalidTextOf = (node: AnyNode): string =>
    !readOnly && node.type === 'approval' ? validateApprovalNode(node) : ''

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
        {chain.map((node, index) => (
          <Fragment key={node.id}>
            <NodeCard
              node={node}
              selected={node.id === selectedId}
              invalidText={invalidTextOf(node)}
              summary={nodeSummary(node)}
              readOnly={readOnly}
              canUp={index > 1}
              canDown={index > 0 && index < chain.length - 1}
              onClick={() => setSelectedId(node.id)}
              onMoveUp={() => moveNode(index, -1)}
              onMoveDown={() => moveNode(index, 1)}
              onRemove={() => removeNode(index)}
            />
            <Connector onAdd={readOnly ? undefined : () => addApprovalAfter(index)} />
          </Fragment>
        ))}
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
        title={selected ? `节点配置：${selected.name || '未命名'}` : '节点配置'}
        open={!!selected}
        onClose={() => setSelectedId(null)}
        width={420}
        destroyOnHidden
      >
        {selected && (
          <NodeConfigPanel
            node={selected}
            readOnly={readOnly}
            users={users}
            roles={roles}
            userNameOf={userNameOf}
            roleNameOf={roleNameOf}
            onChange={(patch) => updateNode(selected.id, patch)}
          />
        )}
      </Drawer>
    </Card>
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
  userNameOf: (id: number) => string
  roleNameOf: (id: number) => string
  onChange: (patch: Partial<AnyNode>) => void
}

function NodeConfigPanel({
  node,
  readOnly,
  users,
  roles,
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
          description="M1 表单由业务方自持，表单字段按业务类型预置，仅作展示与条件求值声明。"
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

  if (node.type !== 'approval') {
    return (
      <Alert
        type="warning"
        showIcon
        message="暂不支持编辑"
        description="M1 简版设计器仅支持发起节点与审批节点；该节点将在保存时原样保留。"
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
                  : '发起时指定',
          },
          {
            key: 'mode',
            label: '多人模式',
            children: BPM_MULTI_MODE_META[approval.multiMode] ?? approval.multiMode,
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
              { label: '发起人自选', value: 'self_select' },
            ]}
            onChange={(v) =>
              onChange({ assignee: { ...rule, type: v as 'users' | 'roles' | 'self_select' } })
            }
          />
        </div>
      </div>

      {rule.type === 'users' && (
        <div>
          <Text type="secondary">审批用户（多选）</Text>
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

      {rule.type === 'self_select' && (
        <Alert
          type="info"
          showIcon
          message="发起时由发起人指定审批人；仅允许配置在紧邻发起节点的第一个审批节点上（M1 约束）。"
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
            ]}
          />
        </div>
      </div>

      {/* M1 拒绝后走向固定为 reject（流程结束）；back_to_start 属 M2，后端发布校验会拒绝，故不提供选项 */}

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
