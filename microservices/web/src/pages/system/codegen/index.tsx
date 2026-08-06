import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import axios from 'axios'
import { Alert, Button, Form, Input, Radio, Space, Steps, Typography } from 'antd'
import { EyeOutlined, LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  getCodegenCapabilities,
  getCodegenSchema,
  downloadCodegen,
  listCodegenTables,
  previewCodegen,
  writeCodegen,
  CodegenHTTPError,
  type CodegenCapabilities,
  type CodegenColumn,
  type CodegenFieldConfig,
  type CodegenPlan,
  type CodegenRequest,
  type CodegenSchema,
  type CodegenTable,
  type CodegenTplType,
} from '@/api/codegen'
import { getDictTypeList } from '@/api/system/dict'
import FieldConfigTable, { type DictTypeOption } from './FieldConfigTable'
import PlanPreview from './PlanPreview'
import RelationConfig, { type RelationConfigValue } from './RelationConfig'
import TablePicker from './TablePicker'

type WizardValues = {
  module?: string
  title?: string
  tpl_type?: CodegenTplType
}

const managedFields = new Set(['id', 'tenant_id', 'created_at', 'updated_at', 'deleted_at'])

function inferComponent(column: CodegenColumn): CodegenFieldConfig['component'] {
  if (column.go_type === 'bool') return 'switch'
  if (column.go_type === 'int64' || column.go_type === 'float64') return 'number'
  if (column.go_type === 'time.Time') return column.db_type === 'date' ? 'date' : 'datetime'
  return column.db_type.includes('text') ? 'textarea' : 'input'
}

function initialFields(schema: CodegenSchema): CodegenFieldConfig[] {
  return schema.columns
    .filter((column) => !column.primary_key && !managedFields.has(column.name))
    .map((column) => ({
      name: column.name,
      label: column.label || column.comment || column.name,
      in_list: true,
      in_search: column.go_type === 'string',
      in_form: true,
      required: !column.nullable,
      dict_type: '',
      component: inferComponent(column),
    }))
}

function moduleFromTable(table: string) {
  const normalized = table.toLowerCase().replace(/^[^a-z]+/, '').replace(/[^a-z0-9]/g, '')
  return normalized.slice(0, 32)
}

export default function CodegenPage() {
  const { t } = useTranslation()
  const [step, setStep] = useState(0)
  const [tables, setTables] = useState<CodegenTable[]>([])
  const [schema, setSchema] = useState<CodegenSchema | null>(null)
  const [fieldConfigs, setFieldConfigs] = useState<CodegenFieldConfig[]>([])
  const [relations, setRelations] = useState<RelationConfigValue>({ m2ms: [] })
  const [dictTypes, setDictTypes] = useState<DictTypeOption[]>([])
  const [capabilities, setCapabilities] = useState<CodegenCapabilities | null>(null)
  const [plan, setPlan] = useState<CodegenPlan | null>(null)
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<WizardValues>()
  const mode = (Form.useWatch('tpl_type', form) || 'crud') as CodegenTplType

  async function loadTables() {
    setLoading(true)
    try {
      const response = await listCodegenTables()
      setTables(response.list || [])
    } catch {
      message.error(t('加载表列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadTables()
    void getCodegenCapabilities().then(setCapabilities).catch(() => setCapabilities(null))
    void getDictTypeList({ page: 1, page_size: 500, status: 1 })
      .then((response) => setDictTypes(response.list.map((item) => ({ label: `${item.name} (${item.code})`, value: item.code }))))
      .catch(() => setDictTypes([]))
    // 初始自动加载属于页面既有行为，避免用户先点一次刷新。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function configureTable(table: CodegenTable) {
    setLoading(true)
    try {
      const nextSchema = await getCodegenSchema(table.name)
      setSchema(nextSchema)
      setFieldConfigs(initialFields(nextSchema))
      setRelations({ m2ms: [] })
      setPlan(null)
      form.setFieldsValue({
        module: moduleFromTable(table.name),
        title: table.comment || table.name,
        tpl_type: 'crud',
      })
      setStep(1)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('加载表结构失败'))
    } finally {
      setLoading(false)
    }
  }

  function collectRequest(): CodegenRequest | null {
    if (!schema) return null
    const values = form.getFieldsValue(true)
    if (!values.module || !values.title) return null
    const request: CodegenRequest = {
      table: schema.name,
      module: values.module,
      title: values.title,
      tpl_type: mode,
      fields: fieldConfigs,
    }
    if (mode === 'tree') {
      if (!relations.tree?.parent_field || !relations.tree.name_field) return null
      request.tree = {
        parent_field: relations.tree.parent_field,
        name_field: relations.tree.name_field,
        sort_field: relations.tree.sort_field || undefined,
      }
      if (relations.m2ms.length > 0) request.m2ms = relations.m2ms
    } else if (mode === 'sub') {
      if (!relations.sub) return null
      request.sub = relations.sub
    } else if (relations.m2ms.length > 0) {
      request.m2ms = relations.m2ms
    }
    return request
  }

  async function nextStep() {
    try {
      await form.validateFields()
      if (mode === 'tree' && (!relations.tree?.parent_field || !relations.tree.name_field)) {
        message.error(t('请选择父级字段和显示字段'))
        return
      }
      if (mode === 'sub' && !relations.sub) {
        message.error(t('请选择子表关系'))
        return
      }
      setStep(2)
    } catch {
      // Ant Design Form 已显示字段级错误。
    }
  }

  async function preview() {
    const request = collectRequest()
    if (!request) {
      message.error(t('生成配置不完整'))
      return
    }
    setLoading(true)
    try {
      const nextPlan = await previewCodegen(request)
      setPlan(nextPlan)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('预检失败'))
    } finally {
      setLoading(false)
    }
  }

  function handleOutputError(error: unknown, fallback: string) {
    const status = error instanceof CodegenHTTPError
      ? error.status
      : axios.isAxiosError(error) ? error.response?.status : undefined
    if (status === 409) {
      setPlan(null)
      message.error(t('生成计划已变化，请重新预检'))
      return
    }
    message.error(error instanceof Error ? error.message : fallback)
  }

  async function downloadPlan() {
    if (!plan) return
    setLoading(true)
    try {
      const blob = await downloadCodegen(plan.request, plan.digest)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `codegen-${plan.request.module}.zip`
      anchor.click()
      URL.revokeObjectURL(url)
      message.success(t('ZIP 已下载'))
    } catch (error) {
      handleOutputError(error, t('下载失败'))
    } finally {
      setLoading(false)
    }
  }

  async function writePlan(confirmation: string) {
    if (!plan) return
    setLoading(true)
    try {
      const result = await writeCodegen(plan.request, plan.digest, confirmation)
      message.success(t('已创建 {{a}} 个文件并更新 {{b}} 个接入文件', { a: result.created.length, b: result.patched.length }))
      setPlan(null)
    } catch (error) {
      handleOutputError(error, t('写入失败'))
      throw error
    } finally {
      setLoading(false)
    }
  }

  function reset() {
    setStep(0)
    setSchema(null)
    setFieldConfigs([])
    setRelations({ m2ms: [] })
    setPlan(null)
    form.resetFields()
  }

  return (
    <div className="page-detail" style={{ minWidth: 0 }}>
      <Steps
        current={step}
        items={[{ title: t('选择数据表') }, { title: t('配置生成规则') }, { title: t('预检产物') }]}
        style={{ marginBottom: 24 }}
      />

      {capabilities && !capabilities.preview_enabled && (
        <Alert type="warning" showIcon message={t('当前环境未提供仓库快照，预检暂不可用')} style={{ marginBottom: 16 }} />
      )}

      {step === 0 && (
        <TablePicker tables={tables} loading={loading} onRefresh={() => void loadTables()} onConfigure={(table) => void configureTable(table)} />
      )}

      {step === 1 && schema && (
        <Form form={form} layout="vertical">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16 }}>
            <Form.Item name="module" label={t('模块名')} rules={[{ required: true }, { pattern: /^[a-z][a-z0-9]{1,31}$/, message: t('小写字母开头，2-32 个字符') }]}>
              <Input />
            </Form.Item>
            <Form.Item name="title" label={t('页面标题')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="tpl_type" label={t('生成模式')} initialValue="crud">
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                options={[{ label: t('单表'), value: 'crud' }, { label: t('树表'), value: 'tree' }, { label: t('主子表'), value: 'sub' }]}
                onChange={() => setPlan(null)}
              />
            </Form.Item>
          </div>

          <Typography.Title level={5}>{t('字段配置')}</Typography.Title>
          <FieldConfigTable columns={schema.columns} value={fieldConfigs} dictTypes={dictTypes} onChange={setFieldConfigs} />

          <div style={{ marginTop: 24 }}>
            <RelationConfig
              mode={mode}
              schema={schema}
              dictTypes={dictTypes}
              value={relations}
              loadSchema={getCodegenSchema}
              onChange={setRelations}
            />
          </div>

          <Space style={{ marginTop: 24 }} wrap>
            <Button icon={<LeftOutlined />} onClick={() => setStep(0)}>{t('上一步')}</Button>
            <Button type="primary" icon={<RightOutlined />} iconPosition="end" onClick={() => void nextStep()}>{t('下一步')}</Button>
          </Space>
        </Form>
      )}

      {step === 2 && (
        <div>
          <Space wrap style={{ marginBottom: plan ? 16 : 0 }}>
            <Button icon={<LeftOutlined />} onClick={() => setStep(1)}>{t('上一步')}</Button>
            <Button type="primary" icon={<EyeOutlined />} loading={loading} disabled={capabilities?.preview_enabled === false} onClick={() => void preview()}>
              {plan ? t('重新预检') : t('预检代码')}
            </Button>
            <Button icon={<ReloadOutlined />} onClick={reset}>{t('重新选择')}</Button>
          </Space>
          {plan && (
            <PlanPreview
              plan={plan}
              capabilities={capabilities}
              loading={loading}
              onDownload={downloadPlan}
              onWrite={writePlan}
            />
          )}
        </div>
      )}
    </div>
  )
}
