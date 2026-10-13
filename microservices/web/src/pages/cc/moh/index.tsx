import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Modal, Upload, Tag, Tooltip, Grid } from 'antd'
import { message } from '@/utils/feedback'
import { copyToClipboard } from '@/utils/clipboard'
import {
  ReloadOutlined, UploadOutlined, DeleteOutlined, PlayCircleOutlined, CustomerServiceOutlined, CopyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listMoh, uploadMoh, fetchMohBlob, deleteMoh, type MohFile } from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

function formatBytes(n: number) {
  if (!n) return '0 B'
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export default function CcMohPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [uploading, setUploading] = useState(false)
  const [playing, setPlaying] = useState<{ name: string; url: string } | null>(null)

  const fetcher = useCallback(async () => {
    const res = await listMoh()
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取音乐列表失败')), [t]),
  })

  const handleUpload = async (file: File) => {
    setUploading(true)
    try {
      await uploadMoh(file)
      message.success(t('{{name}} 已上传，排队音乐即刻生效', { name: file.name }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('上传失败（仅支持 wav/mp3，≤50MB）'))
    } finally {
      setUploading(false)
    }
    return false
  }

  const handlePlay = async (name: string) => {
    try {
      const blob = await fetchMohBlob(name)
      setPlaying({ name, url: URL.createObjectURL(blob) })
    } catch {
      message.error(t('拉取音频失败'))
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteMoh(name)
      message.success(t('{{name}} 已删除', { name }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除失败'))
    }
  }

  const closePlayer = () => {
    if (playing) URL.revokeObjectURL(playing.url)
    setPlaying(null)
  }

  const columns: ColumnsType<MohFile> = [
    {
      title: t('文件'),
      dataIndex: 'name',
      render: (v: string) => (
        <Space size={6}><CustomerServiceOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('格式'),
      key: 'fmt',
      width: 80,
      render: (_, r) => <Tag variant="filled">{r.name.endsWith('.mp3') ? 'MP3' : 'WAV'}</Tag>,
    },
    { title: t('大小'), dataIndex: 'size', width: 100, render: formatBytes },
    { title: t('上传时间'), dataIndex: 'modified_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('引用路径'),
      dataIndex: 'path',
      render: (v: string) => (
        <Space size={6}>
          <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>{v}</span>
          <Tooltip title={t('复制（队列可指定单曲）')}>
            <Button
              size="small"
              type="text"
              icon={<CopyOutlined />}
              onClick={() => { void copyToClipboard(v).then((ok) => ok ? message.success(t('已复制')) : message.error(t('复制失败'))) }}
            />
          </Tooltip>
        </Space>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, r) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'play', label: t('试听'), icon: <PlayCircleOutlined />, onClick: () => handlePlay(r.name) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('删除 {{name}}？', { name: r.name }),
              onClick: () => { void handleDelete(r.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-moh-page"
      toolbar={
        <TableToolbar
          title="排队 / 保持音乐"
          total={list.length}
          extra={
            <Space size={16}>
              <Upload
                accept=".wav,.mp3"
                showUploadList={false}
                beforeUpload={(f) => handleUpload(f as unknown as File)}
              >
                <Button type="primary" icon={<UploadOutlined />} loading={uploading}>{t('上传音乐')}</Button>
              </Upload>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
          {t('客户排队 / 被保持时循环播放这里的曲目（随机顺序）;上传即热生效。队列设置里也可指定播放某一首。')}
        </div>
        <Table
          rowKey="name"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无音乐（未上传时排队为静音）" compact /> }}
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
