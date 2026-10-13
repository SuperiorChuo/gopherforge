import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import {
  Button, Card, Table, Tag, Space, Modal, Form, Input, Select, Drawer, Typography, Popconfirm, Divider, List,
} from 'antd'
import { EditOutlined, RobotOutlined, PlusOutlined, ReloadOutlined, SendOutlined, ExportOutlined, CopyOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import { copyToClipboard } from '@/utils/clipboard'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import {
  listSocialPosts, createSocialPost, generateSocialPost, getSocialPost, updateSocialPost, deleteSocialPost,
  publishSocialPost, exportSocialPost, listSocialChannels, listSocialChannelAccounts,
  SOCIAL_PLATFORM_META, POST_STATUS_META, PUBLICATION_STATUS_META, SOCIAL_PLATFORM_OPTIONS,
  type SocialPost, type PostPublication, type ChannelCapabilities, type ChannelAccount, type ExportPackage,
} from '@/api/social'

export default function SocialContentPage() {
  const { t } = useTranslation()
  const [platform, setPlatform] = useState<string | undefined>()
  const [keyword, setKeyword] = useState('')
  const [appliedKeyword, setAppliedKeyword] = useState('')

  const [genOpen, setGenOpen] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [genForm] = Form.useForm()

  const [searchParams, setSearchParams] = useSearchParams()

  const [editing, setEditing] = useState<SocialPost | null>(null)
  const [editForm] = Form.useForm()
  const [publications, setPublications] = useState<PostPublication[]>([])

  // 渠道能力与账号（发布区用）
  const [channels, setChannels] = useState<Record<string, ChannelCapabilities>>({})
  const [accounts, setAccounts] = useState<ChannelAccount[]>([])
  const [pubChannel, setPubChannel] = useState<string | undefined>()
  const [pubAccountId, setPubAccountId] = useState<number | undefined>()
  const [publishing, setPublishing] = useState(false)
  const [exportPkg, setExportPkg] = useState<ExportPackage | null>(null)

  const params = { platform, appliedKeyword }
  const fetcher = useCallback(async (p: typeof params) => {
    const res = await listSocialPosts({ platform: p.platform, keyword: p.appliedKeyword, page: 1, page_size: 50 })
    const list = res.list || []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params,
    fetcher,
    onError: useCallback(() => message.error(t('加载内容失败')), [t]),
  })

  useEffect(() => {
    listSocialChannels().then((res) => setChannels(res.channels || {})).catch(() => setChannels({}))
  }, [])

  // 从发布页「去发布」跳转过来：读 ?edit=<id> 拉取该内容并打开编辑抽屉，随后清掉参数
  useEffect(() => {
    const editId = searchParams.get('edit')
    if (!editId) return
    getSocialPost(Number(editId))
      .then((detail) => openEdit(detail.post))
      .catch(() => message.error(t('内容不存在或已删除')))
      .finally(() => {
        const next = new URLSearchParams(searchParams)
        next.delete('edit')
        setSearchParams(next, { replace: true })
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const onGenerate = async () => {
    const v = await genForm.validateFields()
    setGenerating(true)
    try {
      const post = await generateSocialPost({ prompt: v.prompt, platform: v.platform || '' })
      message.success(t('AI 内容已生成'))
      setGenOpen(false)
      genForm.resetFields()
      void reload()
      openEdit(post)
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('AI 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  const openEdit = async (p: SocialPost) => {
    setEditing(p)
    setExportPkg(null)
    setPubChannel(undefined)
    setPubAccountId(undefined)
    editForm.setFieldsValue({
      title: p.title, summary: p.summary, body_html: p.body_html,
      cover_url: p.cover_url, platform: p.platform, tags: p.tags || [], status: p.status,
    })
    try {
      const detail = await getSocialPost(p.id)
      setPublications(detail.publications || [])
    } catch { setPublications([]) }
  }

  const openNew = () => {
    const empty: SocialPost = {
      id: 0, tenant_id: 0, title: '', body_html: '', summary: '', cover_url: '',
      media: [], tags: [], platform: '', status: 'draft', ai_prompt: '', created_by: 0,
      created_at: '', updated_at: '',
    }
    setEditing(empty)
    setExportPkg(null)
    editForm.setFieldsValue({ title: '', summary: '', body_html: '', cover_url: '', platform: '', tags: [], status: 'draft' })
    setPublications([])
  }

  const onSave = async () => {
    if (!editing) return
    const v = await editForm.validateFields()
    try {
      const saved = editing.id
        ? await updateSocialPost(editing.id, v)
        : await createSocialPost(v)
      message.success(t('已保存'))
      setEditing(saved)
      void reload()
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  // 选渠道 → 拉该渠道账号
  const onPickChannel = (ch: string) => {
    setPubChannel(ch)
    setPubAccountId(undefined)
    setExportPkg(null)
    listSocialChannelAccounts(ch).then((res) => setAccounts(res.list || [])).catch(() => setAccounts([]))
  }

  const currentCap = pubChannel ? channels[pubChannel] : undefined

  const onPublishOrExport = async () => {
    if (!editing || !editing.id || !pubChannel) return
    setPublishing(true)
    try {
      if (currentCap?.publish) {
        if (!pubAccountId) { message.warning(t('请选择发布账号')); return }
        const rec = await publishSocialPost(editing.id, { channel: pubChannel, account_id: pubAccountId })
        message.success(rec.status === 'published' ? t('已发布') : t('已提交，等待结果'))
        const detail = await getSocialPost(editing.id)
        setPublications(detail.publications || [])
      } else {
        const pkg = await exportSocialPost(editing.id, { channel: pubChannel, account_id: pubAccountId })
        setExportPkg(pkg)
        message.success(t('已生成导出内容，复制后去 App 发布'))
      }
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('操作失败'))
    } finally {
      setPublishing(false)
    }
  }

  const copyText = (txt: string) => {
    void copyToClipboard(txt).then(
      (ok) => (ok ? message.success(t('文案已复制')) : message.error(t('复制失败，请手动选择'))),
    )
  }

  const columns: ColumnsType<SocialPost> = [
    { title: t('标题'), dataIndex: 'title', render: (txt: string) => txt || <Typography.Text type="secondary">{t('（未命名）')}</Typography.Text> },
    {
      title: t('平台'), dataIndex: 'platform', width: 120,
      render: (p: string) => { const m = SOCIAL_PLATFORM_META[p] || { label: p || '通用', color: 'default' }; return <Tag color={m.color}>{t(m.label)}</Tag> },
    },
    {
      title: t('状态'), dataIndex: 'status', width: 100,
      render: (s: string) => { const m = POST_STATUS_META[s] || { label: s, color: 'default' }; return <Tag color={m.color}>{t(m.label)}</Tag> },
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 180, responsive: ['lg'] },
    {
      title: t('操作'), width: 150, fixed: 'right',
      render: (_: unknown, r: SocialPost) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>{t('编辑')}</Button>
          <Popconfirm title={t('删除该内容？')} onConfirm={() => deleteSocialPost(r.id).then(() => { message.success(t('已删除')); void reload() })}>
            <Button size="small" danger>{t('删除')}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const channelKeys = Object.keys(channels)

  return (
    <ListPageShell
      className="social-content-page"
      toolbar={
        <TableToolbar
          title="内容中心"
          icon={<EditOutlined />}
          gradient="linear-gradient(135deg, #22d3ee, #6366f1)"
          glow="rgba(99, 102, 241, 0.4)"
          description="一次创作，多平台风格适配 → 发布到微信 / 导出到小红书知乎等"
          extra={
            <Space wrap>
              <Select
                allowClear style={{ minWidth: 140 }} placeholder={t('平台筛选')}
                value={platform} onChange={setPlatform} options={SOCIAL_PLATFORM_OPTIONS.filter((o) => o.value !== '').map((o) => ({ ...o, label: t(o.label) }))}
              />
              <Input.Search
                allowClear placeholder={t('搜索标题/摘要')} style={{ width: 200 }}
                value={keyword} onChange={(e) => setKeyword(e.target.value)} onSearch={() => setAppliedKeyword(keyword)}
              />
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
              <Button icon={<PlusOutlined />} onClick={openNew}>{t('新建内容')}</Button>
              <Button type="primary" icon={<RobotOutlined />} onClick={() => setGenOpen(true)}>{t('AI 生成内容')}</Button>
            </Space>
          }
        />}
      >
        <Table
          rowKey="id" loading={loading} columns={columns} dataSource={list}
          scroll={{ x: 760 }} pagination={{ pageSize: 10 }}
          locale={{ emptyText: <GlassEmpty text="还没有内容，点「AI 生成内容」开始" /> }}
        />

      <Modal
        title={<span><RobotOutlined /> {t('AI 生成社媒内容')}</span>}
        open={genOpen} onOk={onGenerate} confirmLoading={generating} onCancel={() => setGenOpen(false)}
        okText={t('生成')} destroyOnClose
      >
        <Form form={genForm} layout="vertical" initialValues={{ platform: '' }}>
          <Form.Item name="platform" label={t('目标平台风格')}>
            <Select options={SOCIAL_PLATFORM_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} />
          </Form.Item>
          <Form.Item name="prompt" label={t('选题 / 要求')} rules={[{ required: true, message: t('请输入选题') }]}>
            <Input.TextArea rows={4} placeholder={t('例如：推荐一款适合秋天的养生茶')} />
          </Form.Item>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
            {t('选不同平台会生成对应风格：微信=正式长图文，小红书=种草+话题标签，知乎=专业长文，微博=短博文，抖音=口播脚本。')}
          </Typography.Paragraph>
        </Form>
      </Modal>

      <Drawer
        title={editing?.id ? t('编辑内容') : t('新建内容')} width={720} open={!!editing}
        onClose={() => setEditing(null)}
        extra={<Button type="primary" onClick={onSave}>{t('保存')}</Button>}
      >
        {editing && (
          <>
            <Form form={editForm} layout="vertical">
              <Form.Item name="platform" label={t('平台')}><Select options={SOCIAL_PLATFORM_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} /></Form.Item>
              <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('标题必填') }]}><Input /></Form.Item>
              <Form.Item name="summary" label={t('摘要')}><Input.TextArea rows={2} /></Form.Item>
              <Form.Item name="body_html" label={t('正文')}><Input.TextArea rows={10} placeholder={t('正文（HTML 或纯文本，视平台而定）')} /></Form.Item>
              <Form.Item name="cover_url" label={t('封面 URL')}><Input placeholder={t('http 或 data:image URL')} /></Form.Item>
              <Form.Item name="tags" label={t('标签')}><Select mode="tags" placeholder={t('回车添加标签')} /></Form.Item>
              <Form.Item name="status" label={t('状态')}>
                <Select options={[{ label: t('草稿'), value: 'draft' }, { label: t('待发布'), value: 'ready' }, { label: t('已归档'), value: 'archived' }]} />
              </Form.Item>
            </Form>

            {editing.id > 0 && (
              <>
                <Divider>{t('发布 / 导出')}</Divider>
                {channelKeys.length === 0 ? (
                  <GlassEmpty text="暂无可用渠道，更多平台即将接入" />
                ) : (
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Space wrap>
                      <Select
                        style={{ minWidth: 160 }} placeholder={t('选择渠道')} value={pubChannel} onChange={onPickChannel}
                        options={channelKeys.map((ch) => ({ label: SOCIAL_PLATFORM_META[ch]?.label ? t(SOCIAL_PLATFORM_META[ch].label) : ch, value: ch }))}
                      />
                      {currentCap?.publish && (
                        <Select
                          style={{ minWidth: 180 }} placeholder={t('选择账号')} value={pubAccountId} onChange={setPubAccountId}
                          notFoundContent={t('该渠道暂无账号，请先配置')}
                          options={accounts.map((a) => ({ label: a.name || `#${a.id}`, value: a.id }))}
                        />
                      )}
                      <Button
                        type="primary" loading={publishing} onClick={onPublishOrExport} disabled={!pubChannel}
                        icon={currentCap?.publish ? <SendOutlined /> : <ExportOutlined />}
                      >
                        {currentCap?.publish ? t('发布') : t('导出文案')}
                      </Button>
                    </Space>
                    {currentCap?.note && <Typography.Text type="secondary" style={{ fontSize: 12 }}>{currentCap.note}</Typography.Text>}

                    {exportPkg && (
                      <Card size="small" title={t('导出内容（复制后去 App 粘贴发布）')} extra={<Button size="small" icon={<CopyOutlined />} onClick={() => copyText(exportPkg.platform_text)}>{t('复制文案')}</Button>}>
                        <Input.TextArea value={exportPkg.platform_text} rows={8} readOnly />
                        {(exportPkg.images?.length || 0) > 0 && (
                          <>
                            <Divider style={{ margin: '8px 0' }}>{t('素材下载')}</Divider>
                            <Space wrap>
                              {exportPkg.images!.map((im, i) => (
                                <a key={i} href={im.url} target="_blank" rel="noreferrer" download>{im.name || t('图片 {{n}}', { n: i + 1 })}</a>
                              ))}
                            </Space>
                          </>
                        )}
                        {exportPkg.tips && <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 8 }}>{exportPkg.tips}</Typography.Paragraph>}
                      </Card>
                    )}

                    {publications.length > 0 && (
                      <List
                        size="small" header={<b>{t('发布记录')}</b>} dataSource={publications}
                        renderItem={(p) => {
                          const m = PUBLICATION_STATUS_META[p.status] || { label: p.status, color: 'default' }
                          return (
                            <List.Item>
                              <Space>
                                <Tag>{t(SOCIAL_PLATFORM_META[p.channel]?.label || p.channel)}</Tag>
                                <Tag color={m.color}>{t(m.label)}</Tag>
                                <Typography.Text type="secondary" style={{ fontSize: 12 }}>{p.created_at}</Typography.Text>
                                {p.external_url && <a href={p.external_url} target="_blank" rel="noreferrer">{t('查看')}</a>}
                              </Space>
                            </List.Item>
                          )
                        }}
                      />
                    )}
                  </Space>
                )}
              </>
            )}
          </>
        )}
      </Drawer>
    </ListPageShell>
  )
}
