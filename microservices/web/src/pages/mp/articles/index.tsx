import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Table, Tag, Space, Modal, Form, Input, Select, Drawer, Image, Spin, Typography, Popconfirm,
} from 'antd'
import { EditOutlined, RobotOutlined, PictureOutlined, SendOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import {
  listMpAccounts, listMpArticles, generateMpArticle, updateMpArticle, deleteMpArticle,
  generateMpArticleCover, publishMpArticle,
  MP_ARTICLE_STATUS_META, type MpAccount, type MpArticle,
} from '@/api/mp'

export default function MpArticlesPage() {
  const { t } = useTranslation()
  const [accounts, setAccounts] = useState<MpAccount[]>([])
  const [accountId, setAccountId] = useState<number | undefined>()
  const [genOpen, setGenOpen] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [genForm] = Form.useForm()
  const [editing, setEditing] = useState<MpArticle | null>(null)
  const [editForm] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [coverLoading, setCoverLoading] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const writeInFlightRef = useRef(false)
  const operationBusy = saving || coverLoading || publishing

  useEffect(() => {
    listMpAccounts({ page: 1, page_size: 100 })
      .then((res) => {
        setAccounts(res.list || [])
        if (res.list?.length) setAccountId((prev) => prev ?? res.list[0].id)
      })
      .catch(() => setAccounts([]))
  }, [])

  const fetcher = useCallback(async (p: { accountId?: number }) => {
    const res = await listMpArticles({ account_id: p.accountId, page: 1, page_size: 50 })
    const list = res.list || []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: { accountId },
    fetcher,
    onError: useCallback(() => message.error(t('加载文章失败')), [t]),
  })

  const onGenerate = async () => {
    const values = await genForm.validateFields()
    if (!accountId) { message.warning(t('请先选择公众号账号')); return }
    setGenerating(true)
    try {
      const art = await generateMpArticle({ account_id: accountId, prompt: values.prompt })
      message.success(t('AI 文章已生成'))
      setGenOpen(false)
      genForm.resetFields()
      void reload()
      openEdit(art)
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('AI 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  const openEdit = (a: MpArticle) => {
    setEditing(a)
    editForm.setFieldsValue({
      title: a.title, author: a.author, digest: a.digest,
      content_html: a.content_html, cover_url: a.cover_url,
    })
  }

  const onSaveEdit = async () => {
    if (!editing || writeInFlightRef.current) return
    writeInFlightRef.current = true
    setSaving(true)
    try {
      const v = await editForm.validateFields()
      const updated = await updateMpArticle(editing.id, v)
      message.success(t('已保存'))
      setEditing(updated)
      void reload()
    } catch (e) {
      if (e && typeof e === 'object' && 'errorFields' in e) return
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      writeInFlightRef.current = false
      setSaving(false)
    }
  }

  const onGenCover = async () => {
    if (!editing || writeInFlightRef.current) return
    writeInFlightRef.current = true
    setCoverLoading(true)
    try {
      const { cover_url } = await generateMpArticleCover(editing.id, { prompt: editForm.getFieldValue('cover_prompt') })
      editForm.setFieldValue('cover_url', cover_url)
      setEditing({ ...editing, cover_url })
      message.success(t('封面已生成'))
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('封面生成失败'))
    } finally {
      writeInFlightRef.current = false
      setCoverLoading(false)
    }
  }

  const onPublish = async () => {
    if (!editing || writeInFlightRef.current) return
    writeInFlightRef.current = true
    setPublishing(true)
    try {
      const values = await editForm.validateFields()
      await updateMpArticle(editing.id, values)
      const updated = await publishMpArticle(editing.id)
      message.success(updated.status === 'publishing' ? t('已提交发布，等待微信回调确认') : t('已存入公众号草稿箱'))
      setEditing(updated)
      void reload()
    } catch (e) {
      if (e && typeof e === 'object' && 'errorFields' in e) return
      message.error(e instanceof Error ? e.message : t('发布失败'))
    } finally {
      writeInFlightRef.current = false
      setPublishing(false)
    }
  }

  const columns: ColumnsType<MpArticle> = [
    { title: t('标题'), dataIndex: 'title', render: (title: string) => title || <Typography.Text type="secondary">{t('（未命名）')}</Typography.Text> },
    { title: t('摘要'), dataIndex: 'digest', ellipsis: true, responsive: ['lg'] },
    {
      title: t('状态'), dataIndex: 'status', width: 110,
      render: (s: string) => {
        const m = MP_ARTICLE_STATUS_META[s] || { label: s, color: 'default' }
        return <Tag color={m.color}>{t(m.label)}</Tag>
      },
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 180, responsive: ['lg'] },
    {
      title: t('操作'), width: 160, fixed: 'right',
      render: (_: unknown, r: MpArticle) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>{t('编辑')}</Button>
          <Popconfirm title={t('删除该文章？')} onConfirm={() => deleteMpArticle(r.id).then(() => { message.success(t('已删除')); void reload() })}>
            <Button size="small" danger>{t('删除')}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <ListPageShell
      className="mp-articles-page"
      toolbar={
        <TableToolbar
          title="AI 文章"
          icon={<EditOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.4)"
          description="AI 生成图文 → 生成封面 → 编辑 → 草稿箱 / 自动发布"
          extra={
            <Space wrap>
              <Select
                style={{ minWidth: 180 }}
                placeholder={t('选择公众号')}
                value={accountId}
                onChange={setAccountId}
                options={accounts.map((a) => ({ label: a.name || a.app_id, value: a.id }))}
              />
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setGenOpen(true)} disabled={!accountId}>
                {t('AI 生成文章')}
              </Button>
            </Space>
          }
        />}
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={list}
          scroll={{ x: 720 }}
          locale={{ emptyText: <GlassEmpty text="还没有文章，点右上角「AI 生成文章」开始" /> }}
          pagination={{ pageSize: 10 }}
        />

      <Modal
        title={<span><RobotOutlined /> {t('AI 生成公众号文章')}</span>}
        open={genOpen}
        onOk={onGenerate}
        confirmLoading={generating}
        onCancel={() => setGenOpen(false)}
        okText={t('生成')}
        destroyOnClose
      >
        <Form form={genForm} layout="vertical">
          <Form.Item name="prompt" label={t('选题 / 写作要求')} rules={[{ required: true, message: t('请输入选题或要求') }]}>
            <Input.TextArea rows={4} placeholder={t('例如：写一篇介绍秋季养生茶饮的公众号文章，风格轻松，约500字')} />
          </Form.Item>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
            {t('AI 会返回标题、摘要与正文，生成后可在编辑页调整、生成封面再发布。')}
          </Typography.Paragraph>
        </Form>
      </Modal>

      <Drawer
        title={t('编辑图文')}
        width={680}
        open={!!editing}
        closable={!operationBusy}
        maskClosable={!operationBusy}
        onClose={() => {
          if (!writeInFlightRef.current) setEditing(null)
        }}
        extra={
          <Space>
            <Button onClick={onSaveEdit} loading={saving} disabled={operationBusy}>{t('保存')}</Button>
            <Button
              type="primary"
              icon={<SendOutlined />}
              loading={publishing}
              disabled={operationBusy}
              onClick={onPublish}
            >
              {t('发布 / 存草稿箱')}
            </Button>
          </Space>
        }
      >
        {editing && (
          <Form form={editForm} layout="vertical" disabled={operationBusy}>
            <Form.Item label={t('封面')}>
              <Space direction="vertical" style={{ width: '100%' }}>
                {editing.cover_url
                  ? <Image src={editing.cover_url} height={160} style={{ objectFit: 'cover', borderRadius: 8 }} />
                  : <GlassEmpty text="暂无封面" />}
                <Space.Compact style={{ width: '100%' }}>
                  <Input placeholder={t('封面描述（留空用标题）')} onChange={(e) => editForm.setFieldValue('cover_prompt', e.target.value)} />
                  <Button icon={<PictureOutlined />} loading={coverLoading} disabled={operationBusy} onClick={onGenCover}>
                    {t('AI 生成封面')}
                  </Button>
                </Space.Compact>
              </Space>
            </Form.Item>
            <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('标题必填') }]}>
              <Input placeholder={t('文章标题')} />
            </Form.Item>
            <Form.Item name="author" label={t('作者')}>
              <Input placeholder={t('作者（可选）')} />
            </Form.Item>
            <Form.Item name="digest" label={t('摘要')}>
              <Input.TextArea rows={2} placeholder={t('摘要（120 字内）')} />
            </Form.Item>
            <Form.Item name="content_html" label={t('正文（HTML）')}>
              <Input.TextArea rows={12} placeholder={t('正文 HTML')} />
            </Form.Item>
            <Form.Item name="cover_url" hidden><Input /></Form.Item>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
              {coverLoading && <><Spin size="small" /> {t('正在生成封面…')} </>}
              {t('当前状态：')}
              <Tag color={(MP_ARTICLE_STATUS_META[editing.status] || { color: 'default' }).color}>
                {t((MP_ARTICLE_STATUS_META[editing.status] || { label: editing.status }).label)}
              </Tag>
              {editing.article_url && <a href={editing.article_url} target="_blank" rel="noreferrer">{t('查看已发布文章')}</a>}
            </Typography.Paragraph>
          </Form>
        )}
      </Drawer>
    </ListPageShell>
  )
}
