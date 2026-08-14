import { Button, Checkbox, Input, Select, Space, Table, Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import type { ColumnsType } from 'antd/es/table'
import type { CodegenColumn, CodegenFieldConfig, CodegenFormComponent } from '@/api/system/codegen'

export type DictTypeOption = { label: string; value: string }

type Props = {
  columns: CodegenColumn[]
  value: CodegenFieldConfig[]
  dictTypes: DictTypeOption[]
  onChange: (value: CodegenFieldConfig[]) => void
  compact?: boolean
}

const componentOptions: Array<{ label: string; value: CodegenFormComponent }> = [
  { label: '单行输入', value: 'input' },
  { label: '多行输入', value: 'textarea' },
  { label: '数字', value: 'number' },
  { label: '开关', value: 'switch' },
  { label: '日期', value: 'date' },
  { label: '日期时间', value: 'datetime' },
  { label: '下拉选择', value: 'select' },
]

export default function FieldConfigTable({ columns, value, dictTypes, onChange, compact }: Props) {
  const { t } = useTranslation()
  const byName = new Map(columns.map((column) => [column.name, column]))

  function update(name: string, patch: Partial<CodegenFieldConfig>) {
    onChange(value.map((field) => field.name === name ? { ...field, ...patch } : field))
  }

  function updateAll(key: 'in_list' | 'in_form', checked: boolean) {
    onChange(value.map((field) => ({ ...field, [key]: checked })))
  }

  const tableColumns: ColumnsType<CodegenFieldConfig> = [
    {
      title: t('字段'), dataIndex: 'name', width: 160, fixed: 'left',
      render: (name: string) => <span style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{name}</span>,
    },
    {
      title: t('类型'), width: 110,
      render: (_, field) => <Tag>{byName.get(field.name)?.db_type || '-'}</Tag>,
    },
    {
      title: t('显示名'), dataIndex: 'label', width: 150,
      render: (_, field) => <Input size="small" value={field.label} aria-label={t('{{name}} 显示名', { name: field.name })} onChange={(event) => update(field.name, { label: event.target.value })} />,
    },
    {
      title: t('控件'), dataIndex: 'component', width: 140,
      render: (_, field) => (
        <Select
          size="small"
          value={field.component}
          options={componentOptions.map((o) => ({ ...o, label: t(o.label) }))}
          aria-label={t('{{name}} 控件', { name: field.name })}
          style={{ width: '100%' }}
          onChange={(component) => update(field.name, { component })}
        />
      ),
    },
    {
      title: t('字典'), dataIndex: 'dict_type', width: 190,
      render: (_, field) => (
        <Select
          allowClear
          showSearch
          size="small"
          value={field.dict_type || undefined}
          options={dictTypes}
          aria-label={t('{{name}} 字典', { name: field.name })}
          style={{ width: '100%' }}
          onChange={(dictType) => update(field.name, { dict_type: dictType || '', component: dictType ? 'select' : field.component })}
        />
      ),
    },
    ...(['in_list', 'in_search', 'in_form', 'required'] as const).map((key) => ({
      title: t(({ in_list: '列表', in_search: '搜索', in_form: '表单', required: '必填' })[key]),
      dataIndex: key,
      width: 72,
      align: 'center' as const,
      render: (_: unknown, field: CodegenFieldConfig) => (
        <Checkbox
          aria-label={t('{{name}} {{key}}', { name: field.name, key })}
          checked={field[key]}
          disabled={key === 'in_search' && byName.get(field.name)?.go_type !== 'string'}
          onChange={(event) => update(field.name, { [key]: event.target.checked })}
        />
      ),
    })),
  ]

  return (
    <>
      {!compact && (
        <Space style={{ marginBottom: 12 }} wrap>
          <Button size="small" onClick={() => updateAll('in_list', true)}>{t('列表全选')}</Button>
          <Button size="small" onClick={() => updateAll('in_list', false)}>{t('清空列表')}</Button>
          <Button size="small" onClick={() => updateAll('in_form', true)}>{t('表单全选')}</Button>
          <Button size="small" onClick={() => updateAll('in_form', false)}>{t('清空表单')}</Button>
        </Space>
      )}
      <Table
        rowKey="name"
        size="small"
        columns={tableColumns}
        dataSource={value}
        pagination={false}
        scroll={{ x: 1120 }}
      />
    </>
  )
}
