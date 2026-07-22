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

// 生成模式：crud=单表（默认）、tree=树表、sub=主子表
export type CodegenTplType = 'crud' | 'tree' | 'sub'

export type CodegenTreeConfig = {
  parent_field: string // 父级字段（如 parent_id）
  name_field: string // 显示字段（如 name），用作树节点标题
  sort_field?: string // 可选排序字段（如 sort）
}

export type CodegenSubConfig = {
  table: string // 子表表名
  fk_field: string // 子表中指向主表 id 的外键列
}

export type CodegenRequest = {
  table: string
  module: string
  title: string
  tpl_type?: CodegenTplType
  tree?: CodegenTreeConfig
  sub?: CodegenSubConfig
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
