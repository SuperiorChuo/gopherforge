import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Modal, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, SoundOutlined, PlayCircleOutlined, DownloadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listFsRecordings, fetchFsRecordingBlob, type FsRecording } from '@/api/cc'
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

export default function CcRecordingsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  // 播放弹窗：blob URL 关闭时释放
  const [playing, setPlaying] = useState<{ name: string; url: string } | null>(null)

  const fetcher = useCallback(async () => {
    const res = await listFsRecordings()
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list: recs, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取录音列表失败')), [t]),
  })

  const handlePlay = async (name: string) => {
    try {
      const blob = await fetchFsRecordingBlob(name)
      setPlaying({ name, url: URL.createObjectURL(blob) })
    } catch {
      message.error(t('拉取录音失败'))
    }
  }

  const handleDownload = async (name: string) => {
    try {
      const blob = await fetchFsRecordingBlob(name)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      a.click()
      URL.revokeObjectURL(url)
    } catch {
      message.error(t('下载录音失败'))
    }
  }

  const closePlayer = () => {
    if (playing) URL.revokeObjectURL(playing.url)
    setPlaying(null)
  }

  const columns: ColumnsType<FsRecording> = [
    {
      title: t('文件'),
      dataIndex: 'name',
      render: (v: string) => (
        <Space size={6}><SoundOutlined /><span className="cell-mono" style={{ fontSize: 12 }}>{v}</span></Space>
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
            { key: 'play', label: t('播放'), icon: <PlayCircleOutlined />, onClick: () => handlePlay(row.name) },
            { key: 'download', label: t('下载'), icon: <DownloadOutlined />, onClick: () => handleDownload(row.name) },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-recordings-page"
      toolbar={
        <TableToolbar
          title="通话录音"
          total={recs.length}
          extra={<Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>}
        />}
    >
        <Table
          rowKey="name"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={recs}
          loading={loading}
          pagination={{ pageSize: 20, showSizeChanger: false, hideOnSinglePage: true }}
          locale={{ emptyText: <GlassEmpty text="暂无录音（通话接通后自动生成）" compact /> }}
        />

      <Modal title={playing?.name} open={!!playing} onCancel={closePlayer} footer={null} destroyOnHidden>
        {playing && (
          // eslint-disable-next-line jsx-a11y/media-has-caption
          <audio controls autoPlay src={playing.url} style={{ width: '100%' }} />
        )}
      </Modal>
    </ListPageShell>
  )
}
