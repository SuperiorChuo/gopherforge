import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Modal, Upload, Tag, Tooltip, Form, Input, Grid } from 'antd'
import { message } from '@/utils/feedback'
import { copyToClipboard } from '@/utils/clipboard'
import {
  ReloadOutlined, UploadOutlined, DeleteOutlined, PlayCircleOutlined, AudioOutlined, CopyOutlined, SoundOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listPrompts, uploadPrompt, fetchPromptBlob, deletePrompt, generatePromptTTS, type PromptFile,
} from '@/api/cc'
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

type TtsForm = { name: string; text: string; voice?: string }

export default function CcPromptsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [uploading, setUploading] = useState(false)
  const [playing, setPlaying] = useState<{ name: string; url: string } | null>(null)
  const [ttsModal, setTtsModal] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [ttsForm] = Form.useForm<TtsForm>()

  const fetcher = useCallback(async () => {
    const res = await listPrompts()
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取提示音列表失败')), [t]),
  })

  const handleUpload = async (file: File) => {
    setUploading(true)
    try {
      await uploadPrompt(file)
      message.success(t('{{name}} 已上传', { name: file.name }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('上传失败（仅支持 wav/mp3，≤50MB）'))
    } finally {
      setUploading(false)
    }
    return false
  }

  const handleGenerate = async (values: TtsForm) => {
    setGenerating(true)
    try {
      const res = await generatePromptTTS({
        name: values.name.trim(),
        text: values.text.trim(),
        voice: values.voice?.trim() || undefined,
      })
      message.success(t('已生成 {{name}}', { name: res.name }))
      setTtsModal(false)
      ttsForm.resetFields()
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('TTS 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  const handlePlay = async (name: string) => {
    try {
      const blob = await fetchPromptBlob(name)
      setPlaying({ name, url: URL.createObjectURL(blob) })
    } catch {
      message.error(t('拉取音频失败'))
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deletePrompt(name)
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

  const columns: ColumnsType<PromptFile> = [
    {
      title: t('文件'),
      dataIndex: 'name',
      render: (v: string) => (
        <Space size={6}><SoundOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('格式'),
      key: 'fmt',
      width: 80,
      render: (_, r) => <Tag variant="filled">{r.name.endsWith('.mp3') ? 'MP3' : 'WAV'}</Tag>,
    },
    { title: t('大小'), dataIndex: 'size', width: 100, render: formatBytes },
    { title: t('生成时间'), dataIndex: 'modified_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('引用路径'),
      dataIndex: 'path',
      render: (v: string) => (
        <Space size={6}>
          <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>{v}</span>
          <Tooltip title={t('复制（可填进队列播报音 / IVR 提示音）')}>
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
      className="cc-prompts-page"
      toolbar={
        <TableToolbar
          title="提示音库"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<AudioOutlined />} onClick={() => { ttsForm.resetFields(); setTtsModal(true) }}>
                {t('TTS 生成')}
              </Button>
              <Upload
                accept=".wav,.mp3"
                showUploadList={false}
                beforeUpload={(f) => handleUpload(f as unknown as File)}
              >
                <Button icon={<UploadOutlined />} loading={uploading}>{t('上传录音')}</Button>
              </Upload>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
          {t('队列排队播报、IVR 导航语、超时提示都可从这里取音；上传录音或用 TTS 文本转语音生成（自动转 8k 单声道电话音质）。')}
        </div>
        <Table
          rowKey="name"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无提示音（可上传录音或用 TTS 生成）" compact /> }}
        />

      <Modal
        title={t('TTS 文本转语音')}
        open={ttsModal}
        onCancel={() => setTtsModal(false)}
        onOk={() => ttsForm.submit()}
        confirmLoading={generating}
        okText={t('生成')}
        destroyOnHidden
        width={520}
      >
        <Form form={ttsForm} layout="vertical" onFinish={handleGenerate} style={{ marginTop: 8 }}>
          <Form.Item
            name="name"
            label={t('文件名')}
            tooltip={t('仅字母/数字/._-；自动补 .wav 后缀')}
            rules={[
              { required: true, message: t('请输入文件名') },
              { pattern: /^[A-Za-z0-9._-]{1,120}$/, message: t('仅字母/数字/._-（≤120）') },
            ]}
          >
            <Input placeholder={t('如 welcome')} addonAfter=".wav" />
          </Form.Item>
          <Form.Item
            name="text"
            label={t('文本内容')}
            rules={[{ required: true, message: t('请输入要合成的文本') }, { max: 4000, message: t('≤4000 字') }]}
          >
            <Input.TextArea rows={4} placeholder={t('如：您好，欢迎致电，请稍候，坐席马上为您服务。')} />
          </Form.Item>
          <Form.Item
            name="voice"
            label={t('音色（可选）')}
            tooltip={t('留空用服务端默认音色；具体可选值取决于 TTS 供应商')}
            rules={[{ pattern: /^[A-Za-z0-9._-]{0,40}$/, message: t('仅字母/数字/._-') }]}
          >
            <Input placeholder={t('如 alloy / nova（留空=默认）')} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title={playing?.name} open={!!playing} onCancel={closePlayer} footer={null} destroyOnHidden>
        {playing && (
          // eslint-disable-next-line jsx-a11y/media-has-caption
          <audio controls autoPlay src={playing.url} style={{ width: '100%' }} />
        )}
      </Modal>
    </ListPageShell>
  )
}
