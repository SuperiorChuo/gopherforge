import { useEffect, useState } from 'react'
import { Tabs, Card, Input, Button, message, Table, Space } from 'antd'
import { SaveOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemSetting } from '@/types'
import { getSettingList, upsertSetting } from '@/api/system/setting'

const GROUPS = [
  { key: 'security', label: '安全设置' },
  { key: 'email', label: '邮件设置' },
  { key: 'storage', label: '存储设置' },
  { key: 'general', label: '通用设置' },
]

function SettingGroupPanel({ group }: { group: string }) {
  const [list, setList] = useState<SystemSetting[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState<Record<string, boolean>>({})
  const [values, setValues] = useState<Record<string, string>>({})

  const fetchSettings = async () => {
    setLoading(true)
    try {
      const res = await getSettingList(group)
      setList(res)
      const map: Record<string, string> = {}
      for (const s of res) {
        map[s.setting_key] = JSON.stringify(s.value_json, null, 2)
      }
      setValues(map)
    } catch {
      message.error('加载设置失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchSettings()
  }, [group])

  const handleSave = async (key: string) => {
    const raw = values[key] ?? '{}'
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(raw)
    } catch {
      message.error('JSON 格式错误，请检查输入')
      return
    }
    setSaving((prev) => ({ ...prev, [key]: true }))
    try {
      await upsertSetting(key, parsed)
      message.success('保存成功')
    } catch {
      message.error('保存失败')
    } finally {
      setSaving((prev) => ({ ...prev, [key]: false }))
    }
  }

  const columns: ColumnsType<SystemSetting> = [
    { title: '设置键', dataIndex: 'setting_key', width: 200 },
    {
      title: '设置值 (JSON)',
      dataIndex: 'setting_key',
      render: (key: string) => (
        <Input.TextArea
          rows={4}
          value={values[key] ?? ''}
          onChange={(e) => setValues((prev) => ({ ...prev, [key]: e.target.value }))}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
        />
      ),
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 170 },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button
          type="primary"
          size="small"
          icon={<SaveOutlined />}
          loading={saving[record.setting_key]}
          onClick={() => handleSave(record.setting_key)}
        >
          保存
        </Button>
      ),
    },
  ]

  return (
    <Card
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={fetchSettings} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <Table
        rowKey="setting_key"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
      />
    </Card>
  )
}

export default function SettingPage() {
  return (
    <Tabs
      defaultActiveKey={GROUPS[0].key}
      items={GROUPS.map((g) => ({
        key: g.key,
        label: g.label,
        children: <SettingGroupPanel group={g.key} />,
      }))}
    />
  )
}
