export type TableQueryState<T> = {
  list: T[]
  total: number
  loading: boolean
  error: boolean
}

export type TableQueryResult<T> = {
  list: T[]
  total: number
}

export function createInitialTableQueryState<T>(): TableQueryState<T> {
  return { list: [], total: 0, loading: false, error: false }
}

/** 每次发起列表请求前取号；返回值必须和当时的 current 比较，过期响应直接丢。 */
export function nextTableQueryRequestID(current: number): number {
  return current + 1
}

export function isCurrentTableQueryRequest(requestID: number, current: number): boolean {
  return requestID === current
}

/** 非静默刷新：开 loading，清 error，旧 list/total 保留。 */
export function beginTableQueryReload<T>(
  current: TableQueryState<T>,
  options?: { silent?: boolean },
): TableQueryState<T> {
  if (options?.silent) return current
  return { ...current, loading: true, error: false }
}

/** 只有当前请求才落结果；过期响应原样返回，避免旧列表盖新列表。 */
export function applyTableQueryResult<T>(
  current: TableQueryState<T>,
  requestID: number,
  latestID: number,
  result: TableQueryResult<T>,
): TableQueryState<T> {
  if (!isCurrentTableQueryRequest(requestID, latestID)) return current
  return { list: result.list, total: result.total, loading: false, error: false }
}

/** 只有当前请求才记失败；静默刷新失败不改 error，避免轮询把已展示错误冲掉。 */
export function applyTableQueryError<T>(
  current: TableQueryState<T>,
  requestID: number,
  latestID: number,
  options?: { silent?: boolean },
): TableQueryState<T> {
  if (!isCurrentTableQueryRequest(requestID, latestID)) return current
  return { ...current, loading: false, error: options?.silent ? current.error : true }
}

/** 乐观更新本地行，不改 total/loading/error。 */
export function patchTableQueryList<T>(
  current: TableQueryState<T>,
  updater: (list: T[]) => T[],
): TableQueryState<T> {
  return { ...current, list: updater(current.list) }
}
