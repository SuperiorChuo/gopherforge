import { useState } from 'react'
import { Alert, Button, Modal, Space, Table, Typography, Upload } from 'antd'
import { DownloadOutlined, InboxOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'

const { Text } = Typography

/** 逐行导入错误明细（后端部分成功语义的契约） */
export interface ExcelImportRowError {
  row: number
  username?: string
  reason: string
}

export interface ExcelImportResult {
  total: number
  success: number
  failed: number
  errors?: ExcelImportRowError[]
}

interface ExcelImportModalProps {
  open: boolean
  title: string
  /** 上传区下方的说明文案 */
  hint?: string
  onClose: () => void
  /** 有成功行时关闭后回调（刷新列表用） */
  onDone?: () => void
  downloadTemplate: () => Promise<void>
  doImport: (file: File) => Promise<ExcelImportResult>
}

/**
 * 通用 Excel 批量导入弹窗（路线图第 11 项）：下载模板 → 选文件 → 导入 →
 * 汇总 + 逐行错误明细。部分成功不回滚，与后端语义一致。
 */
export default function ExcelImportModal({
  open,
  title,
  hint,
  onClose,
  onDone,
  downloadTemplate,
  doImport,
}: ExcelImportModalProps) {
  const [file, setFile] = useState<File | null>(null)
  const [importing, setImporting] = useState(false)
  const [result, setResult] = useState<ExcelImportResult | null>(null)

  const reset = () => {
    setFile(null)
    setResult(null)
    setImporting(false)
  }

  const close = () => {
    const hadSuccess = (result?.success ?? 0) > 0
    reset()
    onClose()
    if (hadSuccess) onDone?.()
  }

  const run = async () => {
    if (!file) {
      message.warning('请先选择 .xlsx 文件')
      return
    }
    setImporting(true)
    try {
      const res = await doImport(file)
      setResult(res)
      if (res.failed === 0) message.success(`导入完成：成功 ${res.success} 条`)
      else message.warning(`导入完成：成功 ${res.success} 条，失败 ${res.failed} 条`)
    } catch {
      // 整体失败（表头不符/文件超限等）由拦截器提示
    } finally {
      setImporting(false)
    }
  }

  return (
    <Modal
      title={title}
      open={open}
      onCancel={close}
      destroyOnHidden
      width={560}
      footer={[
        <Button key="close" onClick={close}>
          关闭
        </Button>,
        <Button key="run" type="primary" loading={importing} disabled={!file} onClick={() => void run()}>
          开始导入
        </Button>,
      ]}
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Button
          icon={<DownloadOutlined />}
          onClick={() => void downloadTemplate().catch(() => {})}
        >
          下载导入模板
        </Button>
        <Upload.Dragger
          accept=".xlsx"
          maxCount={1}
          beforeUpload={(f) => {
            setFile(f as unknown as File)
            setResult(null)
            return false
          }}
          onRemove={() => {
            setFile(null)
            return true
          }}
        >
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p className="ant-upload-text">点击或拖拽 .xlsx 文件到此处</p>
          {hint && <p className="ant-upload-hint">{hint}</p>}
        </Upload.Dragger>
        {result && (
          <>
            <Alert
              type={result.failed === 0 ? 'success' : 'warning'}
              showIcon
              message={`共 ${result.total} 条：成功 ${result.success}，失败 ${result.failed}`}
            />
            {(result.errors?.length ?? 0) > 0 && (
              <Table
                size="small"
                rowKey={(r) => `${r.row}-${r.reason}`}
                dataSource={result.errors}
                pagination={result.errors!.length > 8 ? { pageSize: 8 } : false}
                columns={[
                  { title: '行号', dataIndex: 'row', width: 70 },
                  {
                    title: '用户名',
                    dataIndex: 'username',
                    width: 130,
                    render: (v?: string) => v || <Text type="secondary">—</Text>,
                  },
                  { title: '失败原因', dataIndex: 'reason' },
                ]}
              />
            )}
          </>
        )}
      </Space>
    </Modal>
  )
}
