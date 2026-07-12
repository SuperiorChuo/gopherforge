import request from '@/utils/request'
import type { PageRequest, PageResponse, FileRecord } from '@/types'

type FileListParams = PageRequest & { keyword?: string }

export const getFileList = (params: FileListParams) =>
  request.get<unknown, PageResponse<FileRecord>>('/api/v1/system/files', { params })

export const uploadFile = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post<unknown, FileRecord>('/api/v1/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export const deleteFile = (id: number) =>
  request.delete<unknown, void>(`/api/v1/system/files/${id}`)
