/** 单体版不内置 AI 服务；保留导出以免页面 import 断裂。 */
export async function getLogsInsight(_payload?: unknown): Promise<never> {
  throw new Error('单体版未启用 AI 能力，请使用微服务产品线')
}

export async function compose(_payload?: unknown): Promise<never> {
  throw new Error('单体版未启用 AI 能力，请使用微服务产品线')
}
