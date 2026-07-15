import request from '@/utils/request'
import type { PageRequest, PageResponse, FileRecord } from '@/types'

type FileListParams = PageRequest & { keyword?: string; file_type?: string }

export const getFileList = (params: FileListParams) =>
  request.get<unknown, PageResponse<FileRecord>>('/api/v1/files', { params })

export const uploadFile = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post<unknown, FileRecord>('/api/v1/files/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

// 批量上传，后端字段名为 files
export const uploadFiles = (files: File[]) => {
  const form = new FormData()
  files.forEach((f) => form.append('files', f))
  return request.post<unknown, FileRecord[]>('/api/v1/files/upload/multiple', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export const deleteFile = (id: number) =>
  request.delete<unknown, void>(`/api/v1/files/${id}`)

export interface FileStats {
  total: number
  total_size: number
  by_type?: Record<string, { count: number; size: number }>
}

export const getFileStats = () =>
  request.get<unknown, FileStats>('/api/v1/files/stats', { silent: true })

export const batchDeleteFiles = (ids: number[]) =>
  request.delete<unknown, void>('/api/v1/files/batch', { data: { ids } })

// 仅图片可预览；带 token 取回字节流，返回 object URL（调用方负责 revoke）
export const previewFile = async (id: number) => {
  const blob = await request.get<unknown, Blob>(`/api/v1/files/${id}/preview`, {
    responseType: 'blob',
  })
  return URL.createObjectURL(blob)
}

// 下载需带 Authorization，走 blob 再触发浏览器保存
export const downloadFile = async (id: number, fileName: string) => {
  const blob = await request.get<unknown, Blob>(`/api/v1/files/${id}/download`, {
    responseType: 'blob',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  a.click()
  URL.revokeObjectURL(url)
}
