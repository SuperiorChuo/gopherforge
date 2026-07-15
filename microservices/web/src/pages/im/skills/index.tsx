import { useEffect, useState } from 'react'
import {
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  createSkillGroup,
  deleteAgentSkill,
  listSkillAgents,
  listSkillGroups,
  updateSkillGroup,
  upsertAgentSkill,
  type ImAgentSkill,
  type ImSkillGroup,
} from '@/api/im'

const strategyOptions = [
  { label: '轮询 round_robin', value: 'round_robin' },
  { label: '最少负载 least_load', value: 'least_load' },
  { label: '仅手动 manual', value: 'manual' },
]

export default function ImSkillsPage() {
  const [list, setList] = useState<ImSkillGroup[]>([])
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [editRow, setEditRow] = useState<ImSkillGroup | null>(null)
  const [agentsOf, setAgentsOf] = useState<ImSkillGroup | null>(null)
  const [agents, setAgents] = useState<ImAgentSkill[]>([])
  const [agentLoading, setAgentLoading] = useState(false)
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()
  const [agentForm] = Form.useForm()

  async function load() {
    setLoading(true)
    try {
      const data = await listSkillGroups()
      setList(data.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function onCreate() {
    const values = await form.validateFields()
    try {
      await createSkillGroup({
        name: values.name,
        code: values.code,
        strategy: values.strategy || 'round_robin',
        status: 1,
      })
      message.success('已创建')
      setCreateOpen(false)
      form.resetFields()
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  function openEdit(row: ImSkillGroup) {
    setEditRow(row)
    editForm.setFieldsValue({
      name: row.name,
      code: row.code,
      strategy: row.strategy,
      status: row.status,
    })
  }

  async function onSaveEdit() {
    if (!editRow) return
    const values = await editForm.validateFields()
    try {
      await updateSkillGroup(editRow.id, {
        name: values.name,
        code: values.code,
        strategy: values.strategy,
        status: values.status,
      })
      message.success('已保存')
      setEditRow(null)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  async function openAgents(row: ImSkillGroup) {
    setAgentsOf(row)
    setAgentLoading(true)
    try {
      const data = await listSkillAgents(row.id)
      setAgents(data.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载坐席失败')
    } finally {
      setAgentLoading(false)
    }
  }

  async function onAddAgent() {
    if (!agentsOf) return
    const values = await agentForm.validateFields()
    try {
      await upsertAgentSkill({
        agent_user_id: Number(values.agent_user_id),
        skill_group_id: agentsOf.id,
        max_concurrent: values.max_concurrent || 5,
        status: 1,
      })
      message.success('已绑定坐席')
      agentForm.resetFields()
      await openAgents(agentsOf)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '绑定失败')
    }
  }

  async function onRemoveAgent(id: number) {
    if (!agentsOf) return
    try {
      await deleteAgentSkill(id)
      message.success('已移除')
      await openAgents(agentsOf)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '移除失败')
    }
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        title="智能客服 · 技能组 (IM M3)"
        extra={
          <Space>
            <Button href="/im/desk">坐席工作台</Button>
            <Button type="primary" onClick={() => setCreateOpen(true)}>
              新建技能组
            </Button>
            <Button onClick={() => void load()}>刷新</Button>
          </Space>
        }
      >
        <Typography.Paragraph type="secondary">
          技能组用于排队路由。策略：<code>round_robin</code> 轮询、
          <code>least_load</code> 最少会话数、
          <code>manual</code> 仅手动接入。坐席需先「上线」才会被自动分配。
          绑定的 <code>agent_user_id</code> 为后台用户 ID。
        </Typography.Paragraph>
        <Table
          rowKey="id"
          loading={loading}
          dataSource={list}
          pagination={false}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 70 },
            { title: '名称', dataIndex: 'name' },
            { title: 'Code', dataIndex: 'code', width: 120 },
            {
              title: '策略',
              dataIndex: 'strategy',
              width: 140,
              render: (v: string) => <Tag>{v}</Tag>,
            },
            {
              title: '状态',
              dataIndex: 'status',
              width: 90,
              render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
            },
            {
              title: '坐席数',
              dataIndex: 'agent_count',
              width: 90,
            },
            {
              title: '操作',
              width: 220,
              render: (_, row) => (
                <Space>
                  <Button size="small" onClick={() => openEdit(row)}>
                    编辑
                  </Button>
                  <Button size="small" type="primary" onClick={() => void openAgents(row)}>
                    坐席
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Modal title="新建技能组" open={createOpen} onCancel={() => setCreateOpen(false)} onOk={() => void onCreate()}>
        <Form form={form} layout="vertical" initialValues={{ strategy: 'round_robin' }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="售前组" />
          </Form.Item>
          <Form.Item name="code" label="Code" rules={[{ required: true }]} extra="唯一标识，如 sales">
            <Input placeholder="sales" />
          </Form.Item>
          <Form.Item name="strategy" label="分配策略">
            <Select options={strategyOptions} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title={editRow ? `编辑 #${editRow.id}` : '编辑'} open={!!editRow} onCancel={() => setEditRow(null)} onOk={() => void onSaveEdit()}>
        <Form form={editForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="Code" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="strategy" label="分配策略">
            <Select options={strategyOptions} />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select
              options={[
                { label: '启用', value: 1 },
                { label: '停用', value: 0 },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={agentsOf ? `技能组坐席 · ${agentsOf.name}` : '坐席'}
        open={!!agentsOf}
        onCancel={() => setAgentsOf(null)}
        footer={null}
        width={720}
      >
        <Form form={agentForm} layout="inline" style={{ marginBottom: 16 }} onFinish={() => void onAddAgent()}>
          <Form.Item name="agent_user_id" label="用户 ID" rules={[{ required: true }]}>
            <InputNumber min={1} placeholder="后台 user id" style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="max_concurrent" label="最大并发" initialValue={5}>
            <InputNumber min={1} max={50} style={{ width: 100 }} />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              绑定
            </Button>
          </Form.Item>
        </Form>
        <Table
          rowKey="id"
          loading={agentLoading}
          dataSource={agents}
          pagination={false}
          size="small"
          columns={[
            { title: '绑定 ID', dataIndex: 'id', width: 80 },
            { title: '用户 ID', dataIndex: 'agent_user_id', width: 100 },
            { title: '最大并发', dataIndex: 'max_concurrent', width: 100 },
            {
              title: '状态',
              dataIndex: 'status',
              width: 80,
              render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
            },
            {
              title: '在线',
              width: 100,
              render: (_, row) => {
                const st = row.presence?.status || 'offline'
                const color = st === 'online' ? 'green' : st === 'busy' ? 'gold' : 'default'
                return <Tag color={color}>{st}</Tag>
              },
            },
            {
              title: '当前负载',
              dataIndex: 'assigned_count',
              width: 100,
            },
            {
              title: '操作',
              width: 90,
              render: (_, row) => (
                <Button size="small" danger onClick={() => void onRemoveAgent(row.id)}>
                  移除
                </Button>
              ),
            },
          ]}
        />
      </Modal>
    </Space>
  )
}
