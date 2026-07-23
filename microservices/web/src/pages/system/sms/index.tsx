import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Switch, Tabs, Descriptions,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, SendOutlined,
  EditOutlined, DeleteOutlined, EyeOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as SmsAPI from '@/api/system/sms'
import type { SmsChannel, SmsTemplate, SmsLog, SmsProvider, SmsChannelConfig } from '@/api/system/sms'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

const providerLabels: Record<SmsProvider, string> = {
  debug: '调试',
  aliyun: '阿里云',
  tencent: '腾讯云',
}

const providerColors: Record<SmsProvider, string> = {
  debug: 'default',
  aliyun: 'orange',
  tencent: 'blue',
}

const templateTypeLabels: Record<number, string> = { 1: '验证码', 2: '通知', 3: '营销' }
const templateTypeColors: Record<number, string> = { 1: 'blue', 2: 'green', 3: 'orange' }

// 与后端 pkg/sms 的占位规则一致：{name} 形式，字母数字下划线
const paramPattern = /\{([a-zA-Z0-9_]+)\}/g

const extractParams = (content: string): string[] => {
  const keys: string[] = []
  for (const match of content.matchAll(paramPattern)) {
    if (!keys.includes(match[1])) keys.push(match[1])
  }
  return keys
}

// ---------- 渠道 Tab ----------

interface ChannelSearchParams {
  keyword?: string
  provider?: string
  status?: number
  page: number
  page_size: number
}

function ChannelTab() {
  const [list, setList] = useState<SmsChannel[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<ChannelSearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SmsChannel | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()
  const provider = Form.useWatch<SmsProvider | undefined>('provider', form)

  const fetchList = useCallback(async (p: ChannelSearchParams) => {
    setLoading(true)
    try {
      const res = await SmsAPI.getSmsChannelList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取短信渠道列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  const handleSearch = (values: { keyword?: string; provider?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: SmsChannel) => {
    setEditRecord(record)
    const config = record.config ?? {}
    form.setFieldsValue({
      name: record.name,
      provider: record.provider,
      status: record.status,
      remark: record.remark,
      config_sign_name: config.sign_name,
      config_access_key_id: config.access_key_id,
      config_access_key_secret: config.access_key_secret,
      config_region_id: config.region_id,
      config_secret_id: config.secret_id,
      config_secret_key: config.secret_key,
      config_sdk_app_id: config.sdk_app_id,
      config_region: config.region,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await SmsAPI.deleteSmsChannel(id)
      message.success('删除成功')
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      // 拦截器已提示（被模板引用时后端返回 400）
    }
  }

  const handleToggleStatus = async (record: SmsChannel, checked: boolean) => {
    try {
      await SmsAPI.updateSmsChannelStatus(record.id, checked ? 1 : 0)
      message.success(checked ? '已启用' : '已停用')
      fetchList(params)
    } catch {
      message.error('状态更新失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      // 按 provider 组装 config JSON；密钥留空/为 ****** 时后端保留旧值
      const config: SmsChannelConfig = {}
      const put = (key: string, value?: string) => {
        if (value !== undefined && value !== null && value !== '') config[key] = value
      }
      put('sign_name', values.config_sign_name)
      if (values.provider === 'aliyun') {
        put('access_key_id', values.config_access_key_id)
        put('access_key_secret', values.config_access_key_secret)
        put('region_id', values.config_region_id)
      } else if (values.provider === 'tencent') {
        put('secret_id', values.config_secret_id)
        put('secret_key', values.config_secret_key)
        put('sdk_app_id', values.config_sdk_app_id)
        put('region', values.config_region)
      }
      const payload = {
        name: values.name,
        provider: values.provider,
        status: values.status,
        remark: values.remark ?? '',
        config,
      }
      if (editRecord) {
        await SmsAPI.updateSmsChannel(editRecord.id, payload)
        message.success('更新成功')
      } else {
        await SmsAPI.createSmsChannel(payload)
        message.success('创建成功')
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      // 拦截器已提示
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<SmsChannel> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '渠道名称', dataIndex: 'name', ellipsis: true },
    {
      title: '服务商',
      dataIndex: 'provider',
      width: 90,
      render: (v: SmsProvider) => (
        <Tag variant="filled" color={providerColors[v] ?? 'default'}>{providerLabels[v] ?? v}</Tag>
      ),
    },
    {
      title: '签名',
      dataIndex: 'config',
      width: 140,
      ellipsis: true,
      render: (config?: SmsChannelConfig | null) => config?.sign_name || '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number, record) => (
        <Switch
          size="small"
          checked={v === 1}
          checkedChildren="启用"
          unCheckedChildren="停用"
          disabled={!hasPerm('system:sms-channel:update')}
          onChange={(checked) => handleToggleStatus(record, checked)}
        />
      ),
    },
    { title: '备注', dataIndex: 'remark', ellipsis: true },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:sms-channel:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:sms-channel:delete') && (
            <Popconfirm title="确认删除该渠道?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Form
        form={searchForm}
        layout="inline"
        className="list-filter-form"
        onFinish={handleSearch}
        style={{ marginBottom: 16 }}
      >
        <Form.Item name="keyword">
          <Input placeholder="搜索渠道名称" prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="provider">
          <Select placeholder="服务商" style={{ width: 110 }} allowClear>
            <Select.Option value="debug">调试</Select.Option>
            <Select.Option value="aliyun">阿里云</Select.Option>
            <Select.Option value="tencent">腾讯云</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="状态" style={{ width: 100 }} allowClear>
            <Select.Option value={1}>启用</Select.Option>
            <Select.Option value={0}>停用</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
            <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <TableToolbar
        title="短信渠道"
        total={total}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
            {hasPerm('system:sms-channel:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增渠道</Button>
            )}
          </Space>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        locale={{ emptyText: <GlassEmpty text="暂无短信渠道" compact /> }}
        pagination={{
          total,
          current: params.page,
          pageSize: params.page_size,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
      />

      <Modal
        title={editRecord ? '编辑渠道' : '新增渠道'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="渠道名称" rules={[{ required: true, message: '请输入渠道名称' }]}>
            <Input placeholder="如：阿里云主渠道" />
          </Form.Item>
          <Form.Item name="provider" label="服务商" initialValue="debug" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="debug">调试（不真实外发，直接成功）</Select.Option>
              <Select.Option value="aliyun">阿里云</Select.Option>
              <Select.Option value="tencent">腾讯云</Select.Option>
            </Select>
          </Form.Item>
          {provider !== 'debug' && (
            <Form.Item
              name="config_sign_name"
              label="短信签名"
              rules={[{ required: true, message: '请输入云厂商审核通过的短信签名' }]}
            >
              <Input placeholder="如：某某科技" />
            </Form.Item>
          )}
          {provider === 'aliyun' && (
            <>
              <Form.Item name="config_access_key_id" label="AccessKey ID" rules={[{ required: true, message: '请输入 AccessKey ID' }]}>
                <Input placeholder="LTAI****************" />
              </Form.Item>
              <Form.Item
                name="config_access_key_secret"
                label="AccessKey Secret"
                rules={editRecord ? [] : [{ required: true, message: '请输入 AccessKey Secret' }]}
                extra={editRecord ? '留空或保持 ****** 表示不修改' : undefined}
              >
                <Input.Password placeholder="密钥只保存在服务端" autoComplete="new-password" />
              </Form.Item>
              <Form.Item name="config_region_id" label="地域（可选）">
                <Input placeholder="默认 cn-hangzhou" />
              </Form.Item>
            </>
          )}
          {provider === 'tencent' && (
            <>
              <Form.Item name="config_secret_id" label="SecretId" rules={[{ required: true, message: '请输入 SecretId' }]}>
                <Input placeholder="AKID****************" />
              </Form.Item>
              <Form.Item
                name="config_secret_key"
                label="SecretKey"
                rules={editRecord ? [] : [{ required: true, message: '请输入 SecretKey' }]}
                extra={editRecord ? '留空或保持 ****** 表示不修改' : undefined}
              >
                <Input.Password placeholder="密钥只保存在服务端" autoComplete="new-password" />
              </Form.Item>
              <Form.Item name="config_sdk_app_id" label="SdkAppId" rules={[{ required: true, message: '请输入短信应用 SdkAppId' }]}>
                <Input placeholder="1400******" />
              </Form.Item>
              <Form.Item name="config_region" label="地域（可选）">
                <Input placeholder="默认 ap-guangzhou" />
              </Form.Item>
            </>
          )}
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>停用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} maxLength={255} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}

// ---------- 模板 Tab ----------

interface TemplateSearchParams {
  keyword?: string
  channel_id?: number
  type?: number
  status?: number
  page: number
  page_size: number
}

function TemplateTab() {
  const [list, setList] = useState<SmsTemplate[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<TemplateSearchParams>({ page: 1, page_size: 10 })
  const [channels, setChannels] = useState<SmsChannel[]>([])
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SmsTemplate | null>(null)
  const [submitting, setSubmitting] = useState(false)
  // 测试发送弹窗
  const [testRecord, setTestRecord] = useState<SmsTemplate | null>(null)
  const [testSending, setTestSending] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const [testForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const channelNames = useMemo(() => {
    const map = new Map<number, string>()
    channels.forEach((c) => map.set(c.id, c.name))
    return map
  }, [channels])

  const fetchList = useCallback(async (p: TemplateSearchParams) => {
    setLoading(true)
    try {
      const res = await SmsAPI.getSmsTemplateList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取短信模板列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  useEffect(() => {
    SmsAPI.getEnabledSmsChannels()
      .then(setChannels)
      .catch(() => { /* 拦截器已提示 */ })
  }, [])

  const handleSearch = (values: { keyword?: string; channel_id?: number; type?: number; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: SmsTemplate) => {
    setEditRecord(record)
    form.setFieldsValue({
      code: record.code,
      name: record.name,
      channel_id: record.channel_id,
      content: record.content,
      type: record.type,
      provider_template_id: record.provider_template_id,
      status: record.status,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const openTest = (record: SmsTemplate) => {
    setTestRecord(record)
    // 预填参数 JSON：从模板内容提取 {xxx} 占位
    const keys = extractParams(record.content)
    const sample: Record<string, string> = {}
    keys.forEach((k) => { sample[k] = '' })
    testForm.setFieldsValue({
      mobile: '',
      params: JSON.stringify(sample, null, 2),
    })
  }

  const handleDelete = async (id: number) => {
    try {
      await SmsAPI.deleteSmsTemplate(id)
      message.success('删除成功')
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error('删除失败')
    }
  }

  const handleToggleStatus = async (record: SmsTemplate, checked: boolean) => {
    try {
      await SmsAPI.updateSmsTemplateStatus(record.id, checked ? 1 : 0)
      message.success(checked ? '已启用' : '已停用')
      fetchList(params)
    } catch {
      message.error('状态更新失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await SmsAPI.updateSmsTemplate(editRecord.id, values)
        message.success('更新成功')
      } else {
        await SmsAPI.createSmsTemplate(values)
        message.success('创建成功')
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      // 拦截器已提示（code 重复等后端返回 400）
    } finally {
      setSubmitting(false)
    }
  }

  const handleTestSend = async () => {
    const values = await testForm.validateFields().catch(() => null)
    if (!values || !testRecord) return
    let parsedParams: Record<string, string> = {}
    const raw = (values.params ?? '').trim()
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as Record<string, unknown>
        parsedParams = Object.fromEntries(
          Object.entries(parsed).map(([k, v]) => [k, String(v)]),
        )
      } catch {
        message.error('参数 JSON 格式不正确')
        return
      }
    }
    setTestSending(true)
    try {
      const result = await SmsAPI.sendSms({
        mobile: values.mobile,
        template_code: testRecord.code,
        params: parsedParams,
      })
      if (result.status === 'success') {
        message.success(`发送成功${result.provider_msg_id ? `（回执 ${result.provider_msg_id}）` : ''}`)
        setTestRecord(null)
      } else {
        message.error(`发送失败：${result.error || '未知错误'}`)
      }
    } catch {
      // 拦截器已提示（缺参/模板停用等后端返回 4xx）
    } finally {
      setTestSending(false)
    }
  }

  const columns: ColumnsType<SmsTemplate> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '模板编码', dataIndex: 'code', width: 150, ellipsis: true },
    { title: '模板名称', dataIndex: 'name', width: 140, ellipsis: true },
    {
      title: '类型',
      dataIndex: 'type',
      width: 80,
      render: (v: number) => (
        <Tag variant="filled" color={templateTypeColors[v] ?? 'default'}>{templateTypeLabels[v] ?? v}</Tag>
      ),
    },
    {
      title: '渠道',
      dataIndex: 'channel_id',
      width: 120,
      ellipsis: true,
      render: (v: number) => channelNames.get(v) ?? `#${v}`,
    },
    { title: '模板内容', dataIndex: 'content', ellipsis: true },
    { title: '云模板号', dataIndex: 'provider_template_id', width: 120, ellipsis: true, render: (v: string) => v || '-' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number, record) => (
        <Switch
          size="small"
          checked={v === 1}
          checkedChildren="启用"
          unCheckedChildren="停用"
          disabled={!hasPerm('system:sms-template:update')}
          onChange={(checked) => handleToggleStatus(record, checked)}
        />
      ),
    },
    {
      title: '操作',
      width: 210,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:sms:send') && (
            <Button type="link" size="small" icon={<SendOutlined />} onClick={() => openTest(record)}>测试</Button>
          )}
          {hasPerm('system:sms-template:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:sms-template:delete') && (
            <Popconfirm title="确认删除该模板?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Form
        form={searchForm}
        layout="inline"
        className="list-filter-form"
        onFinish={handleSearch}
        style={{ marginBottom: 16 }}
      >
        <Form.Item name="keyword">
          <Input placeholder="搜索编码 / 名称" prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
        </Form.Item>
        <Form.Item name="channel_id">
          <Select placeholder="渠道" style={{ width: 140 }} allowClear>
            {channels.map((c) => (
              <Select.Option key={c.id} value={c.id}>{c.name}</Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item name="type">
          <Select placeholder="类型" style={{ width: 100 }} allowClear>
            <Select.Option value={1}>验证码</Select.Option>
            <Select.Option value={2}>通知</Select.Option>
            <Select.Option value={3}>营销</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="状态" style={{ width: 100 }} allowClear>
            <Select.Option value={1}>启用</Select.Option>
            <Select.Option value={0}>停用</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
            <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <TableToolbar
        title="短信模板"
        total={total}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
            {hasPerm('system:sms-template:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增模板</Button>
            )}
          </Space>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        locale={{ emptyText: <GlassEmpty text="暂无短信模板" compact /> }}
        pagination={{
          total,
          current: params.page,
          pageSize: params.page_size,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
      />

      <Modal
        title={editRecord ? '编辑模板' : '新增模板'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={600}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="code"
            label="模板编码"
            rules={[{ required: true, message: '请输入模板编码' }]}
            extra="业务调用发送接口时使用，租户内唯一，如 user-register"
          >
            <Input placeholder="user-register" disabled={!!editRecord} />
          </Form.Item>
          <Form.Item name="name" label="模板名称" rules={[{ required: true, message: '请输入模板名称' }]}>
            <Input placeholder="如：注册验证码" />
          </Form.Item>
          <Form.Item name="channel_id" label="发送渠道" rules={[{ required: true, message: '请选择发送渠道' }]}>
            <Select placeholder="选择启用中的渠道">
              {channels.map((c) => (
                <Select.Option key={c.id} value={c.id}>
                  {c.name}（{providerLabels[c.provider] ?? c.provider}）
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            name="content"
            label="模板内容"
            rules={[{ required: true, message: '请输入模板内容' }]}
            extra="用 {参数名} 形式占位，如：您的验证码是 {code}，{expire} 分钟内有效"
          >
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="type" label="类型" initialValue={1}>
            <Select>
              <Select.Option value={1}>验证码</Select.Option>
              <Select.Option value={2}>通知</Select.Option>
              <Select.Option value={3}>营销</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="provider_template_id"
            label="云厂商模板号"
            extra="阿里云 TemplateCode / 腾讯云模板 ID；调试渠道可留空"
          >
            <Input placeholder="如：SMS_123456789" />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>停用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} maxLength={255} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`测试发送${testRecord ? `：${testRecord.name}` : ''}`}
        open={!!testRecord}
        onOk={handleTestSend}
        onCancel={() => setTestRecord(null)}
        confirmLoading={testSending}
        okText="发送"
        destroyOnHidden
        width={480}
      >
        <Form form={testForm} layout="vertical" style={{ marginTop: 16 }}>
          {testRecord && (
            <Form.Item label="模板内容">
              <div style={{ padding: '8px 12px', background: 'rgba(148, 163, 184, 0.1)', borderRadius: 8 }}>
                {testRecord.content}
              </div>
            </Form.Item>
          )}
          <Form.Item
            name="mobile"
            label="手机号"
            rules={[{ required: true, message: '请输入手机号' }]}
          >
            <Input placeholder="13800000000" maxLength={20} />
          </Form.Item>
          <Form.Item
            name="params"
            label="参数 JSON"
            extra="按模板占位填写，如 {&quot;code&quot;: &quot;123456&quot;}"
          >
            <Input.TextArea rows={5} placeholder="{}" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}

// ---------- 发送日志 Tab ----------

interface LogSearchParams {
  mobile?: string
  template_code?: string
  status?: string
  page: number
  page_size: number
}

const logStatusPill = (status: SmsLog['status']) => {
  switch (status) {
    case 'success':
      return <StatusPill tone="success" label="成功" />
    case 'failure':
      return <StatusPill tone="danger" label="失败" />
    default:
      return <StatusPill tone="info" label="发送中" pulse />
  }
}

function LogTab() {
  const [list, setList] = useState<SmsLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<LogSearchParams>({ page: 1, page_size: 10 })
  const [detail, setDetail] = useState<SmsLog | null>(null)
  const [searchForm] = Form.useForm()

  const fetchList = useCallback(async (p: LogSearchParams) => {
    setLoading(true)
    try {
      const res = await SmsAPI.getSmsLogList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取发送日志失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  const handleSearch = (values: { mobile?: string; template_code?: string; status?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const columns: ColumnsType<SmsLog> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '手机号', dataIndex: 'mobile', width: 130 },
    { title: '模板编码', dataIndex: 'template_code', width: 150, ellipsis: true },
    { title: '短信内容', dataIndex: 'content', ellipsis: true },
    { title: '渠道', dataIndex: 'channel_name', width: 120, ellipsis: true },
    {
      title: '服务商',
      dataIndex: 'provider',
      width: 90,
      render: (v: string) => providerLabels[v as SmsProvider] ?? v,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: SmsLog['status']) => logStatusPill(v),
    },
    { title: '发送时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setDetail(record)}>详情</Button>
      ),
    },
  ]

  return (
    <>
      <Form
        form={searchForm}
        layout="inline"
        className="list-filter-form"
        onFinish={handleSearch}
        style={{ marginBottom: 16 }}
      >
        <Form.Item name="mobile">
          <Input placeholder="手机号" prefix={<SearchOutlined />} allowClear style={{ width: 170 }} />
        </Form.Item>
        <Form.Item name="template_code">
          <Input placeholder="模板编码" allowClear style={{ width: 170 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="状态" style={{ width: 110 }} allowClear>
            <Select.Option value="sending">发送中</Select.Option>
            <Select.Option value="success">成功</Select.Option>
            <Select.Option value="failure">失败</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
            <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <TableToolbar
        title="发送日志"
        total={total}
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        locale={{ emptyText: <GlassEmpty text="暂无发送日志" compact /> }}
        pagination={{
          total,
          current: params.page,
          pageSize: params.page_size,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
      />

      <Modal
        title="发送详情"
        open={!!detail}
        onCancel={() => setDetail(null)}
        footer={null}
        destroyOnHidden
        width={560}
      >
        {detail && (
          <Descriptions column={1} size="small" bordered style={{ marginTop: 16 }}>
            <Descriptions.Item label="手机号">{detail.mobile}</Descriptions.Item>
            <Descriptions.Item label="模板编码">{detail.template_code}</Descriptions.Item>
            <Descriptions.Item label="短信内容">{detail.content}</Descriptions.Item>
            <Descriptions.Item label="参数">
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {JSON.stringify(detail.params ?? {}, null, 2)}
              </pre>
            </Descriptions.Item>
            <Descriptions.Item label="渠道">
              {detail.channel_name}（{providerLabels[detail.provider as SmsProvider] ?? detail.provider}）
            </Descriptions.Item>
            <Descriptions.Item label="状态">{logStatusPill(detail.status)}</Descriptions.Item>
            {detail.provider_msg_id && (
              <Descriptions.Item label="厂商回执">{detail.provider_msg_id}</Descriptions.Item>
            )}
            {detail.error && (
              <Descriptions.Item label="错误信息">{detail.error}</Descriptions.Item>
            )}
            <Descriptions.Item label="发送时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </>
  )
}

// ---------- 页面入口：一页三 Tab ----------

export default function SmsPage() {
  return (
    <div className="page-list sms-page">
      <Card className="list-main-card" bordered={false}>
        <Tabs
          defaultActiveKey="channel"
          items={[
            { key: 'channel', label: '短信渠道', children: <ChannelTab /> },
            { key: 'template', label: '短信模板', children: <TemplateTab /> },
            { key: 'log', label: '发送日志', children: <LogTab /> },
          ]}
        />
      </Card>
    </div>
  )
}
