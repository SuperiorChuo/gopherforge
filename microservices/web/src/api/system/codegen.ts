import axios from 'axios'
import request from '@/utils/request'

const CODEGEN_REQUEST_TIMEOUT = 90_000

export type CodegenCapabilities = {
  preview_enabled: boolean
  download_enabled: boolean
  write_enabled: boolean
}

export type CodegenTable = {
  name: string
  comment: string
  primary_key: string
  column_count: number
  relation_count: number
}

export type CodegenColumn = {
  name: string
  db_type: string
  go_type: string
  ts_type: string
  nullable: boolean
  primary_key: boolean
  go_field: string
  label: string
  comment: string
  default_value?: string
}

export type CodegenFormComponent = 'input' | 'textarea' | 'number' | 'switch' | 'date' | 'datetime' | 'select'

export type CodegenFieldConfig = {
  name: string
  label: string
  in_list: boolean
  in_search: boolean
  in_form: boolean
  required: boolean
  dict_type: string
  component?: CodegenFormComponent
}

export type CodegenTplType = 'crud' | 'tree' | 'sub'

export type CodegenTreeConfig = {
  parent_field: string
  name_field: string
  sort_field?: string
}

export type CodegenSubConfig = {
  table: string
  fk_field: string
  fields: CodegenFieldConfig[]
}

export type CodegenM2MConfig = {
  name: string
  join_table: string
  fk_field: string
  target_table: string
  target_fk: string
  display_field: string
  label: string
}

export type CodegenRelationKind = 'many_to_one' | 'one_to_many' | 'many_to_many'

export type CodegenForeignKey = {
  name: string
  field: string
  target_table: string
  target_field: string
}

export type CodegenRelationCandidate = {
  kind: CodegenRelationKind
  source_table: string
  target_table: string
  join_table?: string
  fk_field: string
  target_fk?: string
}

export type CodegenSchema = {
  name: string
  comment: string
  primary_key: string
  columns: CodegenColumn[]
  foreign_keys: CodegenForeignKey[]
  relations: CodegenRelationCandidate[]
}

export type CodegenRequest = {
  table: string
  module: string
  title: string
  tpl_type: CodegenTplType
  tree?: CodegenTreeConfig
  sub?: CodegenSubConfig
  fields: CodegenFieldConfig[]
  m2ms?: CodegenM2MConfig[]
}

export type CodegenArtifactOperation = 'create' | 'patch'
export type CodegenArtifactStatus = 'ready' | 'conflict' | 'invalid'

export type CodegenArtifact = {
  path: string
  operation: CodegenArtifactOperation
  content?: string
  diff?: string
  expected_hash?: string
  result_hash: string
  status: CodegenArtifactStatus
}

export type CodegenDiagnostic = {
  severity: 'error' | 'warning'
  code: string
  message: string
  path?: string
}

export type CodegenPlan = {
  digest: string
  request: CodegenRequest
  schemas: CodegenSchema[]
  artifacts: CodegenArtifact[]
  diagnostics: CodegenDiagnostic[]
}

export type CodegenWriteResult = { digest: string; created: string[]; patched: string[] }

export class CodegenHTTPError extends Error {
  status?: number

  constructor(message: string, status?: number) {
    super(message)
    this.name = 'CodegenHTTPError'
    this.status = status
  }
}

export function getCodegenCapabilities() {
  return request.get('/api/v1/codegen/capabilities') as Promise<CodegenCapabilities>
}

export function listCodegenTables() {
  return request.get('/api/v1/codegen/tables') as Promise<{ list: CodegenTable[]; total: number }>
}

export function getCodegenSchema(table: string) {
  return request.get(`/api/v1/codegen/tables/${encodeURIComponent(table)}/schema`) as Promise<CodegenSchema>
}

export function listCodegenColumns(table: string) {
  return request.get(`/api/v1/codegen/tables/${encodeURIComponent(table)}/columns`) as Promise<{ list: CodegenColumn[]; total: number }>
}

export function previewCodegen(codegenRequest: CodegenRequest) {
  return request.post('/api/v1/codegen/preview', codegenRequest, {
    timeout: CODEGEN_REQUEST_TIMEOUT,
  }) as Promise<CodegenPlan>
}

export async function downloadCodegen(codegenRequest: CodegenRequest, expectedDigest: string) {
  try {
    const payload = await request.post('/api/v1/codegen/download', {
      request: codegenRequest,
      expected_digest: expectedDigest,
    }, { responseType: 'blob', silent: true, timeout: CODEGEN_REQUEST_TIMEOUT }) as Blob
    if (payload.type.includes('json')) throw await blobError(payload)
    return payload
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.data instanceof Blob && error.response.data.type.includes('json')) {
      throw await blobError(error.response.data, error.response.status)
    }
    throw error
  }
}

export function writeCodegen(codegenRequest: CodegenRequest, expectedDigest: string, confirmation: string) {
  return request.post('/api/v1/codegen/write', {
    request: codegenRequest,
    expected_digest: expectedDigest,
    confirmation,
  }, { silent: true, timeout: CODEGEN_REQUEST_TIMEOUT }) as Promise<CodegenWriteResult>
}

async function blobError(blob: Blob, status?: number) {
  try {
    const body = JSON.parse(await blob.text()) as { message?: string }
    return new CodegenHTTPError(body.message || '下载失败', status)
  } catch {
    return new CodegenHTTPError('下载失败', status)
  }
}
