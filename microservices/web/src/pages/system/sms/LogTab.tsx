import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Modal, Form, Input, Select, Descriptions } from 'antd'
import { SearchOutlined, ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as SmsAPI from '@/api/system/sms'
import type { SmsLog, SmsProvider } from '@/api/system/sms'
import ListFilterForm from '@/components/common/ListFilterForm'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { providerLabels } from './smsMeta'

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

export default function LogTab() {
  const { t } = useTranslation()
  const [params, setParams] = useState<LogSearchParams>({ page: 1, page_size: 10 })
  const [detail, setDetail] = useState<SmsLog | null>(null)
  const [searchForm] = Form.useForm()

  const fetchList = useCallback(async (p: LogSearchParams) => {
    const res = await SmsAPI.getSmsLogList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => {
    message.error(t('获取发送日志失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  const handleSearch = (values: { mobile?: string; template_code?: string; status?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const columns: ColumnsType<SmsLog> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: t('手机号'), dataIndex: 'mobile', width: 130 },
    { title: t('模板编码'), dataIndex: 'template_code', width: 150, ellipsis: true },
    { title: t('短信内容'), dataIndex: 'content', ellipsis: true },
    { title: t('渠道'), dataIndex: 'channel_name', width: 120, ellipsis: true },
    {
      title: t('服务商'),
      dataIndex: 'provider',
      width: 90,
      render: (v: string) => providerLabels[v as SmsProvider] ? t(providerLabels[v as SmsProvider] ?? '') : v,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (v: SmsLog['status']) => logStatusPill(v),
    },
    { title: t('发送时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: 48,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'detail',
              label: t('详情'),
              icon: <EyeOutlined />,
              onClick: () => setDetail(record),
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
        <Form.Item name="mobile">
          <Input placeholder={t('手机号')} prefix={<SearchOutlined />} allowClear style={{ width: 170 }} />
        </Form.Item>
        <Form.Item name="template_code">
          <Input placeholder={t('模板编码')} allowClear style={{ width: 170 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder={t('状态')} style={{ width: 110 }} allowClear>
            <Select.Option value="sending">{t('发送中')}</Select.Option>
            <Select.Option value="success">{t('成功')}</Select.Option>
            <Select.Option value="failure">{t('失败')}</Select.Option>
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
        title="发送日志"
        total={total}
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
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
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
       scroll={{ x: 960 }} />

      <Modal
        title={t('发送详情')}
        open={!!detail}
        onCancel={() => setDetail(null)}
        footer={null}
        destroyOnHidden
        width={560}
      >
        {detail && (
          <Descriptions column={1} size="small" bordered style={{ marginTop: 16 }}>
            <Descriptions.Item label={t('手机号')}>{detail.mobile}</Descriptions.Item>
            <Descriptions.Item label={t('模板编码')}>{detail.template_code}</Descriptions.Item>
            <Descriptions.Item label={t('短信内容')}>{detail.content}</Descriptions.Item>
            <Descriptions.Item label={t('参数')}>
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {JSON.stringify(detail.params ?? {}, null, 2)}
              </pre>
            </Descriptions.Item>
            <Descriptions.Item label={t('渠道')}>
              {detail.channel_name}（{providerLabels[detail.provider as SmsProvider] ? t(providerLabels[detail.provider as SmsProvider] ?? '') : detail.provider}）
            </Descriptions.Item>
            <Descriptions.Item label={t('状态')}>{logStatusPill(detail.status)}</Descriptions.Item>
            {detail.provider_msg_id && (
              <Descriptions.Item label={t('厂商回执')}>{detail.provider_msg_id}</Descriptions.Item>
            )}
            {detail.error && (
              <Descriptions.Item label={t('错误信息')}>{detail.error}</Descriptions.Item>
            )}
            <Descriptions.Item label={t('发送时间')}>{formatDateTime(detail.created_at)}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </>
  )
}
