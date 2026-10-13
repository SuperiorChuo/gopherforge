import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Table, Tag, Space, Typography, Button, Empty } from 'antd'
import { ShareAltOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import { message } from '@/utils/feedback'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import {
  listSocialPosts, listSocialChannels, getSocialPost,
  SOCIAL_PLATFORM_META, PUBLICATION_STATUS_META,
  type SocialPost, type PostPublication, type ChannelCapabilities,
} from '@/api/social'

type Row = SocialPost & { publications: PostPublication[] }

export default function SocialPublishPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [channels, setChannels] = useState<Record<string, ChannelCapabilities>>({})

  const fetcher = useCallback(async () => {
    const res = await listSocialPosts({ page: 1, page_size: 30 })
    // 拉每条内容的发布记录（数量有限，逐条拉可接受）
    const list = await Promise.all(
      (res.list || []).map(async (p) => {
        try {
          const d = await getSocialPost(p.id)
          return { ...p, publications: d.publications || [] }
        } catch {
          return { ...p, publications: [] }
        }
      }),
    )
    return { list, total: list.length }
  }, [])
  const { list: rows, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('加载失败')), [t]),
  })

  useEffect(() => {
    listSocialChannels().then((r) => setChannels(r.channels || {})).catch(() => setChannels({}))
  }, [])

  const columns: ColumnsType<Row> = [
    { title: t('标题'), dataIndex: 'title', render: (txt: string) => txt || <Typography.Text type="secondary">{t('（未命名）')}</Typography.Text> },
    {
      title: t('平台'), dataIndex: 'platform', width: 120,
      render: (p: string) => { const m = SOCIAL_PLATFORM_META[p] || { label: p || '通用', color: 'default' }; return <Tag color={m.color}>{t(m.label)}</Tag> },
    },
    {
      title: t('发布状态'), key: 'pubs',
      render: (_: unknown, r: Row) =>
        r.publications.length === 0
          ? <Typography.Text type="secondary">{t('未发布')}</Typography.Text>
          : (
            <Space wrap>
              {r.publications.map((p) => {
                const m = PUBLICATION_STATUS_META[p.status] || { label: p.status, color: 'default' }
                const chLabel = SOCIAL_PLATFORM_META[p.channel]?.label || p.channel
                return p.external_url
                  ? <a key={p.id} href={p.external_url} target="_blank" rel="noreferrer"><Tag color={m.color}>{t(chLabel)}·{t(m.label)}</Tag></a>
                  : <Tag key={p.id} color={m.color}>{t(chLabel)}·{t(m.label)}</Tag>
              })}
            </Space>
          ),
    },
    {
      title: t('操作'), width: 120, fixed: 'right',
      render: (_: unknown, r: Row) => (
        <Button size="small" type="primary" ghost onClick={() => navigate(`/social/content?edit=${r.id}`)}>{t('去发布')}</Button>
      ),
    },
  ]

  const channelKeys = Object.keys(channels)

  return (
    <ListPageShell
      className="social-publish-page"
      toolbar={
        <TableToolbar
          title="发布中心"
          icon={<ShareAltOutlined />}
          gradient="linear-gradient(135deg, #f472b6, #a855f7)"
          glow="rgba(168, 85, 247, 0.4)"
          description="总览各内容在各渠道的发布状态；发布/导出在「内容中心」编辑页操作"
          extra={<Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>}
        />}
      >
        <Card size="small" style={{ marginBottom: 12 }}>
          <Space wrap>
            <Typography.Text strong>{t('已接入渠道：')}</Typography.Text>
            {channelKeys.length === 0
              ? <Typography.Text type="secondary">{t('暂无（更多平台即将接入）')}</Typography.Text>
              : channelKeys.map((ch) => {
                const cap = channels[ch]
                return (
                  <Tag key={ch} color={cap.publish ? 'green' : 'orange'}>
                    {t(SOCIAL_PLATFORM_META[ch]?.label || ch)}·{cap.publish ? t('API 直发') : t('导出')}
                  </Tag>
                )
              })}
          </Space>
        </Card>
        <Table
          rowKey="id" loading={loading} columns={columns} dataSource={rows}
          scroll={{ x: 720 }} pagination={{ pageSize: 10 }}
          locale={{ emptyText: <GlassEmpty text="还没有内容，去「内容中心」创建" /> }}
        />
        {channelKeys.length === 0 && rows.length === 0 && (
          <Empty description={t('更多社媒平台（小红书 / 抖音 / 知乎 / 微博）即将接入')} />
        )}
    </ListPageShell>
  )
}
