import { request } from '@/utils/request';

// 文件列表请求
export interface FileListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  file_type?: string;
}

// 文件列表响应
export interface FileListResponse {
  list: FileItem[];
  total: number;
  page: number;
  page_size: number;
}

// 文件项
export interface FileItem {
  id: number;
  user_id: number;
  file_name: string;
  file_path: string;
  file_size: number;
  file_type: string;
  mime_type: string;
  extension: string;
  storage_type: string;
  url: string;
  hash?: string;
  created_at: string;
  updated_at: string;
}

// 文件统计
export interface FileStats {
  total: number;
  total_count?: number;
  total_size: number;
  by_type: Record<string, number | { count: number; size: number }>;
}

// 上传文件响应
export interface UploadFileResponse {
  id: number;
  file_name: string;
  url: string;
  file_size: number;
}

const Api = {
  Upload: '/files/upload',
  UploadMultiple: '/files/upload/multiple',
  GetFileList: '/files',
  GetMyFiles: '/files/my',
  GetFileStats: '/files/stats',
  GetFile: '/files',
  Download: '/files',
  Preview: '/files',
  DeleteFile: '/files',
  DeleteFiles: '/files/batch',
};

/**
 * 上传单个文件
 */
export function uploadFile(file: File) {
  const formData = new FormData();
  formData.append('file', file);
  return request.post<UploadFileResponse>({
    url: Api.Upload,
    data: formData,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
}

/**
 * 批量上传文件
 */
export function uploadMultipleFiles(files: File[]) {
  const formData = new FormData();
  files.forEach((file) => {
    formData.append('files', file);
  });
  return request.post<UploadFileResponse[]>({
    url: Api.UploadMultiple,
    data: formData,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
}

/**
 * 获取文件列表
 */
export function getFileList(params: FileListRequest) {
  return request.get<FileListResponse>({
    url: Api.GetFileList,
    params,
  });
}

/**
 * 获取我的文件
 */
export function getMyFiles(params: FileListRequest) {
  return request.get<FileListResponse>({
    url: Api.GetMyFiles,
    params,
  });
}

/**
 * 获取文件统计
 */
export function getFileStats() {
  return request.get<FileStats>({
    url: Api.GetFileStats,
  });
}

/**
 * 获取文件详情
 */
export function getFile(id: number) {
  return request.get<FileItem>({
    url: `${Api.GetFile}/${id}`,
  });
}

/**
 * 下载文件
 */
export function downloadFile(id: number) {
  return request.get<Blob>({
    url: `${Api.Download}/${id}/download`,
    responseType: 'blob',
  }, {
    isTransformResponse: false,
  });
}

/**
 * 预览文件
 */
export function previewFile(id: number) {
  return request.get<Blob>({
    url: `${Api.Preview}/${id}/preview`,
    responseType: 'blob',
  }, {
    isTransformResponse: false,
  });
}

/**
 * 删除文件
 */
export function deleteFile(id: number) {
  return request.delete({
    url: `${Api.DeleteFile}/${id}`,
  });
}

/**
 * 批量删除文件
 */
export function deleteFiles(ids: number[]) {
  return request.delete({
    url: Api.DeleteFiles,
    data: { ids },
  });
}
