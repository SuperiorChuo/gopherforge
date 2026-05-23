import { request } from '@/utils/request';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

// 文件列表请求
export interface FileListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  file_type?: string;
}

export type FileListResponse = Schema<'FileListResponse'>;
export type FileItem = Schema<'FileItem'>;
export type FileStats = Schema<'FileStats'>;
export type FileHashCheckResponse = Schema<'FileHashCheck'>;
export type UploadFileResponse = Schema<'FileItem'>;
export type MultipleUploadResponse = Schema<'MultipleUploadResponse'>;

const Api = {
  Upload: '/files/upload',
  UploadMultiple: '/files/upload/multiple',
  GetFileList: '/files',
  GetMyFiles: '/files/my',
  GetFileStats: '/files/stats',
  GetFile: '/files',
  Download: '/files',
  Preview: '/files',
  CheckHash: '/files/hash/check',
  DeleteFile: '/files',
  DeleteFiles: '/files/batch',
};

/**
 * 上传单个文件
 */
export function uploadFile(file: File, hash?: string) {
  const formData = new FormData();
  formData.append('file', file);
  if (hash) {
    formData.append('hash', hash);
  }
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
  return request.post<MultipleUploadResponse>({
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
 * 检查文件哈希是否已存在
 */
export function checkFileHash(hash: string) {
  return request.get<FileHashCheckResponse>({
    url: Api.CheckHash,
    params: { hash },
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
