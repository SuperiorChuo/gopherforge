import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

// =====================================================================
// 轻量审批流引擎（BPM）前端契约
// 严格对齐 docs/design/bpm-approval-flow.md：
//   - 节点树 JSON Schema 照 §2.2（camelCase，前端所见即所存，无转换层）
//   - 实体字段照 §2.3 DDL（snake_case）
//   - 端点照 §4 的 M1 集合
// 后端 bpm-service 并行开发中，形态以本文件 + 设计文档为对齐基准。
// =====================================================================

// ---------------------------------------------------------------------
// 节点树 JSON Schema（§2.2，逐字对齐）
// ---------------------------------------------------------------------

/** 顶层：一条流程定义的节点树 */
export interface FlowSchema {
  /** schema 结构版本，便于以后演进 */
  version: number
  /** 唯一发起节点，链的头 */
  start: StartNode
}

/** 所有节点共享字段 */
export interface BaseNode {
  /** 节点唯一 id（前端生成 uuid），流转日志据此定位 */
  id: string
  /** 节点显示名，如“部门经理审批” */
  name: string
  type: 'start' | 'approval' | 'cc' | 'condition'
  /** 下一个节点；null/缺省表示到达结束 */
  next?: AnyNode | null
}

/** 发起节点 */
export interface StartNode extends BaseNode {
  type: 'start'
  /** M1 表单由业务方自持；这里仅声明发起时需带的字段 key（用于条件求值/展示） */
  formFields?: string[]
}

/** 审批节点 */
export interface ApprovalNode extends BaseNode {
  type: 'approval'
  /** 审批人规则 */
  assignee: AssigneeRule
  /** 会签 | 或签 | 依次 */
  multiMode: 'AND' | 'OR' | 'SEQ'
  /** 拒绝时的走向：结束流程（reject）还是退回发起人（back_to_start） */
  onReject: 'reject' | 'back_to_start'
  /** 超时提醒阈值（小时），空=不提醒 */
  timeoutHours?: number
  /** 依次(SEQ)时是否允许当前人退回上一审批人 */
  allowBackPrev?: boolean
}

/** 抄送节点 */
export interface CcNode extends BaseNode {
  type: 'cc'
  /** 抄送对象规则（复用审批人规则解析） */
  targets: AssigneeRule
}

/** 条件分支节点（排他，M1 唯一网关；设计器 M1 不产出，仅作类型兼容） */
export interface ConditionNode extends BaseNode {
  type: 'condition'
  /** 从上到下取第一个命中；最后一个应为 default */
  branches: ConditionBranch[]
}

export interface ConditionBranch {
  id: string
  /** 如 “金额 >= 10万” */
  name: string
  /** null 表示 default 兜底分支 */
  expr: ConditionExpr | null
  /** 命中后进入的子链 */
  next: AnyNode | null
}

/** 条件表达式（M1 只做简单比较 + AND/OR 组合，不做脚本） */
export type ConditionExpr =
  | { op: 'and' | 'or'; items: ConditionExpr[] }
  | {
      op: 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne' | 'in'
      /** 取自发起表单快照，如 "amount_cents" */
      field: string
      value: string | number | Array<string | number>
    }

/** 审批人规则 */
export interface AssigneeRule {
  /** M1 四种；type=self_select 时发起时由发起人指定 */
  type: 'users' | 'roles' | 'dept_leader' | 'self_select'
  /** type=users */
  userIds?: number[]
  /** type=roles */
  roleIds?: number[]
  /** type=dept_leader：以谁的部门为基准取主管 */
  deptLeaderBase?: 'initiator' | 'form_field'
  /** deptLeaderBase=form_field 时的字段名 */
  deptFormField?: string
  /** 找不到候选人时的兜底：自动通过 / 转指定人 / 挂起等管理员处理 */
  emptyFallback?: 'auto_pass' | 'to_users' | 'suspend'
  /** emptyFallback=to_users 时 */
  fallbackUserIds?: number[]
}

export type AnyNode = StartNode | ApprovalNode | CcNode | ConditionNode

// ---------------------------------------------------------------------
// 实体类型（§2.3 DDL，snake_case）
// ---------------------------------------------------------------------

export type BpmDefinitionStatus = 'draft' | 'active' | 'suspended' | 'archived'

export interface BpmDefinition {
  id: number
  tenant_id?: number
  /** 逻辑标识，如 expense_approval */
  key: string
  name: string
  version: number
  status: BpmDefinitionStatus | string
  /** 列表接口可能不带（节点树较大），编辑前以详情接口为准 */
  node_tree?: FlowSchema
  form_schema?: unknown
  /** 业务类型，如 demo_expense（业务方自定义） */
  biz_type?: string
  remark?: string
  created_by?: number
  created_at: string
  updated_at?: string
  /** 列表按 key 聚合（最新版本平铺）时附带：当前生效版本号（无 active 版本时为空） */
  active_version?: number
  /** 列表附带：当前生效版本对应的定义行 id */
  active_id?: number
}

export type BpmInstanceStatus = 'running' | 'approved' | 'rejected' | 'canceled'

export interface BpmInstance {
  id: number
  tenant_id?: number
  definition_id: number
  definition_key: string
  title: string
  biz_type: string
  /** 业务对象 id（字符串承载，通用） */
  biz_id: string
  status: BpmInstanceStatus | string
  /** 当前推进到的节点 id（node_tree 内 id） */
  current_node_id?: string
  /** 若后端顺手返回当前节点名则直接用，否则前端经 diagram 反查 */
  current_node_name?: string
  /** 发起时表单快照（条件求值依据） */
  form_snapshot?: Record<string, unknown>
  variables?: Record<string, unknown>
  initiator_id: number
  initiator_name?: string
  initiator_dept?: number
  finished_at?: string
  created_at: string
  updated_at?: string
}

export type BpmTaskStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'canceled'
  | 'skipped'
  | 'returned'

export interface BpmTask {
  id: number
  tenant_id?: number
  instance_id: number
  node_id: string
  node_name: string
  /** 退回重审时同节点的第几轮 */
  round?: number
  assignee_id: number
  /** 后端可选冗余的处理人姓名；缺省时前端用 useUserNameMap 映射 */
  assignee_name?: string
  /** 转办前的原处理人（空=未转办） */
  origin_assignee?: number
  multi_mode?: 'AND' | 'OR' | 'SEQ'
  seq_order?: number
  status: BpmTaskStatus | string
  /** 审批意见 */
  comment?: string
  timeout_at?: string
  reminded_at?: string
  acted_at?: string
  created_at: string
  updated_at?: string
  // ---- 待办/已办列表附带的实例摘要（§4.3：含实例标题、发起人、节点名、到达时间、timeout_at）----
  instance_title?: string
  instance_status?: BpmInstanceStatus | string
  initiator_id?: number
  initiator_name?: string
  biz_type?: string
  biz_id?: string
}

/** 任务详情：实例摘要 + form_snapshot + 我可用的动作列表（§4.3） */
export interface BpmTaskDetail {
  task: BpmTask
  instance: BpmInstance
  /** 如 ["approve","reject"] */
  actions?: string[]
}

export type BpmLogAction =
  | 'submit'
  | 'approve'
  | 'reject'
  | 'transfer'
  | 'return_start'
  | 'return_prev'
  | 'cancel'
  | 'cc'
  | 'timeout_remind'
  | 'auto_pass'
  | 'finish_approved'
  | 'finish_rejected'

/** 流转日志（bpm_process_log），时间线数据源；操作人姓名由前端用现有用户接口映射（§4.4 M1 约定） */
export interface BpmTimelineItem {
  id: number
  instance_id: number
  /** 系统级动作（发起/撤销/终态）可为空 */
  node_id?: string
  /** 若后端冗余返回节点名则直接用，否则前端经 node_tree 反查 */
  node_name?: string
  task_id?: number
  action: BpmLogAction | string
  /** 0=系统 */
  operator_id: number
  operator_name?: string
  /** 附加信息：意见、转办目标、退回目标等 */
  detail?: Record<string, unknown>
  created_at: string
}

export type BpmNodeRuntimeState = 'done' | 'doing' | 'todo' | 'skipped'

export interface BpmNodeRuntime {
  state: BpmNodeRuntimeState
  /** 该节点的完整任务对象列表（后端确认：assignee_id / status / acted_at / comment 等字段） */
  tasks?: BpmTask[]
}

/** 流转图数据：定义 node_tree + 每个节点的运行时标注（§4.4） */
export interface BpmDiagram {
  node_tree: FlowSchema
  /** node_id → 运行时标注 */
  nodes: Record<string, BpmNodeRuntime>
}

// ---------------------------------------------------------------------
// 展示元数据（中文文案集中处）
// ---------------------------------------------------------------------

export const BPM_DEFINITION_STATUS_META: Record<
  string,
  { label: string; tone: 'success' | 'muted' | 'danger' | 'info' | 'warning' }
> = {
  draft: { label: '草稿', tone: 'info' },
  active: { label: '已发布', tone: 'success' },
  suspended: { label: '已停用', tone: 'warning' },
  archived: { label: '已归档', tone: 'muted' },
}

export const BPM_INSTANCE_STATUS_META: Record<string, { label: string; color: string }> = {
  running: { label: '审批中', color: 'processing' },
  approved: { label: '已通过', color: 'success' },
  rejected: { label: '已拒绝', color: 'error' },
  canceled: { label: '已撤销', color: 'default' },
}

export const BPM_TASK_STATUS_META: Record<
  string,
  { label: string; tone: 'success' | 'muted' | 'danger' | 'info' | 'warning' }
> = {
  pending: { label: '待处理', tone: 'info' },
  approved: { label: '已同意', tone: 'success' },
  rejected: { label: '已拒绝', tone: 'danger' },
  canceled: { label: '已取消', tone: 'muted' },
  skipped: { label: '已跳过', tone: 'muted' },
  returned: { label: '已退回', tone: 'warning' },
}

export const BPM_ACTION_META: Record<string, { label: string; color: string }> = {
  submit: { label: '发起审批', color: 'blue' },
  approve: { label: '同意', color: 'green' },
  reject: { label: '拒绝', color: 'red' },
  transfer: { label: '转办', color: 'blue' },
  return_start: { label: '退回发起人', color: 'orange' },
  return_prev: { label: '退回上一节点', color: 'orange' },
  cancel: { label: '撤销', color: 'gray' },
  cc: { label: '抄送', color: 'blue' },
  timeout_remind: { label: '超时提醒', color: 'orange' },
  auto_pass: { label: '自动通过', color: 'green' },
  finish_approved: { label: '审批通过', color: 'green' },
  finish_rejected: { label: '审批拒绝', color: 'red' },
}

export const BPM_MULTI_MODE_META: Record<string, string> = {
  AND: '会签（全部同意）',
  OR: '或签（一人同意）',
  SEQ: '依次（按顺序逐个）',
}

export const BPM_ASSIGNEE_TYPE_META: Record<string, string> = {
  users: '指定用户',
  roles: '指定角色',
  dept_leader: '部门主管',
  self_select: '发起人自选',
}

/** biz_type 预置（发起节点 formFields 由业务类型预置，只读展示；见 §3.6/§5.1）。
 *  脚手架给一个通用示例，业务方按需扩展自己的 biz_type 与字段。 */
export const BPM_BIZ_TYPE_PRESETS: Record<string, { label: string; formFields: string[] }> = {
  demo_expense: {
    label: '示例：报销审批',
    formFields: ['amount_cents', 'reason', 'applicant'],
  },
}

// ---------------------------------------------------------------------
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

/** 把链式 node_tree 展平成数组（start 恒为下标 0）；带环保护 */
export function flowToChain(schema?: FlowSchema | null): AnyNode[] {
  const chain: AnyNode[] = []
  let cur: AnyNode | null | undefined = schema?.start
  let guard = 0
  while (cur && guard < 200) {
    chain.push(cur)
    cur = cur.next ?? null
    guard += 1
  }
  return chain
}

/** 把数组重新串回链式 node_tree；节点浅拷贝后重建 next 指针 */
export function chainToFlow(chain: AnyNode[], schemaVersion = 1): FlowSchema {
  const cloned = chain.map((n) => ({ ...n }))
  for (let i = 0; i < cloned.length; i += 1) {
    cloned[i].next = i + 1 < cloned.length ? cloned[i + 1] : null
  }
  const start = cloned[0]
  if (!start || start.type !== 'start') {
    throw new Error('节点树必须以发起节点开头')
  }
  return { version: schemaVersion, start: start as StartNode }
}

/** 单个审批节点的配置校验，返回错误文案（空串=通过）；供卡片内联标红与发布前整树校验共用 */
export function validateApprovalNode(node: ApprovalNode): string {
  if (!node.name?.trim()) return '节点名称不能为空'
  const rule = node.assignee
  if (!rule?.type) return '未配置审批人'
  if (rule.type === 'users' && !(rule.userIds && rule.userIds.length > 0)) return '未选择审批用户'
  if (rule.type === 'roles' && !(rule.roleIds && rule.roleIds.length > 0)) return '未选择审批角色'
  if (!node.multiMode) return '未选择多人审批模式'
  // 与后端发布校验对齐（M1 拒绝）：SEQ 依次、back_to_start 退回发起人均属 M2
  if (node.multiMode === 'SEQ') return 'M1 暂不支持「依次」模式'
  if (node.onReject === 'back_to_start') return 'M1 拒绝后走向仅支持「流程结束」'
  return ''
}

/** 发布前整树校验（前端先挡一道，后端发布时二次校验 §2.2 约束） */
export function validateChain(chain: AnyNode[]): string[] {
  const errors: string[] = []
  if (!chain.length || chain[0].type !== 'start') {
    errors.push('缺少发起节点')
    return errors
  }
  const approvals = chain.filter((n): n is ApprovalNode => n.type === 'approval')
  if (!approvals.length) errors.push('至少需要一个审批节点')
  chain.forEach((node, idx) => {
    // 与后端 M1 发布校验对齐：condition 节点、cc 的 self_select 直接拒绝
    if (node.type === 'condition') {
      errors.push(`节点「${node.name || '未命名'}」：M1 暂不支持条件分支节点`)
      return
    }
    if (node.type === 'cc' && node.targets?.type === 'self_select') {
      errors.push(`节点「${node.name || '未命名'}」：抄送对象不支持发起人自选`)
      return
    }
    if (node.type !== 'approval') return
    const err = validateApprovalNode(node)
    if (err) errors.push(`第 ${idx} 个节点「${node.name || '未命名'}」：${err}`)
    // §2.2 约束 5：self_select 只允许出现在紧邻发起节点之后的审批节点
    if (node.assignee?.type === 'self_select' && idx !== 1) {
      errors.push(`节点「${node.name || '未命名'}」：发起人自选只允许配置在紧邻发起节点的第一个审批节点上`)
    }
  })
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
// API 封装 —— §4.1 管理端（流程定义，权限：bpm:definition:*）
// ---------------------------------------------------------------------

export type BpmDefinitionListParams = PageRequest & {
  keyword?: string
  biz_type?: string
}

export interface BpmDefinitionCreateData {
  key: string
  name: string
  biz_type?: string
  node_tree: FlowSchema
  remark?: string
}

export type BpmDefinitionUpdateData = Partial<Omit<BpmDefinitionCreateData, 'key'>>

/** 定义列表（按 key 聚合显示最新版本，含 active 版本号） */
export const listDefinitions = (params: BpmDefinitionListParams) =>
  request.get<unknown, PageResponse<BpmDefinition>>('/api/v1/bpm/definitions', { params })

/** 新建定义 → version=1, status=draft */
export const createDefinition = (data: BpmDefinitionCreateData) =>
  request.post<unknown, BpmDefinition>('/api/v1/bpm/definitions', data)

/** 定义详情（含 node_tree） */
export const getDefinition = (id: number) =>
  request.get<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}`)

/** 修改 draft 版本（active 版本不可改，需另存新版本） */
export const updateDefinition = (id: number, data: BpmDefinitionUpdateData) =>
  request.put<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}`, data)

/** 发布：后端 Schema 校验 → 该版本 active，同 key 旧 active → archived */
export const publishDefinition = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/publish`)

/** 以某版本为底复制出新 draft 版本（version=max+1） */
export const newDefinitionVersion = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/new-version`)

/** 停用（不再允许新发起，在途实例不受影响） */
export const suspendDefinition = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/suspend`)

/** 按 key 取当前 active 版本（发起端/业务端用） */
export const getActiveDefinitionByKey = (key: string) =>
  request.get<unknown, BpmDefinition>(`/api/v1/bpm/definitions/keys/${encodeURIComponent(key)}/active`)

// ---------------------------------------------------------------------
// API 封装 —— §4.2 发起端 + §4.4 实例端
// 注：POST /api/v1/bpm/instances 的业务发起走业务后端 internal 变体
//（表单快照由业务后端权威生成），前端不封装裸发起接口。
// ---------------------------------------------------------------------

export type BpmInstanceListParams = PageRequest & { status?: string }

/** 我发起的 */
export const listMyInstances = (params: BpmInstanceListParams) =>
  request.get<unknown, PageResponse<BpmInstance>>('/api/v1/bpm/instances/my', { params })

/** 撤销（仅发起人，且首个审批节点尚无人审过 §3.3） */
export const cancelInstance = (id: number) =>
  request.post<unknown, void>(`/api/v1/bpm/instances/${id}/cancel`)

/** 实例详情：基本信息 + form_snapshot + 当前节点 */
export const getInstance = (id: number, silent = false) =>
  request.get<unknown, BpmInstance>(`/api/v1/bpm/instances/${id}`, { silent })

/** 时间线：流转日志按时间正序；后端返回 {list}（兼容裸数组防御） */
export const getInstanceTimeline = (id: number, silent = false) =>
  request
    .get<unknown, BpmTimelineItem[] | { list?: BpmTimelineItem[] } | null>(
      `/api/v1/bpm/instances/${id}/timeline`,
      { silent },
    )
    .then((d) => (Array.isArray(d) ? d : (d?.list ?? [])))

/** 流转图数据：node_tree + 节点运行时标注 */
export const getInstanceDiagram = (id: number, silent = false) =>
  request.get<unknown, BpmDiagram>(`/api/v1/bpm/instances/${id}/diagram`, { silent })

// 注：bpm 的 by-biz 反查仅有 internal 变体（/api/v1/bpm/internal/instances/by-biz，
// X-Internal-Token，前端不可用）。业务侧嵌入审批进度时，反查走各业务后端
// 自建的代理端点（脚手架不含具体业务，故此处不提供反查封装）。

// ---------------------------------------------------------------------
// API 封装 —— §4.3 任务端（审批人视角；M1 动作：同意/拒绝）
// ---------------------------------------------------------------------

export type BpmTaskListParams = PageRequest & { keyword?: string }

/** 我的待办（silent 供业务页探测 BPM 可用性复用） */
export const listTodoTasks = (params: BpmTaskListParams, silent = false) =>
  request.get<unknown, PageResponse<BpmTask>>('/api/v1/bpm/tasks/todo', { params, silent })

/** 我的已办 */
export const listDoneTasks = (params: BpmTaskListParams) =>
  request.get<unknown, PageResponse<BpmTask>>('/api/v1/bpm/tasks/done', { params })

/** 任务详情（含实例摘要 + form_snapshot + 我可用的动作列表） */
export const getTask = (id: number) =>
  request.get<unknown, BpmTaskDetail>(`/api/v1/bpm/tasks/${id}`)

/** 审批动作的返回体（后端确认形态） */
export interface BpmTaskActionResult {
  task_id: number
  instance_id: number
  /** 动作落库后的实例状态（据此可即时提示“流程已通过/已拒绝”） */
  instance_status: BpmInstanceStatus | string
}

/** 同意（意见可选） */
export const approveTask = (id: number, comment?: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/approve`, {
    comment: comment || undefined,
  })

/** 拒绝（意见必填，前端强制 §3.3） */
export const rejectTask = (id: number, comment: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/reject`, { comment })
