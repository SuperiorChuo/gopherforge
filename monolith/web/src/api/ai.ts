/** 单体版不内置 AI 服务；保留导出以免页面 import 断裂。
 * 返回类型与微服务版对齐，让 cherry-pick 过来的页面能通过编译；运行时仍抛错。 */
export async function getLogsInsight(_payload?: unknown): Promise<{ report: string }> {
  throw new Error('单体版未启用 AI 能力，请使用微服务产品线')
}

export async function compose(_payload?: unknown): Promise<{ content: string }> {
  throw new Error('单体版未启用 AI 能力，请使用微服务产品线')
}
