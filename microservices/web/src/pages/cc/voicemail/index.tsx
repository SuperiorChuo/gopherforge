import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Modal, Input, Alert, Tag, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, MailOutlined, PlayCircleOutlined, DeleteOutlined, SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listFsVoicemails, fetchFsVoicemailBlob, deleteFsVoicemail, type FsVoicemail,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

function formatBytes(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export default function CcVoicemailPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  // 筛选草稿（输入框实时值）与生效查询值分离：回车/点查询才触发重新拉取
  const [extFilter, setExtFilter] = useState('')
  const [queryExt, setQueryExt] = useState('')
  // 播放弹窗：blob URL 关闭时释放
  const [playing, setPlaying] = useState<{ file: string; url: string } | null>(null)

  const queryParams = useMemo(() => ({ ext: queryExt }), [queryExt])
  const fetcher = useCallback(async (p: { ext: string }) => {
    const res = await listFsVoicemails(p.ext || undefined)
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list: rows, loading, reload } = useTableQuery({
    params: queryParams,
    fetcher,
    onError: useCallback(() => message.error(t('获取留言列表失败')), [t]),
  })

  const handlePlay = async (row: FsVoicemail) => {
    try {
      const blob = await fetchFsVoicemailBlob(row.ext, row.file)
      setPlaying({ file: row.file, url: URL.createObjectURL(blob) })
    } catch {
      message.error(t('拉取留言失败'))
    }
  }

  const handleDelete = async (row: FsVoicemail) => {
    try {
      await deleteFsVoicemail(row.ext, row.file)
      message.success(t('留言已删除'))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除留言失败'))
    }
  }

  const closePlayer = () => {
    if (playing) URL.revokeObjectURL(playing.url)
    setPlaying(null)
  }

  const columns: ColumnsType<FsVoicemail> = [
    {
      title: t('分机'),
      dataIndex: 'ext',
      width: 110,
      render: (v: string) => <Tag color="cyan" variant="filled">{v}</Tag>,
    },
    {
      title: t('留言文件'),
      dataIndex: 'file',
      render: (v: string) => (
        <Space size={6}><MailOutlined /><span className="cell-mono" style={{ fontSize: 12 }}>{v}</span></Space>
      ),
    },
    { title: t('大小'), dataIndex: 'size', width: 110, render: formatBytes },
    { title: t('时间'), dataIndex: 'modified_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'play', label: t('试听'), icon: <PlayCircleOutlined />, onClick: () => handlePlay(row) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除该留言？'),
              onClick: () => { void handleDelete(row) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-voicemail-page"
      toolbar={
        <TableToolbar
          title="语音留言"
          total={rows.length}
          extra={
            <Space size={12}>
              <Input
                allowClear
                placeholder={t('按分机号筛选')}
                prefix={<SearchOutlined />}
                value={extFilter}
                onChange={(e) => setExtFilter(e.target.value)}
                onPressEnter={() => setQueryExt(extFilter.trim())}
                style={{ width: 180 }}
              />
              <Button icon={<SearchOutlined />} onClick={() => setQueryExt(extFilter.trim())}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { setExtFilter(''); setQueryExt('') }}>{t('刷新')}</Button>
            </Space>
          }
        />}
      >
      <Alert
        type="info"
        showIcon
        icon={<MailOutlined />}
        style={{ marginBottom: 16 }}
        message={t('坐席拨 *98 也可在话机上听取留言（拨 *98 后按提示输分机号 + 信箱密码；直接拨 *98分机号 可直入指定信箱）')}
      />
        <Table
          rowKey={(r) => `${r.ext}/${r.file}`}
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={rows}
          loading={loading}
          pagination={{ pageSize: 20, showSizeChanger: false, hideOnSinglePage: true }}
          locale={{ emptyText: <GlassEmpty text="暂无留言（分机开启语音信箱并无人接听后生成）" compact /> }}
        />

      <Modal title={playing?.file} open={!!playing} onCancel={closePlayer} footer={null} destroyOnHidden>
        {playing && (
          // eslint-disable-next-line jsx-a11y/media-has-caption
          <audio controls autoPlay src={playing.url} style={{ width: '100%' }} />
        )}
      </Modal>
    </ListPageShell>
  )
}
