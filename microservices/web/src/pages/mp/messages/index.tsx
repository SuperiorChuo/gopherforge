import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Form, Input, Modal, Segmented, Select, Space, Table, Tag, Tooltip, Typography, Upload } from 'antd'
import { MessageOutlined, PictureOutlined, ReloadOutlined, SearchOutlined, SendOutlined, UploadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  listMpAccounts, listMpMessages, MP_DIRECTION_META, replyMpMessage, uploadMpCustomerImage,
  type MpAccount, type MpMessage,
} from '@/api/mp'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import './index.css'

const { Text } = Typography

interface SearchParams {
  account_id?: number
  open_id?: string
  direction?: string
  page: number
  page_size: number
}

interface ReplyFormValues {
  msg_type: 'text' | 'image'
  content?: string
  media_id?: string
}

export default function MpMessagesPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('social:write')
  const [accounts, setAccounts] = useState<MpAccount[]>([])
  const [replyTarget, setReplyTarget] = useState<MpMessage>()
  const [replying, setReplying] = useState(false)
  const [mediaUploading, setMediaUploading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>(
    { page: 1, page_size: 10 },
    ['page', 'page_size', 'account_id'],
  )
  const [searchForm] = Form.useForm()
  const [replyForm] = Form.useForm<ReplyFormValues>()
  const replyType = Form.useWatch('msg_type', replyForm) ?? 'text'
  const replyTargetID = useRef<number | undefined>(undefined)
  const replySequence = useRef(0)
  const uploadSequence = useRef(0)
  const replyBusy = replying || mediaUploading

  const accountByID = useMemo(
    () => new Map(accounts.map((account) => [account.id, account])),
    [accounts],
  )

  useEffect(() => {
    listMpAccounts({ page: 1, page_size: 100 })
      .then((res) => setAccounts(res.list ?? []))
      .catch(() => {
        setAccounts([])
        message.error(t('获取公众号账号失败'))
      })
  }, [t])

  const fetchList = useCallback((p: SearchParams) => listMpMessages({ ...p }), [])
  const onLoadError = useCallback(() => message.error(t('获取消息记录失败')), [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  const openReply = (row: MpMessage) => {
    replySequence.current += 1
    uploadSequence.current += 1
    replyTargetID.current = row.id
    setReplyTarget(row)
  }

  const clearReply = () => {
    replySequence.current += 1
    uploadSequence.current += 1
    replyTargetID.current = undefined
    setReplying(false)
    setMediaUploading(false)
    setReplyTarget(undefined)
    replyForm.resetFields()
  }

  useEffect(() => {
    if (!replyTarget) return
    replyForm.resetFields()
    replyForm.setFieldsValue({ msg_type: 'text', content: '', media_id: '' })
  }, [replyForm, replyTarget])

  const submitReply = async () => {
    if (!replyTarget) return
    const targetID = replyTarget.id
    const sequence = ++replySequence.current
    const values = await replyForm.validateFields().catch(() => undefined)
    if (!values || replyTargetID.current !== targetID || replySequence.current !== sequence) return
    setReplying(true)
    try {
      const result = await replyMpMessage(targetID, values)
      if (replyTargetID.current !== targetID || replySequence.current !== sequence) return
      if (result.archived) {
        message.success(values.msg_type === 'image' ? t('图片客服消息已发送') : t('客服回复已发送'))
      } else {
        message.warning(t('消息已发送，但留档失败，请勿重复发送'))
      }
      clearReply()
      await reload()
    } catch {
      // 全局请求层会展示服务端返回的具体错误。
    } finally {
      if (replyTargetID.current === targetID && replySequence.current === sequence) setReplying(false)
    }
  }

  const uploadReplyImage = async (file: File) => {
    if (!replyTarget) return
    if (file.size > 10 * 1024 * 1024) {
      message.error(t('图片大小不能超过 10 MB'))
      return
    }
    const targetID = replyTarget.id
    const accountID = replyTarget.account_id
    const sequence = ++uploadSequence.current
    setMediaUploading(true)
    try {
      const result = await uploadMpCustomerImage(accountID, file)
      if (replyTargetID.current !== targetID || uploadSequence.current !== sequence) return
      replyForm.setFieldValue('media_id', result.media_id)
      message.success(t('图片已上传到微信临时素材库'))
    } catch {
      // 全局请求层会展示服务端返回的具体错误。
    } finally {
      if (replyTargetID.current === targetID && uploadSequence.current === sequence) setMediaUploading(false)
    }
  }

  const columns: ColumnsType<MpMessage> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('公众号'), dataIndex: 'account_id', width: 150,
      render: (id: number) => accountByID.get(id)?.name || <span className="cell-mono">#{id}</span>,
    },
    {
      title: t('方向'), dataIndex: 'direction', width: 90,
      render: (v: string) => {
        const meta = MP_DIRECTION_META[v]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} pulse={false} /> : <span className="cell-muted">{v || '—'}</span>
      },
    },
    { title: t('类型'), dataIndex: 'msg_type', width: 100, render: (v: string) => <Tag variant="filled">{v || '—'}</Tag> },
    { title: 'OpenID', dataIndex: 'open_id', width: 220, render: (v: string) => <span className="cell-mono">{v || '—'}</span> },
    {
      title: t('内容'), dataIndex: 'content', ellipsis: true,
      render: (v: string, row) => {
        const text = v || row.event
        return text ? <Text>{text}</Text> : <span className="cell-muted">—</span>
      },
    },
    { title: t('时间'), dataIndex: 'created_at', width: 165, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'), key: 'action', width: 78, fixed: 'right',
      render: (_, row) => {
        if (row.direction !== 'in') return <span className="cell-muted">—</span>
        const account = accountByID.get(row.account_id)
        const unavailableReason = !canWrite
          ? t('缺少社媒内容操作权限')
          : !row.open_id
          ? t('该消息没有 OpenID')
            : !account
            ? t('公众号账号已删除')
            : account.status !== 'active'
              ? t('公众号账号已停用')
            : account.type !== 'service'
              ? t('客服消息仅服务号可用')
              : undefined
        return (
          <Tooltip title={unavailableReason || t('在 48 小时客服窗口内回复')}>
            <Button
              type="text"
              icon={<MessageOutlined />}
              disabled={Boolean(unavailableReason)}
              onClick={() => openReply(row)}
            >
              {t('回复')}
            </Button>
          </Tooltip>
        )
      },
    },
  ]

  return (
    <ListPageShell
      className="mp-messages-page"
      filter={
        <ListFilterForm
          form={searchForm}
          initialValues={params}
          onFinish={(values) => setParams({ ...params, page: 1, ...values })}
        >
          <Form.Item name="account_id">
            <Select
              placeholder={t('全部账号')}
              style={{ width: 200 }}
              allowClear
              showSearch
              optionFilterProp="label"
              options={accounts.map((a) => ({ value: a.id, label: a.name || a.app_id }))}
            />
          </Form.Item>
          <Form.Item name="direction">
            <Select
              placeholder={t('方向')}
              style={{ width: 120 }}
              allowClear
              options={Object.entries(MP_DIRECTION_META).map(([value, m]) => ({ value, label: t(m.label) }))}
            />
          </Form.Item>
          <Form.Item name="open_id">
            <Input placeholder="OpenID" prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.resetFields(); setParams({ page: 1, page_size: 10 }) }}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      }
      toolbar={
        <TableToolbar title="消息记录" total={total}
          extra={<Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>} />}
    >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无消息记录" compact /> }}
          scroll={{ x: 1150 }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />

      <Modal
        title={<Space><SendOutlined />{t('客服回复')}</Space>}
        open={Boolean(replyTarget)}
        okText={t('发送')}
        cancelText={t('取消')}
        confirmLoading={replying}
        okButtonProps={{ icon: <SendOutlined />, disabled: mediaUploading }}
        cancelButtonProps={{ disabled: replyBusy }}
        closable={!replyBusy}
        mask={{ closable: !replyBusy }}
        keyboard={!replyBusy}
        onOk={() => void submitReply()}
        onCancel={() => {
          if (!replyBusy) clearReply()
        }}
        destroyOnHidden
      >
        {replyTarget && (
          <>
            <div className="mp-message-reply-context">
              <div>
                <Text type="secondary">OpenID</Text>
                <Text className="cell-mono" copyable>{replyTarget.open_id}</Text>
              </div>
              <div>
                <Text type="secondary">{t('收到时间')}</Text>
                <Text>{formatDateTime(replyTarget.created_at)}</Text>
              </div>
              <div className="mp-message-reply-source">
                <Text type="secondary">{t('原消息')}</Text>
                <Text>{replyTarget.content || replyTarget.event || '—'}</Text>
              </div>
            </div>
            <Alert
              className="mp-message-window-alert"
              type="info"
              showIcon
              title={t('当前回复受微信 48 小时客服窗口限制')}
            />
            <Form form={replyForm} layout="vertical" initialValues={{ msg_type: 'text' }} disabled={replyBusy}>
              <Form.Item name="msg_type" label={t('消息类型')}>
                <Segmented
                  block
                  options={[
                    { label: t('文本'), value: 'text', icon: <MessageOutlined /> },
                    { label: t('图片'), value: 'image', icon: <PictureOutlined /> },
                  ]}
                />
              </Form.Item>
              {replyType === 'image' ? (
                <>
                  <Form.Item
                    name="media_id"
                    label={t('图片素材')}
                    rules={[{ required: true, whitespace: true, message: t('请上传图片或填写 Media ID') }]}
                  >
                    <Input placeholder={t('微信临时素材 Media ID')} maxLength={128} />
                  </Form.Item>
                  <Upload
                    accept="image/jpeg,image/png,image/gif,image/bmp"
                    maxCount={1}
                    showUploadList={false}
                    beforeUpload={(file) => {
                      void uploadReplyImage(file as File)
                      return Upload.LIST_IGNORE
                    }}
                  >
                    <Button block icon={<UploadOutlined />} loading={mediaUploading} disabled={replying}>{t('上传图片')}</Button>
                  </Upload>
                </>
              ) : (
                <Form.Item
                  name="content"
                  label={t('回复内容')}
                  rules={[
                    { required: true, whitespace: true, message: t('请输入回复内容') },
                    {
                      validator: async (_, value?: string) => {
                        if (!value || new TextEncoder().encode(value.trim()).length <= 2048) return
                        throw new Error(t('回复内容不能超过 2048 字节'))
                      },
                    },
                  ]}
                >
                  <Input.TextArea rows={5} showCount maxLength={2048} placeholder={t('输入客服回复内容')} />
                </Form.Item>
              )}
            </Form>
          </>
        )}
      </Modal>
    </ListPageShell>
  )
}
