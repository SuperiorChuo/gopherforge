import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Input, Select, Space, Switch, Typography } from 'antd'
import { ArrowDownOutlined, ArrowUpOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { BPM_FORM_FIELD_TYPE_META, type BpmFormField, type BpmFormSchema } from '@/api/bpm'

const { Text } = Typography

export default function FormSchemaEditor({
  value,
  readOnly,
  onChange,
}: {
  value: BpmFormSchema | null
  readOnly: boolean
  onChange: (next: BpmFormSchema | null) => void
}) {
  const { t } = useTranslation()
  const fields = value?.fields ?? []
  const commit = (next: BpmFormField[]) => {
    onChange(next.length ? { version: value?.version ?? 1, fields: next } : null)
  }
  const patch = (i: number, p: Partial<BpmFormField>) => {
    commit(fields.map((f, idx) => (idx === i ? { ...f, ...p } : f)))
  }
  const move = (i: number, dir: -1 | 1) => {
    const j = i + dir
    if (j < 0 || j >= fields.length) return
    const next = [...fields]
    ;[next[i], next[j]] = [next[j], next[i]]
    commit(next)
  }

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message={t('配置发起表单后，本流程可在「发起申请」页零代码发起；发布时字段声明自动同步给条件分支求值。金额字段按分存储、元录入。')}
      />
      {fields.map((f, i) => (
        <Card key={i} size="small">
          <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space.Compact block>
              <Input
                style={{ width: '38%' }}
                className="cell-mono"
                disabled={readOnly}
                placeholder={t('key（如 amount_cents）')}
                value={f.key}
                maxLength={64}
                onChange={(e) => patch(i, { key: e.target.value.trim() })}
              />
              <Input
                style={{ width: '32%' }}
                disabled={readOnly}
                placeholder={t('显示名')}
                value={f.label}
                maxLength={64}
                onChange={(e) => patch(i, { label: e.target.value })}
              />
              <Select
                style={{ width: '30%' }}
                disabled={readOnly}
                value={f.type}
                options={Object.entries(BPM_FORM_FIELD_TYPE_META).map(([v, l]) => ({
                  value: v,
                  label: t(l),
                }))}
                onChange={(v) => patch(i, { type: v as BpmFormField['type'] })}
              />
            </Space.Compact>
            {(f.type === 'select' || f.type === 'radio') && (
              <Select
                mode="tags"
                style={{ width: '100%' }}
                disabled={readOnly}
                placeholder={t('输入选项后回车，可多个')}
                value={f.options ?? []}
                open={false}
                tokenSeparators={[',', '，']}
                onChange={(v: string[]) => patch(i, { options: v })}
              />
            )}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Space size={10}>
                <span>
                  <Text type="secondary" style={{ fontSize: 12 }}>{t('必填')}</Text>{' '}
                  <Switch
                    size="small"
                    disabled={readOnly}
                    checked={!!f.required}
                    onChange={(checked) => patch(i, { required: checked || undefined })}
                  />
                </span>
                <Input
                  size="small"
                  style={{ width: 180 }}
                  disabled={readOnly}
                  placeholder={t('占位提示（可空）')}
                  value={f.placeholder ?? ''}
                  maxLength={64}
                  onChange={(e) => patch(i, { placeholder: e.target.value || undefined })}
                />
              </Space>
              {!readOnly && (
                <Space size={0}>
                  <Button type="text" size="small" icon={<ArrowUpOutlined />} disabled={i === 0} onClick={() => move(i, -1)} />
                  <Button type="text" size="small" icon={<ArrowDownOutlined />} disabled={i === fields.length - 1} onClick={() => move(i, 1)} />
                  <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => commit(fields.filter((_, idx) => idx !== i))} />
                </Space>
              )}
            </div>
          </Space>
        </Card>
      ))}
      {!readOnly && (
        <Button
          type="dashed"
          block
          icon={<PlusOutlined />}
          onClick={() => commit([...fields, { key: '', label: '', type: 'input' }])}
        >
          {t('添加字段')}
        </Button>
      )}
      {!fields.length && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {t('未配置表单：本流程为"业务表单"模式，只能由业务后端经 internal 端点发起。')}
        </Text>
      )}
    </Space>
  )
}
