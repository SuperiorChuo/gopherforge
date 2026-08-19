import { useTranslation } from 'react-i18next'
import {
  Alert,
  Descriptions,
  Input,
  InputNumber,
  Radio,
  Segmented,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd'
import type { SystemRole, SystemUser } from '@/types'
import {
  BPM_ASSIGNEE_TYPE_META,
  BPM_DEPT_LEADER_BASE_META,
  BPM_EMPTY_FALLBACK_META,
  BPM_MULTI_MODE_META,
  BPM_ON_REJECT_META,
  type AnyNode,
  type AssigneeRule,
  type BpmFormField,
  type StartNode,
} from '@/api/bpm'

const { Text } = Typography

interface NodeConfigPanelProps {
  node: AnyNode
  readOnly: boolean
  users: SystemUser[]
  roles: SystemRole[]
  /** 发起节点声明的表单字段（dept_leader form_field 的字段名候选） */
  formFields: string[]
  /** 表单 Schema 字段（有值时审批节点可配字段可见性） */
  schemaFields?: BpmFormField[]
  userNameOf: (id: number) => string
  roleNameOf: (id: number) => string
  onChange: (patch: Partial<AnyNode>) => void
}

export default function NodeConfigPanel({
  node,
  readOnly,
  users,
  roles,
  formFields,
  schemaFields,
  userNameOf,
  roleNameOf,
  onChange,
}: NodeConfigPanelProps) {
  const { t } = useTranslation()
  if (node.type === 'start') {
    const fields = (node as StartNode).formFields ?? []
    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message={t('发起节点')}
          description={t('表单由业务方自持，表单字段按业务类型预置，作展示与条件求值声明。')}
        />
        <div>
          <Text type="secondary">{t('节点名称')}</Text>
          <Input
            style={{ marginTop: 4 }}
            value={node.name}
            disabled={readOnly}
            maxLength={128}
            onChange={(e) => onChange({ name: e.target.value })}
          />
        </div>
        <div>
          <Text type="secondary">{t('表单字段声明（只读）')}</Text>
          <div style={{ marginTop: 6 }}>
            {fields.length ? (
              fields.map((f) => (
                <Tag key={f} className="cell-mono">
                  {f}
                </Tag>
              ))
            ) : (
              <Text type="secondary">{t('无')}</Text>
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
          message={t('条件分支节点（排他）')}
          description={t('按发起表单快照从上到下求值各分支条件，进入第一个命中的分支；全不命中走默认分支。点击下方分支卡片配置各分支的条件。')}
        />
        <div>
          <Text type="secondary">{t('节点名称')}</Text>
          <Input
            style={{ marginTop: 4 }}
            value={node.name}
            disabled={readOnly}
            maxLength={128}
            placeholder={t('如：金额分流')}
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
            { key: 'name', label: t('节点名称'), children: cc.name || '-' },
            {
              key: 'type',
              label: t('抄送对象规则'),
              children: t(BPM_ASSIGNEE_TYPE_META[rule.type] ?? rule.type),
            },
            {
              key: 'who',
              label: t('抄送对象'),
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
          message={t('抄送节点')}
          description={t('流程流转到此处时给抄送对象落抄送记录并发站内信，不阻塞流程推进。')}
        />
        <div>
          <Text type="secondary">{t('节点名称')}</Text>
          <Input
            style={{ marginTop: 4 }}
            value={cc.name}
            maxLength={128}
            placeholder={t('如：抄送财务')}
            onChange={(e) => onChange({ name: e.target.value })}
          />
        </div>
        <div>
          <Text type="secondary">{t('抄送对象规则（仅支持用户 / 角色）')}</Text>
          <div style={{ marginTop: 6 }}>
            <Segmented
              block
              value={rule.type === 'roles' ? 'roles' : 'users'}
              options={[
                { label: t('指定用户'), value: 'users' },
                { label: t('指定角色'), value: 'roles' },
              ]}
              onChange={(v) => onChange({ targets: { ...rule, type: v as 'users' | 'roles' } })}
            />
          </div>
        </div>
        {rule.type !== 'roles' && (
          <div>
            <Text type="secondary">{t('抄送用户（多选）')}</Text>
            <Select
              mode="multiple"
              showSearch
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 4 }}
              placeholder={t('选择抄送用户')}
              value={rule.userIds ?? []}
              options={users.map((u) => ({ value: u.id, label: u.nickname || u.username }))}
              onChange={(v: number[]) => onChange({ targets: { ...rule, type: 'users', userIds: v } })}
            />
          </div>
        )}
        {rule.type === 'roles' && (
          <div>
            <Text type="secondary">{t('抄送角色（该角色下所有用户均收到抄送）')}</Text>
            <Select
              mode="multiple"
              showSearch
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 4 }}
              placeholder={t('选择抄送角色')}
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
        message={t('暂不支持编辑')}
        description={t('简版设计器暂不支持编辑此类型节点；该节点将在保存时原样保留。')}
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
          { key: 'name', label: t('节点名称'), children: approval.name || '-' },
          {
            key: 'assignee',
            label: t('审批人规则'),
            children: t(BPM_ASSIGNEE_TYPE_META[rule.type] ?? rule.type),
          },
          {
            key: 'who',
            label: t('审批人'),
            children:
              rule.type === 'users'
                ? (rule.userIds ?? []).map(userNameOf).join('、') || '-'
                : rule.type === 'roles'
                  ? (rule.roleIds ?? []).map(roleNameOf).join('、') || '-'
                  : rule.type === 'dept_leader'
                    ? (rule.deptLeaderBase ?? 'initiator') === 'form_field'
                      ? t('表单字段「{{field}}」指定部门的主管', { field: rule.deptFormField || '-' })
                      : t('发起人所在部门的主管')
                    : t('发起时指定'),
          },
          ...(rule.type === 'dept_leader'
            ? [
                {
                  key: 'fallback',
                  label: t('主管空缺兜底'),
                  children: `${t(BPM_EMPTY_FALLBACK_META[rule.emptyFallback ?? 'suspend'] ?? rule.emptyFallback)}${
                    rule.emptyFallback === 'to_users'
                      ? `：${(rule.fallbackUserIds ?? []).map(userNameOf).join('、') || '-'}`
                      : ''
                  }`,
                },
              ]
            : []),
          {
            key: 'mode',
            label: t('多人模式'),
            children: t(BPM_MULTI_MODE_META[approval.multiMode] ?? approval.multiMode),
          },
          {
            key: 'onReject',
            label: t('拒绝后走向'),
            children: t(BPM_ON_REJECT_META[approval.onReject ?? 'reject'] ?? approval.onReject),
          },
          {
            key: 'backPrev',
            label: t('允许退回上一节点'),
            children: approval.allowBackPrev ? t('允许') : t('不允许'),
          },
          {
            key: 'timeout',
            label: t('超时提醒'),
            children: approval.timeoutHours ? t('{{n}} 小时', { n: approval.timeoutHours }) : t('不提醒'),
          },
        ]}
      />
    )
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <div>
        <Text type="secondary">{t('节点名称')}</Text>
        <Input
          style={{ marginTop: 4 }}
          value={approval.name}
          maxLength={128}
          placeholder={t('如：部门经理审批')}
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </div>

      <div>
        <Text type="secondary">{t('审批人规则')}</Text>
        <div style={{ marginTop: 6 }}>
          <Segmented
            block
            value={rule.type}
            options={[
              { label: t('指定用户'), value: 'users' },
              { label: t('指定角色'), value: 'roles' },
              { label: t('部门主管'), value: 'dept_leader' },
              { label: t('发起人自选'), value: 'self_select' },
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
          <Text type="secondary">{t('审批用户（多选；「依次」模式按此顺序逐个审批）')}</Text>
          <Select
            mode="multiple"
            showSearch
            optionFilterProp="label"
            style={{ width: '100%', marginTop: 4 }}
            placeholder={t('选择审批用户')}
            value={rule.userIds ?? []}
            options={users.map((u) => ({ value: u.id, label: u.nickname || u.username }))}
            onChange={(v: number[]) => onChange({ assignee: { ...rule, userIds: v } })}
          />
        </div>
      )}

      {rule.type === 'roles' && (
        <div>
          <Text type="secondary">{t('审批角色（该角色下所有用户为候选审批人）')}</Text>
          <Select
            mode="multiple"
            showSearch
            optionFilterProp="label"
            style={{ width: '100%', marginTop: 4 }}
            placeholder={t('选择审批角色')}
            value={rule.roleIds ?? []}
            options={roles.map((r) => ({ value: r.id, label: r.name }))}
            onChange={(v: number[]) => onChange({ assignee: { ...rule, roleIds: v } })}
          />
        </div>
      )}

      {rule.type === 'dept_leader' && (
        <>
          <div>
            <Text type="secondary">{t('主管取自谁的部门（基准）')}</Text>
            <div style={{ marginTop: 6 }}>
              <Segmented
                block
                value={rule.deptLeaderBase ?? 'initiator'}
                options={[
                  { label: t(BPM_DEPT_LEADER_BASE_META.initiator), value: 'initiator' },
                  { label: t(BPM_DEPT_LEADER_BASE_META.form_field), value: 'form_field' },
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
              <Text type="secondary">{t('部门来源字段名（表单快照中存部门 ID 的字段）')}</Text>
              {formFields.length ? (
                <Select
                  showSearch
                  allowClear
                  style={{ width: '100%', marginTop: 4 }}
                  placeholder={t('选择表单字段')}
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
                  placeholder={t('如 department_id')}
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
            <Text type="secondary">{t('找不到主管时的兜底（空结果处理）')}</Text>
            <div style={{ marginTop: 6 }}>
              <Radio.Group
                value={rule.emptyFallback ?? 'suspend'}
                onChange={(e) => onChange({ assignee: { ...rule, emptyFallback: e.target.value } })}
                options={[
                  { label: t(BPM_EMPTY_FALLBACK_META.auto_pass), value: 'auto_pass' },
                  { label: t(BPM_EMPTY_FALLBACK_META.to_users), value: 'to_users' },
                  { label: t(BPM_EMPTY_FALLBACK_META.suspend), value: 'suspend' },
                ]}
              />
            </div>
          </div>
          {rule.emptyFallback === 'to_users' && (
            <div>
              <Text type="secondary">{t('兜底审批人（多选）')}</Text>
              <Select
                mode="multiple"
                showSearch
                optionFilterProp="label"
                style={{ width: '100%', marginTop: 4 }}
                placeholder={t('主管空缺时转交这些人审批')}
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
          message={t('发起时由发起人指定审批人；仅允许配置在紧邻发起节点的第一个审批节点上。')}
        />
      )}

      <div>
        <Text type="secondary">{t('多人审批模式')}</Text>
        <div style={{ marginTop: 6 }}>
          <Radio.Group
            value={approval.multiMode}
            onChange={(e) => onChange({ multiMode: e.target.value })}
            options={[
              { label: t(BPM_MULTI_MODE_META.AND), value: 'AND' },
              { label: t(BPM_MULTI_MODE_META.OR), value: 'OR' },
              { label: t(BPM_MULTI_MODE_META.SEQ), value: 'SEQ' },
            ]}
          />
        </div>
        {approval.multiMode === 'SEQ' && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
            {t('依次审批：同一时刻只有一个待办，前一人同意后流转给下一人；任一人拒绝即节点拒绝')}
          </Text>
        )}
      </div>

      <div>
        <Text type="secondary">{t('拒绝后走向')}</Text>
        <div style={{ marginTop: 6 }}>
          <Segmented
            block
            value={approval.onReject ?? 'reject'}
            options={[
              { label: t('结束流程'), value: 'reject' },
              { label: t('退回发起人'), value: 'back_to_start' },
            ]}
            onChange={(v) => onChange({ onReject: v as 'reject' | 'back_to_start' })}
          />
        </div>
        {approval.onReject === 'back_to_start' && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
            {t('拒绝时不结束流程，退回发起人修改后可重新提交')}
          </Text>
        )}
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <Text type="secondary">{t('允许退回上一节点')}</Text>
          <Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
            {t('开启后审批人可将任务退回上一审批节点重审（按实际流转路径回溯）')}
          </Text>
        </div>
        <Switch
          checked={!!approval.allowBackPrev}
          onChange={(checked) => onChange({ allowBackPrev: checked || undefined })}
        />
      </div>

      <div>
        <Text type="secondary">{t('超时（小时，留空不启用）')}</Text>
        <InputNumber
          style={{ width: '100%', marginTop: 4 }}
          min={1}
          precision={0}
          placeholder={t('如 24')}
          value={approval.timeoutHours ?? null}
          onChange={(v) =>
            onChange({
              timeoutHours: v ?? undefined,
              // 关闭超时时联动清掉自动动作（发布校验要求二者成对）
              ...(v ? {} : { timeoutAction: undefined }),
            })
          }
        />
        {!!approval.timeoutHours && (
          <div style={{ marginTop: 8 }}>
            <Text type="secondary">{t('到期动作')}</Text>
            <div style={{ marginTop: 4 }}>
              <Segmented
                block
                value={approval.timeoutAction ?? 'remind'}
                options={[
                  { label: t('仅提醒'), value: 'remind' },
                  { label: t('自动通过'), value: 'auto_pass' },
                  { label: t('自动拒绝'), value: 'auto_reject' },
                ]}
                onChange={(v) =>
                  onChange({
                    timeoutAction: v === 'remind' ? undefined : (v as 'auto_pass' | 'auto_reject'),
                  })
                }
              />
            </div>
            {(approval.timeoutAction === 'auto_pass' || approval.timeoutAction === 'auto_reject') && (
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                {t('到期后由系统自动{{action}}该待办（时间线记系统操作）', {
                  action: approval.timeoutAction === 'auto_pass' ? t('通过') : t('拒绝'),
                })}
              </Text>
            )}
          </div>
        )}
      </div>

      {(schemaFields?.length ?? 0) > 0 && (
        <div>
          <Text type="secondary">{t('表单字段可见性（对此节点隐藏的字段不出现在其任务详情）')}</Text>
          <Space direction="vertical" size={6} style={{ width: '100%', marginTop: 6 }}>
            {schemaFields!.map((f) => {
              const hidden = approval.fieldPerms?.[f.key] === 'hidden'
              return (
                <div
                  key={f.key}
                  style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
                >
                  <span>
                    {f.label}{' '}
                    <Text type="secondary" className="cell-mono" style={{ fontSize: 12 }}>
                      {f.key}
                    </Text>
                  </span>
                  <Switch
                    size="small"
                    checkedChildren={t('隐藏')}
                    unCheckedChildren={t('可见')}
                    checked={hidden}
                    onChange={(checked) => {
                      const perms = { ...(approval.fieldPerms ?? {}) }
                      if (checked) perms[f.key] = 'hidden'
                      else delete perms[f.key]
                      onChange({ fieldPerms: Object.keys(perms).length ? perms : undefined })
                    }}
                  />
                </div>
              )
            })}
          </Space>
        </div>
      )}
    </Space>
  )
}
