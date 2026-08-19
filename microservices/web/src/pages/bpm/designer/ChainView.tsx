import { Fragment, type CSSProperties, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Dropdown, Popconfirm, Space, Tag, Typography } from 'antd'
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  AuditOutlined,
  CaretDownOutlined,
  DeleteOutlined,
  ForkOutlined,
  MailOutlined,
  PlusOutlined,
  UserOutlined,
} from '@ant-design/icons'
import {
  createConditionNode,
  exprSummary,
  genNodeId,
  validateApprovalNode,
  validateCcNode,
  validateConditionNode,
  type AnyNode,
  type ApprovalNode,
  type CcNode,
  type ConditionNode,
  type DesignerBranch,
} from '@/api/bpm'

const { Text } = Typography

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
    background: 'var(--card-bg, rgba(18, 20, 34, 0.6))',
    boxShadow: selected
      ? '0 0 0 2px var(--c-primary), 0 4px 12px rgba(99, 102, 241, 0.2)'
      : invalid
        ? '0 0 0 2px var(--c-error), 0 2px 8px rgba(248, 113, 113, 0.15)'
        : '0 1px 4px rgba(0, 0, 0, 0.1)',
  }
}

// ---------------------------------------------------------------------
// 选中态：节点 或 条件分支（决定右侧抽屉展示哪个配置面板）
// ---------------------------------------------------------------------

export type Selection =
  | { kind: 'node'; id: string }
  | { kind: 'branch'; condId: string; branchId: string }

// ---------------------------------------------------------------------
// 卡片间连接器：竖线 + 「+」按钮 + 箭头
// ---------------------------------------------------------------------

function Connector({ onAdd }: { onAdd?: (type: 'approval' | 'cc' | 'condition') => void }) {
  const { t } = useTranslation()
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      <div style={connectorLineStyle} />
      {onAdd && (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'approval', icon: <AuditOutlined />, label: t('添加审批人') },
              { key: 'cc', icon: <MailOutlined />, label: t('添加抄送人') },
              { key: 'condition', icon: <ForkOutlined />, label: t('添加条件分支') },
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
  const { t } = useTranslation()
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
      ? t('删除该抄送节点？')
      : node.type === 'condition'
        ? t('删除该条件分支（含分支内全部节点）？')
        : t('删除该审批节点？')
  return (
    <div style={cardStyle(selected, !!invalidText)} onClick={onClick}>
      <div
        style={{
          background: HEADER_GRADIENTS[node.type] ?? HEADER_GRADIENTS.approval,
          color: 'var(--text-on-accent)',
          padding: '6px 12px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          minHeight: 34,
        }}
      >
        <Space size={6}>
          {icon}
          <span style={{ fontWeight: 600 }}>{node.name || t('未命名节点')}</span>
        </Space>
        {!readOnly && node.type !== 'start' && (
          <Space size={0} onClick={(e) => e.stopPropagation()}>
            <Button
              type="text"
              size="small"
              style={{ color: 'var(--text-on-accent)' }}
              icon={<ArrowUpOutlined />}
              disabled={!canUp}
              onClick={onMoveUp}
            />
            <Button
              type="text"
              size="small"
              style={{ color: 'var(--text-on-accent)' }}
              icon={<ArrowDownOutlined />}
              disabled={!canDown}
              onClick={onMoveDown}
            />
            <Popconfirm title={removeTitle} onConfirm={onRemove}>
              <Button type="text" size="small" style={{ color: 'var(--text-on-accent)' }} icon={<DeleteOutlined />} />
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
  const { t } = useTranslation()
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
            {isDefault ? t('默认') : t('优先级 {{n}}', { n: index + 1 })}
          </Tag>
          <span style={{ fontWeight: 600, fontSize: 13 }}>{branch.name || t('未命名分支')}</span>
        </Space>
        {!readOnly && canRemove && (
          <span onClick={(e) => e.stopPropagation()}>
            <Popconfirm title={t('删除该分支（含分支内全部节点）？')} onConfirm={onRemove}>
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

export default function ChainView({
  chain,
  onChange,
  readOnly,
  isBranch,
  formFields,
  selection,
  onSelect,
  nodeSummary,
}: ChainViewProps) {
  const { t } = useTranslation()
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
      if (!node.name?.trim()) return t('节点名称不能为空')
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
              {t('↓ 汇合')}
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
              {t('添加分支')}
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
          {t('空分支：直通汇合点')}
        </Text>
      )}
    </>
  )
}
