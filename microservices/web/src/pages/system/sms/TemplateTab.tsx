import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Tag, Modal, Form, Input, Select, Switch, Grid } from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined, SendOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as SmsAPI from '@/api/system/sms'
import type { SmsChannel, SmsTemplate } from '@/api/system/sms'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useCrudModal } from '@/hooks/useCrudModal'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { message } from '@/utils/feedback'
import { extractParams, providerLabels, templateTypeColors, templateTypeLabels } from './smsMeta'

interface TemplateSearchParams {
  keyword?: string
  channel_id?: number
  type?: number
  status?: number
  page: number
  page_size: number
}

export default function TemplateTab() {
  const { t } = useTranslation()
  const [params, setParams] = useState<TemplateSearchParams>({ page: 1, page_size: 10 })
  const [channels, setChannels] = useState<SmsChannel[]>([])
  const editModal = useCrudModal<SmsTemplate>()
  const [submitting, setSubmitting] = useState(false)
  // 测试发送弹窗
  const [testRecord, setTestRecord] = useState<SmsTemplate | null>(null)
  const [testSending, setTestSending] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const [testForm] = Form.useForm()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const channelNames = useMemo(() => {
    const map = new Map<number, string>()
    channels.forEach((c) => map.set(c.id, c.name))
    return map
  }, [channels])

  const fetchList = useCallback(async (p: TemplateSearchParams) => {
    const res = await SmsAPI.getSmsTemplateList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => {
    message.error(t('获取短信模板列表失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

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
    form.resetFields()
    editModal.openCreate()
  }

  const openEdit = (record: SmsTemplate) => {
    editModal.openEdit(record)
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
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        void reload()
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleToggleStatus = async (record: SmsTemplate, checked: boolean) => {
    try {
      await SmsAPI.updateSmsTemplateStatus(record.id, checked ? 1 : 0)
      message.success(checked ? t('已启用') : t('已停用'))
      void reload()
    } catch {
      message.error(t('状态更新失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editModal.record) {
        await SmsAPI.updateSmsTemplate(editModal.record.id, values)
        message.success(t('更新成功'))
      } else {
        await SmsAPI.createSmsTemplate(values)
        message.success(t('创建成功'))
      }
      editModal.close()
      void reload()
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
        message.error(t('参数 JSON 格式不正确'))
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
        message.success(result.provider_msg_id ? t('发送成功（回执 {{id}}）', { id: result.provider_msg_id }) : t('发送成功'))
        setTestRecord(null)
      } else {
        message.error(t('发送失败：{{error}}', { error: result.error || t('未知错误') }))
      }
    } catch {
      // 拦截器已提示（缺参/模板停用等后端返回 4xx）
    } finally {
      setTestSending(false)
    }
  }

  const columns: ColumnsType<SmsTemplate> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: t('模板编码'), dataIndex: 'code', width: 150, ellipsis: true },
    { title: t('模板名称'), dataIndex: 'name', width: 140, ellipsis: true },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 80,
      render: (v: number) => (
        <Tag variant="filled" color={templateTypeColors[v] ?? 'default'}>{templateTypeLabels[v] ? t(templateTypeLabels[v] ?? '') : v}</Tag>
      ),
    },
    {
      title: t('渠道'),
      dataIndex: 'channel_id',
      width: 120,
      ellipsis: true,
      render: (v: number) => channelNames.get(v) ?? `#${v}`,
    },
    { title: t('模板内容'), dataIndex: 'content', ellipsis: true },
    { title: t('云模板号'), dataIndex: 'provider_template_id', width: 120, ellipsis: true, render: (v: string) => v || '-' },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (v: number, record) => (
        <Switch
          size="small"
          checked={v === 1}
          checkedChildren={t('启用')}
          unCheckedChildren={t('停用')}
          disabled={!hasPerm('system:sms-template:update')}
          onChange={(checked) => handleToggleStatus(record, checked)}
        />
      ),
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 120,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.name || record.code })}
          actions={[
            {
              key: 'test',
              label: t('测试'),
              icon: <SendOutlined />,
              show: hasPerm('system:sms:send'),
              onClick: () => openTest(record),
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:sms-template:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:sms-template:delete'),
              confirm: t('确认删除该模板?'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <ListFilterForm
        form={searchForm}
        onFinish={handleSearch}
        style={{ marginBottom: 0 }}
      >
        <Form.Item name="keyword">
          <Input placeholder={t('搜索编码 / 名称')} prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
        </Form.Item>
        <Form.Item name="channel_id">
          <Select placeholder={t('渠道')} style={{ width: 140 }} allowClear>
            {channels.map((c) => (
              <Select.Option key={c.id} value={c.id}>{c.name}</Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item name="type">
          <Select placeholder={t('类型')} style={{ width: 100 }} allowClear>
            <Select.Option value={1}>{t('验证码')}</Select.Option>
            <Select.Option value={2}>{t('通知')}</Select.Option>
            <Select.Option value={3}>{t('营销')}</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
            <Select.Option value={1}>{t('启用')}</Select.Option>
            <Select.Option value={0}>{t('停用')}</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
            <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
          </Space>
        </Form.Item>
      </ListFilterForm>

      <TableToolbar
        title="短信模板"
        total={total}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            {hasPerm('system:sms-template:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增模板')}</Button>
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
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
       scroll={{ x: 960 }} />

      <EntityFormModal
        title={editModal.record ? t('编辑模板') : t('新增模板')}
        open={editModal.open}
        form={form}
        onClose={editModal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        width={600}
      >
        <>
          <Form.Item
            name="code"
            label={t('模板编码')}
            rules={[{ required: true, message: t('请输入模板编码') }]}
            extra={t('业务调用发送接口时使用，租户内唯一，如 user-register')}
          >
            <Input placeholder="user-register" disabled={!!editModal.record} />
          </Form.Item>
          <Form.Item name="name" label={t('模板名称')} rules={[{ required: true, message: t('请输入模板名称') }]}>
            <Input placeholder={t('如：注册验证码')} />
          </Form.Item>
          <Form.Item name="channel_id" label={t('发送渠道')} rules={[{ required: true, message: t('请选择发送渠道') }]}>
            <Select placeholder={t('选择启用中的渠道')}>
              {channels.map((c) => (
                <Select.Option key={c.id} value={c.id}>
                  {c.name}（{providerLabels[c.provider] ? t(providerLabels[c.provider] ?? '') : c.provider}）
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            name="content"
            label={t('模板内容')}
            rules={[{ required: true, message: t('请输入模板内容') }]}
            extra={t('用 {参数名} 形式占位，如：您的验证码是 {code}，{expire} 分钟内有效')}
          >
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="type" label={t('类型')} initialValue={1}>
            <Select>
              <Select.Option value={1}>{t('验证码')}</Select.Option>
              <Select.Option value={2}>{t('通知')}</Select.Option>
              <Select.Option value={3}>{t('营销')}</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="provider_template_id"
            label={t('云厂商模板号')}
            extra={t('阿里云 TemplateCode / 腾讯云模板 ID；调试渠道可留空')}
          >
            <Input placeholder={t('如：SMS_123456789')} />
          </Form.Item>
          <Form.Item name="status" label={t('状态')} initialValue={1}>
            <Select>
              <Select.Option value={1}>{t('启用')}</Select.Option>
              <Select.Option value={0}>{t('停用')}</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="remark" label={t('备注')}>
            <Input.TextArea rows={2} maxLength={255} />
          </Form.Item>
        </>
      </EntityFormModal>

      <Modal
        title={testRecord ? t('测试发送：{{name}}', { name: testRecord.name }) : t('测试发送')}
        open={!!testRecord}
        onOk={handleTestSend}
        onCancel={() => setTestRecord(null)}
        confirmLoading={testSending}
        okText={t('发送')}
        destroyOnHidden
        width={480}
      >
        <Form form={testForm} layout="vertical" style={{ marginTop: 16 }}>
          {testRecord && (
            <Form.Item label={t('模板内容')}>
              <div style={{ padding: '8px 12px', background: 'rgba(148, 163, 184, 0.1)', borderRadius: 8 }}>
                {testRecord.content}
              </div>
            </Form.Item>
          )}
          <Form.Item
            name="mobile"
            label={t('手机号')}
            rules={[{ required: true, message: t('请输入手机号') }]}
          >
            <Input placeholder="13800000000" maxLength={20} />
          </Form.Item>
          <Form.Item
            name="params"
            label={t('参数 JSON')}
            extra={t('按模板占位填写，如 {&quot;code&quot;: &quot;123456&quot;}')}
          >
            <Input.TextArea rows={5} placeholder="{}" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
