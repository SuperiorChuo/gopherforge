
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
  /** 超时阈值（小时），空=不启用超时 */
  timeoutHours?: number
  /** 到期动作：remind（缺省提醒）| auto_pass | auto_reject（收官项 Q3） */
  timeoutAction?: 'remind' | 'auto_pass' | 'auto_reject'
  /** 依次(SEQ)时是否允许当前人退回上一审批人 */
  allowBackPrev?: boolean
  /** 表单字段权限（M1 仅 hidden）：该节点任务详情按此过滤快照（表单构建器） */
  fieldPerms?: Record<string, 'hidden'>
}

// ---------------------------------------------------------------------
// 表单构建器（M1，流程表单模式；见 docs/design/bpm-form-builder.md）
// ---------------------------------------------------------------------

export type BpmFormFieldType =
  | 'input'
  | 'textarea'
  | 'number'
  | 'amount'
  | 'select'
  | 'radio'
  | 'date'
  | 'switch'

export interface BpmFormField {
  /** ^[a-z][a-z0-9_]{0,63}$，表单内唯一 */
  key: string
  label: string
  type: BpmFormFieldType
  required?: boolean
  placeholder?: string
  /** select / radio 必填 */
  options?: string[]
  min?: number
  max?: number
  rows?: number
}

export interface BpmFormSchema {
  version: number
  fields: BpmFormField[]
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
  /** 表单 Schema（流程表单模式；空=业务表单，业务后端 internal 发起） */
  form_schema?: BpmFormSchema | null
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

export type BpmInstanceStatus = 'running' | 'approved' | 'rejected' | 'canceled' | 'suspended'

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
  /** 详情接口附带：冻结版本的表单 Schema（流程表单模式，动态渲染用） */
  form_schema?: BpmFormSchema | null
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
  /** 加签发起人（空=非加签产生的任务） */
  add_sign_by?: number
  /** 委派人（非空=委派办理中，当前 assignee 为受托人） */
  delegated_by?: number
  /** 最近一次委派办结的受托人 */
  delegate_resolved_by?: number
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
  /** 如 ["approve","reject","transfer","return_start","return_prev","resubmit"] */
  actions?: string[]
}

/** 抄送记录（bpm_cc_record，M2；GET /cc/my 行结构按契约） */
export interface BpmCcRecord {
  id: number
  instance_id: number
  instance_title: string
  node_name: string
  initiator_id: number
  /** 空=未读 */
  read_at?: string
  created_at: string
}

export type BpmLogAction =
  | 'submit'
  | 'approve'
  | 'reject'
  | 'transfer'
  | 'return_start'
  | 'return_prev'
  | 'cancel'
  | 'resubmit'
  | 'cc'
  | 'timeout_remind'
  | 'auto_pass'
  | 'suspend'
  | 'branch'
  | 'terminate'
  | 'finish_approved'
  | 'finish_rejected'
  | 'add_sign'
  | 'delegate'
  | 'delegate_resolve'

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
