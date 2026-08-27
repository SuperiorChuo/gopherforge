import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { downloadFile, getFileList } from '@/api/system/file'
import type { FileRecord } from '@/types'
import { message } from '@/utils/feedback'
import { formatBytes } from '@/utils/format'
import SimpleListPage from '../SimpleListPage'

export default function MobileFilesPage() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState<number | null>(null)

  const load = useCallback(async () => {
    const res = await getFileList({ page: 1, page_size: 30 })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])

  const handleDownload = async (row: FileRecord) => {
    setSaving(row.id)
    try {
      await downloadFile(row.id, row.file_name || `file-${row.id}`)
    } catch {
      message.error(t('下载失败'))
    } finally {
      setSaving(null)
    }
  }

  return (
    <SimpleListPage
      title="文件"
      emptyText="暂无文件"
      load={load}
      rowKey={(row: FileRecord) => row.id}
      render={(row) => (
        <article className="m-row">
          <div className="m-row-top">
            <span className="m-pill">{row.file_type || row.mime_type || t('附件')}</span>
            <time>{row.created_at ? dayjs(row.created_at).format('MM-DD HH:mm') : '--'}</time>
          </div>
          <strong>{row.file_name || t('未命名文件')}</strong>
          <p>{[formatBytes(row.file_size), row.storage_type].filter(Boolean).join(' · ')}</p>
          <div className="m-actions">
            <button
              type="button"
              className="m-text-btn"
              disabled={saving === row.id}
              onClick={() => void handleDownload(row)}
            >
              {saving === row.id ? t('下载中…') : t('下载')}
            </button>
          </div>
        </article>
      )}
    />
  )
}
