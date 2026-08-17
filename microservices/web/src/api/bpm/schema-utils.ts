import type {
  ApprovalNode,
  AnyNode,
  BpmFormSchema,
  CcNode,
  ConditionBranch,
  ConditionExpr,
  ConditionNode,
  FlowSchema,
  StartNode,
} from './types'
import {
  BPM_BIZ_TYPE_PRESETS,
  BPM_CONDITION_OP_META,
  BPM_FORM_FIELD_LABELS,
} from './metadata'

const FORM_KEY_RE = /^[a-z][a-z0-9_]{0,63}$/

/** 表单 Schema 前端预校验（后端发布时二次权威校验），返回错误清单 */
export function validateFormSchema(schema?: BpmFormSchema | null): string[] {
  const errors: string[] = []
  const fields = schema?.fields ?? []
  if (!fields.length) return errors
  if (fields.length > 50) errors.push('表单字段数超过上限 50')
  const seen = new Set<string>()
  fields.forEach((f, i) => {
    const where = `第 ${i + 1} 个字段`
    if (!FORM_KEY_RE.test(f.key || '')) errors.push(`${where}：key「${f.key || ''}」非法（小写字母开头，仅小写字母/数字/下划线）`)
    if (f.key && seen.has(f.key)) errors.push(`${where}：key「${f.key}」重复`)
    if (f.key) seen.add(f.key)
    if (!f.label?.trim()) errors.push(`${where}：缺少显示名`)
    if ((f.type === 'select' || f.type === 'radio') && !(f.options && f.options.length > 0)) {
      errors.push(`字段「${f.label || f.key}」需要至少一个选项`)
    }
  })
  return errors
}

/** 表单字段中文名映射（schema 优先，兜底 BPM_FORM_FIELD_LABELS / key 本身） */
export function formFieldLabels(schema?: BpmFormSchema | null): Record<string, string> {
  const map: Record<string, string> = { ...BPM_FORM_FIELD_LABELS }
  for (const f of schema?.fields ?? []) {
    if (f.key) map[f.key] = f.label || f.key
  }
  return map
}

// 节点树工具（设计器与只读渲染共用；放在纯 ts 文件避免组件文件混合导出）
// ---------------------------------------------------------------------

/** 生成节点唯一 id */
export const genNodeId = (): string =>
  typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `n-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

/** 新建定义时的默认节点树：仅一个发起节点，formFields 按 biz_type 预置 */
export function createDefaultFlowSchema(bizType?: string): FlowSchema {
  return {
    version: 1,
    start: {
      id: genNodeId(),
      name: '发起人',
      type: 'start',
      formFields: (bizType && BPM_BIZ_TYPE_PRESETS[bizType]?.formFields) || [],
      next: null,
    },
  }
}

/**
 * 设计器编辑态分支：子链以数组承载（branch.chain），保存时经 chainToFlow
 * 重建 next 链并剥离该辅助字段。M3 条件分支设计器专用。
 */
export type DesignerBranch = ConditionBranch & { chain?: AnyNode[] }

/** 把一条链表展平成数组（递归处理条件分支子链，写入 branch.chain）；带环保护 */
export function chainFromHead(head?: AnyNode | null): AnyNode[] {
  const chain: AnyNode[] = []
  let cur: AnyNode | null | undefined = head
  let guard = 0
  while (cur && guard < 200) {
    if (cur.type === 'condition') {
      const branches = (cur.branches ?? []).map((b) => ({
        ...b,
        chain: chainFromHead(b.next),
      }))
      chain.push({ ...cur, branches } as AnyNode)
    } else {
      chain.push(cur)
    }
    cur = cur.next ?? null
    guard += 1
  }
  return chain
}

/** 把链式 node_tree 展平成数组（start 恒为下标 0，分支子链递归展平） */
export function flowToChain(schema?: FlowSchema | null): AnyNode[] {
  return chainFromHead(schema?.start)
}

/** 把数组重新串回一条链表（递归重建分支 next 链，剥离编辑辅助字段 chain） */
function chainToHead(chain: AnyNode[]): AnyNode | null {
  const cloned = chain.map((n) => ({ ...n }))
  for (let i = 0; i < cloned.length; i += 1) {
    const n = cloned[i]
    if (n.type === 'condition') {
      n.branches = (n.branches ?? []).map((b) => {
        const { chain: sub, ...rest } = b as DesignerBranch
        return { ...rest, next: sub ? chainToHead(sub) : (b.next ?? null) }
      })
    }
    n.next = i + 1 < cloned.length ? cloned[i + 1] : null
  }
  return cloned[0] ?? null
}

/** 把数组重新串回链式 node_tree */
export function chainToFlow(chain: AnyNode[], schemaVersion = 1): FlowSchema {
  const start = chainToHead(chain)
  if (!start || start.type !== 'start') {
    throw new Error('节点树必须以发起节点开头')
  }
  return { version: schemaVersion, start: start as StartNode }
}

/** 在编辑态链（含分支子链）里按 id 定位节点 */
export function findNodeById(chain: AnyNode[], id: string): AnyNode | null {
  for (const n of chain) {
    if (n.id === id) return n
    if (n.type === 'condition') {
      for (const b of n.branches ?? []) {
        const hit = findNodeById((b as DesignerBranch).chain ?? [], id)
        if (hit) return hit
      }
    }
  }
  return null
}

/** 在编辑态链（含分支子链）里按 id 打补丁，返回新链（不可变更新） */
export function updateNodeById(chain: AnyNode[], id: string, patch: Partial<AnyNode>): AnyNode[] {
  return chain.map((n) => {
    if (n.id === id) return { ...n, ...patch } as AnyNode
    if (n.type === 'condition') {
      const branches = (n.branches ?? []).map((b) => {
        const db = b as DesignerBranch
        return db.chain ? { ...b, chain: updateNodeById(db.chain, id, patch) } : b
      })
      return { ...n, branches } as AnyNode
    }
    return n
  })
}

/** 新建条件分支节点（一个待配置条件 + 一个默认兜底分支） */
export function createConditionNode(): ConditionNode {
  return {
    id: genNodeId(),
    name: '条件分支',
    type: 'condition',
    branches: [
      { id: genNodeId(), name: '条件 1', expr: { op: 'and', items: [] }, next: null },
      { id: genNodeId(), name: '默认', expr: null, next: null },
    ],
    next: null,
  }
}

// ---------------------------------------------------------------------
// 条件表达式草稿（简版编辑器：一层 AND/OR + 若干比较行；保存即 ConditionExpr）
// ---------------------------------------------------------------------

export type ConditionLeafOp = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne' | 'in'

export interface ConditionRowDraft {
  field: string
  op: ConditionLeafOp
  /** 原样字符串承载；in 用逗号分隔多值，保存时数字化 */
  value: string
}

export interface ConditionDraft {
  logic: 'and' | 'or'
  rows: ConditionRowDraft[]
}

function scalarToDraft(v: unknown): string {
  return Array.isArray(v) ? v.join(',') : String(v ?? '')
}

type ConditionGroup = Extract<ConditionExpr, { items: ConditionExpr[] }>

function isConditionGroup(e: ConditionExpr): e is ConditionGroup {
  return e.op === 'and' || e.op === 'or'
}

/** ConditionExpr → 编辑草稿（嵌套组合超出简版编辑器能力时平铺忽略组合层级） */
export function exprToDraft(expr?: ConditionExpr | null): ConditionDraft {
  if (!expr) return { logic: 'and', rows: [] }
  if (isConditionGroup(expr)) {
    const rows: ConditionRowDraft[] = []
    for (const item of expr.items ?? []) {
      if (isConditionGroup(item)) continue
      rows.push({ field: item.field, op: item.op, value: scalarToDraft(item.value) })
    }
    return { logic: expr.op, rows }
  }
  return { logic: 'and', rows: [{ field: expr.field, op: expr.op, value: scalarToDraft(expr.value) }] }
}

function parseScalar(s: string): string | number {
  const t = s.trim()
  return t !== '' && Number.isFinite(Number(t)) ? Number(t) : t
}

/** 编辑草稿 → ConditionExpr；全部行为空返回 null（发布校验会拦非默认分支） */
export function draftToExpr(d: ConditionDraft): ConditionExpr | null {
  const rows = d.rows.filter((r) => r.field && r.op && r.value.trim() !== '')
  if (!rows.length) return null
  const leaves: ConditionExpr[] = rows.map((r) => ({
    op: r.op,
    field: r.field,
    value:
      r.op === 'in'
        ? r.value
            .split(/[,，、]/)
            .map((s) => s.trim())
            .filter(Boolean)
            .map(parseScalar)
        : parseScalar(r.value),
  }))
  return leaves.length === 1 ? leaves[0] : { op: d.logic, items: leaves }
}

/** 分支条件的摘要文案（分支卡片展示用） */
export function exprSummary(expr?: ConditionExpr | null): string {
  if (!expr) return '其余情况进入此分支'
  const d = exprToDraft(expr)
  if (!d.rows.length) return '未配置条件'
  const parts = d.rows.map(
    (r) =>
      `${BPM_FORM_FIELD_LABELS[r.field] ?? r.field} ${BPM_CONDITION_OP_META[r.op] ?? r.op} ${r.value}`,
  )
  return parts.join(d.logic === 'and' ? ' 且 ' : ' 或 ')
}

/**
 * 单个审批节点的配置校验，返回错误文案（空串=通过）；供卡片内联标红与发布前整树校验共用。
 * formFields：发起节点声明的表单字段（用于 dept_leader form_field 的字段名合法性校验）。
 */
export function validateApprovalNode(node: ApprovalNode, formFields?: string[]): string {
  if (!node.name?.trim()) return '节点名称不能为空'
  const rule = node.assignee
  if (!rule?.type) return '未配置审批人'
  if (rule.type === 'users' && !(rule.userIds && rule.userIds.length > 0)) return '未选择审批用户'
  if (rule.type === 'roles' && !(rule.roleIds && rule.roleIds.length > 0)) return '未选择审批角色'
  if (rule.type === 'dept_leader') {
    const base = rule.deptLeaderBase ?? 'initiator'
    if (base === 'form_field') {
      const field = rule.deptFormField?.trim()
      if (!field) return '部门主管规则：请填写部门来源的表单字段名'
      if (formFields?.length && !formFields.includes(field)) {
        return `部门主管规则：字段「${field}」不在发起表单字段声明中`
      }
    }
    if (
      rule.emptyFallback === 'to_users' &&
      !(rule.fallbackUserIds && rule.fallbackUserIds.length > 0)
    ) {
      return '部门主管规则：请选择空结果兜底的指定审批人'
    }
  }
  if (!node.multiMode) return '未选择多人审批模式'
  return ''
}

/** 条件分支节点配置校验（M3）：分支数、默认分支唯一且在末尾、条件行完整 */
export function validateConditionNode(node: ConditionNode, formFields?: string[]): string {
  if (!node.name?.trim()) return '节点名称不能为空'
  const branches = node.branches ?? []
  if (branches.length < 2) return '条件分支至少需要 2 个分支'
  const defaults = branches.filter((b) => !b.expr)
  if (defaults.length !== 1) return '必须有且仅有一个默认（兜底）分支'
  if (branches[branches.length - 1].expr) return '默认分支必须位于最后'
  for (const b of branches) {
    if (!b.name?.trim()) return '存在未命名的分支'
    if (!b.expr) continue
    const d = exprToDraft(b.expr)
    if (!d.rows.length) return `分支「${b.name}」未配置条件`
    for (const r of d.rows) {
      if (!r.field) return `分支「${b.name}」存在未选字段的条件行`
      if (!r.value.trim()) return `分支「${b.name}」存在未填值的条件行`
      if (formFields?.length && !formFields.includes(r.field)) {
        return `分支「${b.name}」的字段「${r.field}」不在发起表单字段声明中`
      }
    }
  }
  return ''
}

/** 抄送节点配置校验：抄送对象仅支持 users / roles（M2 约束） */
export function validateCcNode(node: CcNode): string {
  if (!node.name?.trim()) return '节点名称不能为空'
  const rule = node.targets
  if (!rule?.type) return '未配置抄送对象'
  if (rule.type !== 'users' && rule.type !== 'roles') return '抄送对象仅支持指定用户或指定角色'
  if (rule.type === 'users' && !(rule.userIds && rule.userIds.length > 0)) return '未选择抄送用户'
  if (rule.type === 'roles' && !(rule.roleIds && rule.roleIds.length > 0)) return '未选择抄送角色'
  return ''
}

/** 发布前整树校验（前端先挡一道，后端发布时二次校验 §2.2 约束）；递归进分支子链 */
export function validateChain(chain: AnyNode[]): string[] {
  const errors: string[] = []
  if (!chain.length || chain[0].type !== 'start') {
    errors.push('缺少发起节点')
    return errors
  }
  const formFields = chain[0].type === 'start' ? ((chain[0] as StartNode).formFields ?? []) : []
  let approvals = 0

  const walkChain = (nodes: AnyNode[], topLevel: boolean) => {
    nodes.forEach((node, idx) => {
      if (node.type === 'approval') {
        approvals += 1
        const err = validateApprovalNode(node, formFields)
        if (err) errors.push(`节点「${node.name || '未命名'}」：${err}`)
        // §2.2 约束 5：self_select 只允许出现在紧邻发起节点之后的审批节点
        if (node.assignee?.type === 'self_select' && !(topLevel && idx === 1)) {
          errors.push(
            `节点「${node.name || '未命名'}」：发起人自选只允许配置在紧邻发起节点的第一个审批节点上`,
          )
        }
        return
      }
      if (node.type === 'cc') {
        const ccErr = validateCcNode(node)
        if (ccErr) errors.push(`节点「${node.name || '未命名'}」：${ccErr}`)
        return
      }
      if (node.type === 'condition') {
        const err = validateConditionNode(node, formFields)
        if (err) errors.push(`节点「${node.name || '未命名'}」：${err}`)
        for (const b of node.branches ?? []) {
          walkChain((b as DesignerBranch).chain ?? [], false)
        }
      }
    })
  }
  walkChain(chain, true)
  if (!approvals) errors.push('至少需要一个审批节点')
  return errors
}

/** 收集整树 node_id → 节点名映射（含条件分支子链），时间线渲染用 */
export function collectNodeNames(schema?: FlowSchema | null): Record<string, string> {
  const map: Record<string, string> = {}
  const walk = (node?: AnyNode | null, guard = 0) => {
    if (!node || guard > 200) return
    map[node.id] = node.name
    if (node.type === 'condition') {
      node.branches?.forEach((b) => walk(b.next, guard + 1))
    }
    walk(node.next ?? null, guard + 1)
  }
  walk(schema?.start)
  return map
}

// ---------------------------------------------------------------------
