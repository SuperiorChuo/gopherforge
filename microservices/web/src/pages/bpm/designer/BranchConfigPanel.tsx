import { useTranslation } from 'react-i18next'
import { Alert, Button, Input, Segmented, Select, Space, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import {
  BPM_CONDITION_OP_META,
  BPM_FORM_FIELD_LABELS,
  draftToExpr,
  exprToDraft,
  type ConditionLeafOp,
  type DesignerBranch,
} from '@/api/bpm'

const { Text } = Typography

const LEAF_OPS = Object.keys(BPM_CONDITION_OP_META) as ConditionLeafOp[]

export default function BranchConfigPanel({
  branch,
  readOnly,
  formFields,
  fieldLabels,
  onChange,
}: {
  branch: DesignerBranch
  readOnly: boolean
  formFields: string[]
  /** 字段中文名（表单 Schema 优先） */
  fieldLabels?: Record<string, string>
  onChange: (patch: Partial<DesignerBranch>) => void
}) {
  const { t } = useTranslation()
  const isDefault = !branch.expr
  const draft = exprToDraft(branch.expr)

  const commitRows = (logic: 'and' | 'or', rows: typeof draft.rows) => {
    onChange({ expr: draftToExpr({ logic, rows }) ?? { op: 'and', items: [] } })
  }

  const labels = fieldLabels ?? BPM_FORM_FIELD_LABELS
  const fieldOptions = formFields.map((f) => ({
    value: f,
    label: labels[f] && labels[f] !== f ? `${labels[f]}（${f}）` : f,
  }))

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <div>
        <Text type="secondary">{t('分支名称')}</Text>
        <Input
          style={{ marginTop: 4 }}
          value={branch.name}
          disabled={readOnly}
          maxLength={64}
          placeholder={t('如：金额 ≥ 10 万')}
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </div>
      {isDefault ? (
        <Alert
          type="info"
          showIcon
          message={t('默认兜底分支')}
          description={t('其余条件分支均未命中时进入此分支；默认分支不可配置条件、不可删除。')}
        />
      ) : (
        <>
          <div>
            <Text type="secondary">{t('多条件组合方式')}</Text>
            <div style={{ marginTop: 6 }}>
              <Segmented
                block
                disabled={readOnly}
                value={draft.logic}
                options={[
                  { label: t('且（全部满足）'), value: 'and' },
                  { label: t('或（任一满足）'), value: 'or' },
                ]}
                onChange={(v) => commitRows(v as 'and' | 'or', draft.rows)}
              />
            </div>
          </div>
          <div>
            <Text type="secondary">{t('条件（按发起表单快照求值；金额字段单位为分）')}</Text>
            <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 6 }}>
              {draft.rows.map((row, ri) => (
                <Space.Compact key={ri} block>
                  <Select
                    style={{ width: '42%' }}
                    disabled={readOnly}
                    placeholder={t('字段')}
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
                    options={LEAF_OPS.map((op) => ({ value: op, label: t(BPM_CONDITION_OP_META[op]) }))}
                    onChange={(v: ConditionLeafOp) => {
                      const rows = draft.rows.map((r, i) => (i === ri ? { ...r, op: v } : r))
                      commitRows(draft.logic, rows)
                    }}
                  />
                  <Input
                    style={{ width: '28%' }}
                    disabled={readOnly}
                    placeholder={row.op === 'in' ? t('多值逗号分隔') : t('值')}
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
                  {t('添加条件行')}
                </Button>
              )}
              {!draft.rows.length && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {t('尚未配置条件；发布前必须至少一行完整条件')}
                </Text>
              )}
            </Space>
          </div>
        </>
      )}
    </Space>
  )
}
