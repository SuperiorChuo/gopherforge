import type { AgentDefinition, AgentDefinitionStatus } from '@/api/ai/agent'

/** 与服务端 utf8.RuneCountInString 对齐，不能把一个补充平面字符算成两个。 */
export const definitionTextLength = (value: string) => Array.from(value.trim()).length

/** PATCH 只允许管理字段，绝不回传列表里的租户、版本或只读标识。 */
export function definitionPayload(values: Record<string, unknown>): {
  name: string; description: string; status: AgentDefinitionStatus
} {
  if (typeof values.name !== 'string' || (values.description !== undefined && typeof values.description !== 'string')) {
    throw new Error('invalid-agent-definition')
  }
  const name = values.name.trim()
  const description = ((values.description as string | undefined) ?? '').trim()
  if (definitionTextLength(name) < 1 || definitionTextLength(name) > 100 || definitionTextLength(description) > 500 ||
      (values.status !== 'active' && values.status !== 'disabled')) {
    throw new Error('invalid-agent-definition')
  }
  return { name, description, status: values.status }
}

/** 禁用只阻断新运行；已存在 Run 的查看/取消仍由各自权限控制。 */
export function canRunDefinition(definition: Pick<AgentDefinition, 'status' | 'latest_version_id'>, canRun: boolean): boolean {
  return canRun && definition.status === 'active' && Number.isSafeInteger(definition.latest_version_id) && (definition.latest_version_id ?? 0) > 0
}
