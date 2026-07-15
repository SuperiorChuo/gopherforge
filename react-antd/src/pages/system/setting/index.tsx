import { useEffect, useState } from 'react'
import {
  Tabs, Card, Input, Button, Form, InputNumber, Switch, Select, Collapse, Empty, Tag,
} from 'antd'
import { message } from '@/utils/feedback'
import { SaveOutlined, ReloadOutlined } from '@ant-design/icons'
import type { SystemSetting } from '@/types'
import { getSettingList, upsertSetting } from '@/api/system/setting'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

// 后端按 setting_key 前缀过滤分组（LIKE 'group.%'）
const GROUPS = [
  { key: 'security', label: '安全设置' },
  { key: 'notification', label: '通知设置' },
  { key: 'storage', label: '存储设置' },
  { key: 'general', label: '通用设置' },
]

interface FieldDef {
  key: string
  label: string
  type: 'number' | 'string' | 'boolean' | 'emails' | 'textarea'
  tooltip?: string
  min?: number
}

// 已知设置键的字段结构（与 server/internal/pkg/runtimeconfig 消费的字段一一对应）
const FIELD_SCHEMAS: Record<string, { title: string; fields: FieldDef[] }> = {
  'security.policy': {
    title: '安全策略',
    fields: [
      { key: 'password_max_age_days', label: '密码最长有效期（天）', type: 'number', min: 0, tooltip: '0 表示永不过期' },
      { key: 'password_history_count', label: '禁止重复使用的历史密码数', type: 'number', min: 0 },
      { key: 'login_limit_max_failures', label: '登录失败锁定阈值（次）', type: 'number', min: 1 },
      { key: 'login_limit_window_minutes', label: '失败统计窗口（分钟）', type: 'number', min: 1 },
      { key: 'login_limit_lock_minutes', label: '锁定时长（分钟）', type: 'number', min: 1 },
      { key: 'rate_limit_rps', label: '接口限流（请求/秒）', type: 'number', min: 1 },
    ],
  },
  'notification.email': {
    title: '邮件通知',
    fields: [
      { key: 'enabled', label: '启用邮件通知', type: 'boolean' },
      { key: 'smtp_host', label: 'SMTP 服务器', type: 'string' },
      { key: 'sender', label: '发件人地址', type: 'string' },
      { key: 'use_tls', label: '使用 TLS', type: 'boolean', tooltip: '与 STARTTLS 互斥，同时开启会导致整组配置失效' },
      { key: 'start_tls', label: '使用 STARTTLS', type: 'boolean' },
      { key: 'alert_receivers', label: '告警收件人', type: 'emails' },
      { key: 'subject_template', label: '邮件主题模板', type: 'string' },
      { key: 'body_template', label: '邮件正文模板', type: 'textarea' },
    ],
  },
}

function renderField(f: FieldDef) {
  switch (f.type) {
    case 'number':
      return <InputNumber min={f.min} style={{ width: 220 }} />
    case 'boolean':
      return <Switch />
    case 'emails':
      return (
        <Select
          mode="tags"
          style={{ maxWidth: 520 }}
          placeholder="输入邮箱后回车，可添加多个"
          tokenSeparators={[',', ' ']}
          open={false}
        />
      )
    case 'textarea':
      return <Input.TextArea rows={4} style={{ maxWidth: 520 }} />
    default:
      return <Input style={{ maxWidth: 520 }} />
  }
}

// 已知键 → 结构化表单；保存时与原 JSON 合并，schema 之外的字段（如 recipient_groups）原样保留
function SchemaSettingCard({ setting, canUpdate, onSaved }: {
  setting: SystemSetting
  canUpdate: boolean
  onSaved: () => void
}) {
  const schema = FIELD_SCHEMAS[setting.setting_key]
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    form.setFieldsValue(setting.value_json ?? {})
  }, [setting, form])

  const handleSave = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSaving(true)
    try {
      await upsertSetting(setting.setting_key, { ...(setting.value_json ?? {}), ...values })
      message.success('保存成功')
      onSaved()
    } catch {
      message.error('保存失败')
    } finally {
      setSaving(false)
    }
  }

  const extraKeys = Object.keys(setting.value_json ?? {}).filter(
    (k) => !schema.fields.some((f) => f.key === k),
  )

  return (
    <Card
      title={
        <span>
          {schema.title}
          <Tag variant="filled" className="cell-mono" style={{ marginLeft: 10 }}>{setting.setting_key}</Tag>
        </span>
      }
      extra={
        <span className="card-extra-note">
          更新于 {formatDateTime(setting.updated_at)}
        </span>
      }
      style={{ marginBottom: 16 }}
    >
      <Form form={form} labelCol={{ span: 7 }} wrapperCol={{ span: 17 }} style={{ maxWidth: 760 }}>
        {schema.fields.map((f) => (
          <Form.Item
            key={f.key}
            name={f.key}
            label={f.label}
            tooltip={f.tooltip}
            valuePropName={f.type === 'boolean' ? 'checked' : 'value'}
          >
            {renderField(f)}
          </Form.Item>
        ))}
        {extraKeys.length > 0 && (
          <Form.Item label="其他字段" tooltip="结构化表单未覆盖的字段，保存时原样保留">
            <span className="cell-mono card-extra-note">
              {extraKeys.join('、')}
            </span>
          </Form.Item>
        )}
        {canUpdate && (
          <Form.Item wrapperCol={{ offset: 7, span: 17 }} style={{ marginBottom: 0 }}>
            <Button type="primary" icon={<SaveOutlined />} onClick={handleSave} loading={saving}>
              保存
            </Button>
          </Form.Item>
        )}
      </Form>
    </Card>
  )
}

// 未知键回退为 JSON 编辑器
function JsonSettingCard({ setting, canUpdate, onSaved }: {
  setting: SystemSetting
  canUpdate: boolean
  onSaved: () => void
}) {
  const [raw, setRaw] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setRaw(JSON.stringify(setting.value_json ?? {}, null, 2))
  }, [setting])

  const handleSave = async () => {
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(raw)
    } catch {
      message.error('JSON 格式错误，请检查输入')
      return
    }
    setSaving(true)
    try {
      await upsertSetting(setting.setting_key, parsed)
      message.success('保存成功')
      onSaved()
    } catch {
      message.error('保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      title={<Tag variant="filled" className="cell-mono">{setting.setting_key}</Tag>}
      extra={
        <span className="card-extra-note">
          更新于 {formatDateTime(setting.updated_at)}
        </span>
      }
      style={{ marginBottom: 16 }}
    >
      <Input.TextArea
        rows={6}
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
        style={{ fontFamily: 'monospace', fontSize: 12 }}
        readOnly={!canUpdate}
      />
      {canUpdate && (
        <Button
          type="primary"
          icon={<SaveOutlined />}
          onClick={handleSave}
          loading={saving}
          style={{ marginTop: 12 }}
        >
          保存
        </Button>
      )}
    </Card>
  )
}

function SettingGroupPanel({ group }: { group: string }) {
  const [list, setList] = useState<SystemSetting[]>([])
  const [loading, setLoading] = useState(false)
  const { hasPerm } = usePermission()
  const canUpdate = hasPerm('system:setting:update')

  const fetchSettings = async () => {
    setLoading(true)
    try {
      const res = await getSettingList(group)
      setList(res ?? [])
    } catch {
      message.error('加载设置失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchSettings()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group])

  const known = list.filter((s) => FIELD_SCHEMAS[s.setting_key])
  const unknown = list.filter((s) => !FIELD_SCHEMAS[s.setting_key])

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <Button icon={<ReloadOutlined />} onClick={fetchSettings} loading={loading}>刷新</Button>
      </div>

      {list.length === 0 && !loading && (
        <Card>
          <Empty description="该分组暂无设置项" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '32px 0' }} />
        </Card>
      )}

      {known.map((s) => (
        <SchemaSettingCard key={s.setting_key} setting={s} canUpdate={canUpdate} onSaved={fetchSettings} />
      ))}

      {unknown.length > 0 && (
        <Collapse
          ghost
          items={[
            {
              key: 'raw',
              label: `其他设置项（JSON 编辑，${unknown.length} 个）`,
              children: unknown.map((s) => (
                <JsonSettingCard key={s.setting_key} setting={s} canUpdate={canUpdate} onSaved={fetchSettings} />
              )),
            },
          ]}
        />
      )}
    </div>
  )
}

export default function SettingPage() {
  return (
    <Tabs
      className="page-tabs"
      defaultActiveKey={GROUPS[0].key}
      items={GROUPS.map((g) => ({
        key: g.key,
        label: g.label,
        children: <SettingGroupPanel group={g.key} />,
      }))}
    />
  )
}
