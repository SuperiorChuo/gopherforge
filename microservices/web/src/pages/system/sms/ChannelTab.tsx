import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Tag, Form, Input, Select, Switch, Grid } from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as SmsAPI from '@/api/system/sms'
import type { SmsChannel, SmsProvider, SmsChannelConfig } from '@/api/system/sms'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useCrudModal } from '@/hooks/useCrudModal'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { providerColors, providerLabels } from './smsMeta'

interface ChannelSearchParams {
  keyword?: string
  provider?: string
  status?: number
  page: number
  page_size: number
}

export default function ChannelTab() {
  const { t } = useTranslation()
  const [params, setParams] = useState<ChannelSearchParams>({ page: 1, page_size: 10 })
  const editModal = useCrudModal<SmsChannel>()
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const provider = Form.useWatch<SmsProvider | undefined>('provider', form)

  const fetchList = useCallback(async (p: ChannelSearchParams) => {
    const res = await SmsAPI.getSmsChannelList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => {
    message.error(t('获取短信渠道列表失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  const handleSearch = (values: { keyword?: string; provider?: string; status?: number }) => {
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

  const openEdit = (record: SmsChannel) => {
    editModal.openEdit(record)
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
  }

  const handleDelete = async (id: number) => {
    try {
      await SmsAPI.deleteSmsChannel(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        void reload()
      }
    } catch {
      // 拦截器已提示（被模板引用时后端返回 400）
    }
  }

  const handleToggleStatus = async (record: SmsChannel, checked: boolean) => {
    try {
      await SmsAPI.updateSmsChannelStatus(record.id, checked ? 1 : 0)
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
      if (editModal.record) {
        await SmsAPI.updateSmsChannel(editModal.record.id, payload)
        message.success(t('更新成功'))
      } else {
        await SmsAPI.createSmsChannel(payload)
        message.success(t('创建成功'))
      }
      editModal.close()
      void reload()
    } catch {
      // 拦截器已提示
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<SmsChannel> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: t('渠道名称'), dataIndex: 'name', ellipsis: true },
    {
      title: t('服务商'),
      dataIndex: 'provider',
      width: 90,
      render: (v: SmsProvider) => (
        <Tag variant="filled" color={providerColors[v] ?? 'default'}>{providerLabels[v] ? t(providerLabels[v] ?? '') : v}</Tag>
      ),
    },
    {
      title: t('签名'),
      dataIndex: 'config',
      width: 140,
      ellipsis: true,
      render: (config?: SmsChannelConfig | null) => config?.sign_name || '-',
    },
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
          disabled={!hasPerm('system:sms-channel:update')}
          onChange={(checked) => handleToggleStatus(record, checked)}
        />
      ),
    },
    { title: t('备注'), dataIndex: 'remark', ellipsis: true },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: compactActions ? 48 : 96,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:sms-channel:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:sms-channel:delete'),
              confirm: t('确认删除该渠道?'),
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
          <Input placeholder={t('搜索渠道名称')} prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="provider">
          <Select placeholder={t('服务商')} style={{ width: 110 }} allowClear>
            <Select.Option value="debug">{t('调试')}</Select.Option>
            <Select.Option value="aliyun">{t('阿里云')}</Select.Option>
            <Select.Option value="tencent">{t('腾讯云')}</Select.Option>
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
        title="短信渠道"
        total={total}
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            {hasPerm('system:sms-channel:create') && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增渠道')}</Button>
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
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
       scroll={{ x: 960 }} />

      <EntityFormModal
        title={editModal.record ? t('编辑渠道') : t('新增渠道')}
        open={editModal.open}
        form={form}
        onClose={editModal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        width={560}
      >
        <>
          <Form.Item name="name" label={t('渠道名称')} rules={[{ required: true, message: t('请输入渠道名称') }]}>
            <Input placeholder={t('如：阿里云主渠道')} />
          </Form.Item>
          <Form.Item name="provider" label={t('服务商')} initialValue="debug" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="debug">{t('调试（不真实外发，直接成功）')}</Select.Option>
              <Select.Option value="aliyun">{t('阿里云')}</Select.Option>
              <Select.Option value="tencent">{t('腾讯云')}</Select.Option>
            </Select>
          </Form.Item>
          {provider !== 'debug' && (
            <Form.Item
              name="config_sign_name"
              label={t('短信签名')}
              rules={[{ required: true, message: t('请输入云厂商审核通过的短信签名') }]}
            >
              <Input placeholder={t('如：某某科技')} />
            </Form.Item>
          )}
          {provider === 'aliyun' && (
            <>
              <Form.Item name="config_access_key_id" label="AccessKey ID" rules={[{ required: true, message: t('请输入 AccessKey ID') }]}>
                <Input placeholder="LTAI****************" />
              </Form.Item>
              <Form.Item
                name="config_access_key_secret"
                label="AccessKey Secret"
                rules={editModal.record ? [] : [{ required: true, message: t('请输入 AccessKey Secret') }]}
                extra={editModal.record ? t('留空或保持 ****** 表示不修改') : undefined}
              >
                <Input.Password placeholder={t('密钥只保存在服务端')} autoComplete="new-password" />
              </Form.Item>
              <Form.Item name="config_region_id" label={t('地域（可选）')}>
                <Input placeholder={t('默认 cn-hangzhou')} />
              </Form.Item>
            </>
          )}
          {provider === 'tencent' && (
            <>
              <Form.Item name="config_secret_id" label="SecretId" rules={[{ required: true, message: t('请输入 SecretId') }]}>
                <Input placeholder="AKID****************" />
              </Form.Item>
              <Form.Item
                name="config_secret_key"
                label="SecretKey"
                rules={editModal.record ? [] : [{ required: true, message: t('请输入 SecretKey') }]}
                extra={editModal.record ? t('留空或保持 ****** 表示不修改') : undefined}
              >
                <Input.Password placeholder={t('密钥只保存在服务端')} autoComplete="new-password" />
              </Form.Item>
              <Form.Item name="config_sdk_app_id" label="SdkAppId" rules={[{ required: true, message: t('请输入短信应用 SdkAppId') }]}>
                <Input placeholder="1400******" />
              </Form.Item>
              <Form.Item name="config_region" label={t('地域（可选）')}>
                <Input placeholder={t('默认 ap-guangzhou')} />
              </Form.Item>
            </>
          )}
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
    </>
  )
}
