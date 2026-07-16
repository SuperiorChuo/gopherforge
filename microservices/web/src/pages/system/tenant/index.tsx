import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, InputNumber, Modal, Select, Space, Table, Tag } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import type { TenantInfo } from '@/types'
import * as TenantAPI from '@/api/system/tenant'

export default function TenantPage() {
  const [list, setList] = useState<TenantInfo[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editRow, setEditRow] = useState<TenantInfo | null>(null)
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()

  async function load(p = page, kw = keyword) {
    setLoading(true)
    try {
      const data = (await TenantAPI.getTenantList({
        page: p,
        page_size: 20,
        keyword: kw || undefined,
      })) as { list?: TenantInfo[]; total?: number }
      const rows = data.list || []
      setList(rows)
      setTotal(data.total ?? rows.length)
      setPage(p)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function onCreate() {
    const values = await form.validateFields()
    try {
      await TenantAPI.createTenant({
        code: values.code,
        name: values.name,
        plan: values.plan || 'free',
        max_users: values.max_users || 0,
        status: 1,
      })
      message.success('已创建租户')
      setCreateOpen(false)
      form.resetFields()
      await load(1)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  function openEdit(row: TenantInfo) {
    setEditRow(row)
    editForm.setFieldsValue({
      name: row.name,
      plan: row.plan,
      max_users: row.max_users,
      status: row.status,
    })
  }

  async function onSaveEdit() {
    if (!editRow) return
    const values = await editForm.validateFields()
    try {
      await TenantAPI.updateTenant(editRow.id, {
        name: values.name,
        plan: values.plan,
        max_users: values.max_users,
        status: values.status,
      })
      message.success('已保存')
      setEditRow(null)
      await load(page)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  return (
    <Card
      title="租户管理（SaaS M1）"
      extra={
        <Space>
          <Input.Search
            placeholder="code / 名称"
            allowClear
            onSearch={(v) => {
              setKeyword(v)
              void load(1, v)
            }}
            style={{ width: 220 }}
          />
          <Button icon={<ReloadOutlined />} onClick={() => void load(page)}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建租户
          </Button>
        </Space>
      }
    >
      <Table
        rowKey="id"
        loading={loading}
        dataSource={list}
        pagination={{
          current: page,
          total,
          pageSize: 20,
          onChange: (p) => void load(p),
        }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          { title: 'Code', dataIndex: 'code', width: 140 },
          { title: '名称', dataIndex: 'name' },
          {
            title: '套餐',
            dataIndex: 'plan',
            width: 100,
            render: (v: string) => <Tag>{v || 'free'}</Tag>,
          },
          {
            title: '用户上限',
            dataIndex: 'max_users',
            width: 100,
            render: (v: number) => (v > 0 ? v : '不限'),
          },
          {
            title: '状态',
            dataIndex: 'status',
            width: 90,
            render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
          },
          {
            title: '操作',
            width: 100,
            render: (_, row) => (
              <Button size="small" onClick={() => openEdit(row)}>
                编辑
              </Button>
            ),
          },
        ]}
      />

      <Modal title="新建租户" open={createOpen} onCancel={() => setCreateOpen(false)} onOk={() => void onCreate()}>
        <Form form={form} layout="vertical" initialValues={{ plan: 'free', max_users: 0 }}>
          <Form.Item
            name="code"
            label="Code"
            rules={[{ required: true, message: '必填' }]}
            extra="小写字母数字与连字符，登录时填写 tenant_code"
          >
            <Input placeholder="acme" />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="Acme 公司" />
          </Form.Item>
          <Form.Item name="plan" label="套餐">
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label="最大用户数（0=不限）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editRow ? `编辑租户 #${editRow.id}` : '编辑'}
        open={!!editRow}
        onCancel={() => setEditRow(null)}
        onOk={() => void onSaveEdit()}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="plan" label="套餐">
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label="最大用户数">
            <InputNumber min={0} style={{ width: '100%' }} />
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
    </Card>
  )
}
