export const BPM_FORM_FIELD_TYPE_META: Record<string, string> = {
  input: '单行文本',
  textarea: '多行文本',
  number: '数字',
  amount: '金额（元录入，分存储）',
  select: '下拉选择',
  radio: '单选',
  date: '日期',
  switch: '开关',
}

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
  suspended: { label: '已挂起', color: 'warning' },
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
  resubmit: { label: '重新提交', color: 'blue' },
  cc: { label: '抄送', color: 'blue' },
  timeout_remind: { label: '超时提醒', color: 'orange' },
  timeout_pass: { label: '超时自动通过', color: 'green' },
  timeout_reject: { label: '超时自动拒绝', color: 'red' },
  auto_pass: { label: '自动通过', color: 'green' },
  suspend: { label: '实例挂起', color: 'orange' },
  branch: { label: '分支命中', color: 'purple' },
  terminate: { label: '管理员终止', color: 'red' },
  finish_approved: { label: '审批通过', color: 'green' },
  finish_rejected: { label: '审批拒绝', color: 'red' },
  add_sign: { label: '加签', color: 'purple' },
  delegate: { label: '委派', color: 'geekblue' },
  delegate_resolve: { label: '委派办结', color: 'geekblue' },
}

/** 条件表达式叶子操作符文案 */
export const BPM_CONDITION_OP_META: Record<string, string> = {
  gt: '大于',
  gte: '大于等于',
  lt: '小于',
  lte: '小于等于',
  eq: '等于',
  ne: '不等于',
  in: '属于（多值）',
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

export const BPM_DEPT_LEADER_BASE_META: Record<string, string> = {
  initiator: '发起人部门',
  form_field: '表单字段指定部门',
}

export const BPM_EMPTY_FALLBACK_META: Record<string, string> = {
  auto_pass: '自动通过',
  to_users: '转指定人',
  suspend: '挂起待管理员处理',
}

export const BPM_ON_REJECT_META: Record<string, string> = {
  reject: '结束流程',
  back_to_start: '退回发起人',
}

/** 表单快照的已知字段中文名（demo 业务类型字段；时间线与重提编辑器共用） */
export const BPM_FORM_FIELD_LABELS: Record<string, string> = {
  amount_cents: '金额',
  reason: '事由',
  applicant: '申请人',
  title: '标题',
}

/** biz_type 预置（发起节点 formFields 由业务类型预置，只读展示；脚手架内置示例） */
export const BPM_BIZ_TYPE_PRESETS: Record<string, { label: string; formFields: string[] }> = {
  demo_expense: {
    label: '示例：报销审批',
    formFields: ['amount_cents', 'reason', 'applicant'],
  },
}

// ---------------------------------------------------------------------
