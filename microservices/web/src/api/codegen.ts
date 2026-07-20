import request from '@/utils/request'

// 代码生成器（system-service /codegen/*，权限 system:codegen:*）

export type CodegenTable = { name: string }

export type CodegenColumn = {
  name: string
  db_type: string
  go_type: string
  ts_type: string
  nullable: boolean
  primary_key: boolean
  go_field: string
  label: string
}

export type CodegenFieldConfig = {
  name: string
  label: string
  in_list: boolean
  in_search: boolean
  in_form: boolean
  required: boolean
}

export type CodegenRequest = {
  table: string
  module: string
  title: string
  fields: CodegenFieldConfig[]
}

export type CodegenFile = { path: string; content: string }

export function listCodegenTables() {
  return request.get('/api/v1/codegen/tables') as Promise<{ list: CodegenTable[]; total: number }>
}

export function listCodegenColumns(table: string) {
  return request.get(`/api/v1/codegen/tables/${encodeURIComponent(table)}/columns`) as Promise<{ list: CodegenColumn[]; total: number }>
}

export function previewCodegen(req: CodegenRequest) {
  return request.post('/api/v1/codegen/preview', req) as Promise<{ files: CodegenFile[] }>
}

// 下载是二进制流，绕过统一 envelope 拦截器直接拿 blob
export function downloadCodegen(req: CodegenRequest) {
  return request.post('/api/v1/codegen/download', req, { responseType: 'blob' }) as Promise<Blob>
}
