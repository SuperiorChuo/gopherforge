import { useEffect, useMemo, useState } from 'react'
import { Button, Divider, Form, Input, Select, Space, Tooltip, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import type {
  CodegenFieldConfig,
  CodegenM2MConfig,
  CodegenSchema,
  CodegenSubConfig,
  CodegenTplType,
  CodegenTreeConfig,
} from '@/api/codegen'
import FieldConfigTable, { type DictTypeOption } from './FieldConfigTable'

export type RelationConfigValue = {
  tree?: Partial<CodegenTreeConfig>
  sub?: CodegenSubConfig
  m2ms: CodegenM2MConfig[]
}

type Props = {
  mode: CodegenTplType
  schema: CodegenSchema
  dictTypes: DictTypeOption[]
  value: RelationConfigValue
  loadSchema: (table: string) => Promise<CodegenSchema>
  onChange: (value: RelationConfigValue) => void
}

const managedFields = new Set(['id', 'tenant_id', 'created_at', 'updated_at', 'deleted_at'])

function initialFieldConfig(schema: CodegenSchema, excluded = ''): CodegenFieldConfig[] {
  return schema.columns
    .filter((column) => !column.primary_key && column.name !== excluded && !managedFields.has(column.name))
    .map((column) => ({
      name: column.name,
      label: column.label || column.comment || column.name,
      in_list: true,
      in_search: column.go_type === 'string',
      in_form: true,
      required: !column.nullable,
      dict_type: '',
      component: inferComponent(column.go_type, column.db_type),
    }))
}

function inferComponent(goType: string, dbType: string): CodegenFieldConfig['component'] {
  if (goType === 'bool') return 'switch'
  if (goType === 'int64' || goType === 'float64') return 'number'
  if (goType === 'time.Time') return dbType === 'date' ? 'date' : 'datetime'
  return dbType.includes('text') ? 'textarea' : 'input'
}

export default function RelationConfig({ mode, schema, dictTypes, value, loadSchema, onChange }: Props) {
  const [relatedSchemas, setRelatedSchemas] = useState<Record<string, CodegenSchema>>({})
  const integerColumns = schema.columns.filter((column) => !column.primary_key && column.go_type === 'int64' && !managedFields.has(column.name))
  const textColumns = schema.columns.filter((column) => !column.primary_key && column.go_type === 'string' && !managedFields.has(column.name))
  const regularColumns = schema.columns.filter((column) => !column.primary_key && !managedFields.has(column.name))
  const subCandidates = schema.relations.filter((relation) => relation.kind === 'one_to_many')
  const m2mCandidates = schema.relations.filter((relation) => relation.kind === 'many_to_many')

  useEffect(() => {
    setRelatedSchemas({})
  }, [schema.name])

  async function ensureSchema(table: string) {
    if (relatedSchemas[table]) return relatedSchemas[table]
    const target = await loadSchema(table)
    setRelatedSchemas((current) => ({ ...current, [table]: target }))
    return target
  }

  async function selectSubTable(table: string) {
    const candidate = subCandidates.find((relation) => relation.target_table === table)
    if (!candidate) return
    const subSchema = await ensureSchema(table)
    onChange({
      ...value,
      sub: { table, fk_field: candidate.fk_field, fields: initialFieldConfig(subSchema, candidate.fk_field) },
      m2ms: [],
    })
  }

  async function addM2M() {
    const candidate = m2mCandidates.find((relation) => !value.m2ms.some((item) => item.join_table === relation.join_table && item.target_table === relation.target_table))
    if (!candidate || !candidate.join_table || !candidate.target_fk) return
    const target = await ensureSchema(candidate.target_table)
    const display = target.columns.find((column) => !column.primary_key && column.go_type === 'string')?.name || target.primary_key
    onChange({
      ...value,
      m2ms: [...value.m2ms, {
        name: candidate.target_table,
        join_table: candidate.join_table,
        fk_field: candidate.fk_field,
        target_table: candidate.target_table,
        target_fk: candidate.target_fk,
        display_field: display,
        label: target.comment || candidate.target_table,
      }],
    })
  }

  function updateM2M(index: number, patch: Partial<CodegenM2MConfig>) {
    onChange({ ...value, m2ms: value.m2ms.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item) })
  }

  const unusedM2MCount = useMemo(
    () => m2mCandidates.filter((candidate) => !value.m2ms.some((item) => item.join_table === candidate.join_table && item.target_table === candidate.target_table)).length,
    [m2mCandidates, value.m2ms],
  )

  return (
    <div>
      {mode === 'tree' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16 }}>
          <Form.Item label="父级字段" required>
            <Select
              value={value.tree?.parent_field}
              placeholder="选择父级字段"
              options={integerColumns.map((column) => ({ label: `${column.name} (${column.db_type})`, value: column.name }))}
              onChange={(parentField) => onChange({ ...value, tree: { ...value.tree, parent_field: parentField } })}
            />
          </Form.Item>
          <Form.Item label="显示字段" required>
            <Select
              value={value.tree?.name_field}
              placeholder="选择显示字段"
              options={textColumns.map((column) => ({ label: `${column.name} (${column.db_type})`, value: column.name }))}
              onChange={(nameField) => onChange({ ...value, tree: { ...value.tree, name_field: nameField } })}
            />
          </Form.Item>
          <Form.Item label="排序字段">
            <Select
              allowClear
              value={value.tree?.sort_field}
              placeholder="默认按 ID"
              options={regularColumns.filter((column) => column.name !== value.tree?.parent_field).map((column) => ({ label: `${column.name} (${column.db_type})`, value: column.name }))}
              onChange={(sortField) => onChange({ ...value, tree: { ...value.tree, sort_field: sortField } })}
            />
          </Form.Item>
        </div>
      )}

      {mode === 'sub' && (
        <>
          <Form.Item label="子表关系" required>
            <Select
              showSearch
              value={value.sub?.table}
              placeholder="选择已识别的一对多关系"
              options={subCandidates.map((candidate) => ({
                label: `${candidate.target_table} (${candidate.fk_field})`,
                value: candidate.target_table,
              }))}
              onChange={(table) => void selectSubTable(table)}
            />
          </Form.Item>
          {value.sub && relatedSchemas[value.sub.table] && (
            <>
              <Typography.Title level={5}>子表字段</Typography.Title>
              <FieldConfigTable
                compact
                columns={relatedSchemas[value.sub.table].columns}
                value={value.sub.fields}
                dictTypes={dictTypes}
                onChange={(fields) => onChange({ ...value, sub: { ...value.sub!, fields } })}
              />
            </>
          )}
        </>
      )}

      {mode !== 'sub' && (
        <>
          {mode === 'tree' && <Divider />}
          <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 12 }} wrap>
            <Typography.Title level={5} style={{ margin: 0 }}>多对多关系</Typography.Title>
            <Button icon={<PlusOutlined />} disabled={unusedM2MCount === 0} onClick={() => void addM2M()}>
              添加多对多关系
            </Button>
          </Space>
          {value.m2ms.map((relation, index) => {
            const target = relatedSchemas[relation.target_table]
            return (
              <div
                key={`${relation.join_table}-${relation.target_table}`}
                style={{ display: 'grid', gridTemplateColumns: 'minmax(140px, 1fr) minmax(140px, 1fr) minmax(180px, 1fr) 40px', gap: 12, alignItems: 'end', marginBottom: 12 }}
              >
                <Form.Item label="关系名" style={{ marginBottom: 0 }}>
                  <Input value={relation.name} onChange={(event) => updateM2M(index, { name: event.target.value })} />
                </Form.Item>
                <Form.Item label="显示名" style={{ marginBottom: 0 }}>
                  <Input value={relation.label} onChange={(event) => updateM2M(index, { label: event.target.value })} />
                </Form.Item>
                <Form.Item label="显示字段" style={{ marginBottom: 0 }}>
                  <Select
                    aria-label={`${relation.target_table} 显示字段`}
                    value={relation.display_field}
                    options={(target?.columns || []).filter((column) => !column.primary_key).map((column) => ({ label: column.name, value: column.name }))}
                    onChange={(displayField) => updateM2M(index, { display_field: displayField })}
                  />
                </Form.Item>
                <Tooltip title="删除关系">
                  <Button
                    aria-label="删除关系"
                    icon={<DeleteOutlined />}
                    danger
                    onClick={() => onChange({ ...value, m2ms: value.m2ms.filter((_, itemIndex) => itemIndex !== index) })}
                  />
                </Tooltip>
              </div>
            )
          })}
          {m2mCandidates.length === 0 && <Typography.Text type="secondary">当前表没有可配置的多对多关系</Typography.Text>}
        </>
      )}
    </div>
  )
}
