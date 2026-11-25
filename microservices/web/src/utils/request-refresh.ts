export type TokenPair = {
  access: string
  refresh: string
}

export type RefreshLease = {
  owner: string
  expiresAt: number
  nonce?: string
}

export type AuthLockManager = {
  request<T>(name: string, callback: () => Promise<T>): Promise<T>
}

export const hasUsableTokenPair = (pair: TokenPair) => Boolean(pair.access && pair.refresh)

export const tokenPairChanged = (current: TokenPair, previous: TokenPair) =>
  hasUsableTokenPair(current) &&
  (current.access !== previous.access || current.refresh !== previous.refresh)

export const sameTokenPair = (left: TokenPair, right: TokenPair) =>
  left.access === right.access && left.refresh === right.refresh

/**
 * 解析原子保存的 token pair 快照（防跨标签在 setTokens 双键写入间隙读到半更新态）。
 */
export function parseTokenPairSnapshot(raw: unknown): TokenPair | null {
  if (typeof raw === 'string') {
    if (!raw) return null
    try {
      raw = JSON.parse(raw)
    } catch {
      return null
    }
  }
  if (!raw || typeof raw !== 'object') return null
  const snap = raw as Partial<TokenPair>
  if (typeof snap.access !== 'string' || typeof snap.refresh !== 'string') return null
  if (!snap.access || !snap.refresh) return null
  return { access: snap.access, refresh: snap.refresh }
}

/**
 * 读取完整 token pair：
 * - 若存在原子快照且与任一单键匹配（说明正处于 setTokens 双键更新窗口或正常同步态），
 *   优先返回原子快照，消除「新 access + 旧 refresh」撕裂读。
 * - 若外部脚本（如测试 fixture / 旧代码）直接改写了两个单键导致与快照完全无关，
 *   则以单键为准，保持 100% 向后兼容。
 */
export function resolveTokenPair(
  rawAccess: string | null,
  rawRefresh: string | null,
  rawSnapshot: string | null,
): TokenPair {
  const access = rawAccess || ''
  const refresh = rawRefresh || ''
  const snap = parseTokenPairSnapshot(rawSnapshot)
  if (snap && (access === snap.access || refresh === snap.refresh)) {
    return snap
  }
  return { access, refresh }
}

export function parseRefreshLease(value: unknown): RefreshLease | null {
  if (typeof value === 'string') {
    if (!value) return null
    try {
      value = JSON.parse(value)
    } catch {
      return null
    }
  }
  if (!value || typeof value !== 'object') return null
  const lease = value as Partial<RefreshLease>
  if (typeof lease.owner !== 'string' || typeof lease.expiresAt !== 'number') return null
  if (lease.nonce !== undefined && typeof lease.nonce !== 'string') return null
  return lease.nonce !== undefined
    ? { owner: lease.owner, expiresAt: lease.expiresAt, nonce: lease.nonce }
    : { owner: lease.owner, expiresAt: lease.expiresAt }
}

export const createRefreshLease = (
  owner: string,
  now: number,
  ttlMs: number,
  nonce?: string,
): RefreshLease =>
  nonce !== undefined
    ? { owner, expiresAt: now + ttlMs, nonce }
    : { owner, expiresAt: now + ttlMs }

export const canAcquireRefreshLease = (value: unknown, owner: string, now: number) => {
  const current = parseRefreshLease(value)
  return !current || current.owner === owner || current.expiresAt <= now
}

export const ownsRefreshLease = (value: unknown, owner: string, nonce?: string) => {
  const lease = parseRefreshLease(value)
  if (!lease || lease.owner !== owner) return false
  if (nonce !== undefined && lease.nonce !== nonce) return false
  return true
}

export type RefreshCoordinatorDeps = {
  tabID: string
  readTokenPair: () => TokenPair
  writeTokenPair: (pair: TokenPair) => void
  readLease: () => string | null
  writeLease: (raw: string) => void
  clearLease: () => void
  waitForTokenUpdate: (previous: TokenPair, timeoutMs: number) => Promise<TokenPair | null>
  requestRefresh: (refreshToken: string) => Promise<TokenPair>
  getWebLocks?: () => AuthLockManager | null
  lockName?: string
  now?: () => number
  sleep?: (ms: number) => Promise<void>
  nextNonce?: () => string
  waitMs?: number
  recoveryWaitMs?: number
  leaseTtlMs?: number
  leaseConfirmMs?: number
  pollSliceMs?: number
}

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

/**
 * 跨标签 + 单标签并发 Token 刷新协调器：
 * 1. 同标签按 TokenPair 精准归并 in-flight 刷新，不误复用已过期旧对的 Promise；
 * 2. 安全上下文优先使用 Web Locks API（navigator.locks）实现 OS 级互斥；
 * 3. 非安全上下文（如内网 HTTP）使用两阶段争用确认租约（写入 owner+nonce →
 *    短窗退让 → 二次校验持有权与 token 变化），消除 localStorage check-then-set 双发竞态；
 * 4. 极端暂停下若仍触发 401，等待短暂恢复窗口接收胜出标签的新 token 对。
 */
export function createRefreshCoordinator(deps: RefreshCoordinatorDeps) {
  const now = deps.now ?? (() => Date.now())
  const sleep = deps.sleep ?? defaultSleep
  const nextNonce = deps.nextNonce ?? (() => `${now()}-${Math.random().toString(36).slice(2, 10)}`)
  const lockName = deps.lockName ?? 'go-admin-kit-auth-refresh'
  const waitMs = deps.waitMs ?? 10_000
  const recoveryWaitMs = deps.recoveryWaitMs ?? 1_500
  const leaseTtlMs = deps.leaseTtlMs ?? 20_000
  const leaseConfirmMs = deps.leaseConfirmMs ?? 25
  const pollSliceMs = deps.pollSliceMs ?? 500

  let inFlight: { forPair: TokenPair; promise: Promise<TokenPair> } | null = null

  async function tryAcquireLeaseConfirmed(previous: TokenPair): Promise<string | null> {
    const ts = now()
    if (!canAcquireRefreshLease(deps.readLease(), deps.tabID, ts)) return null

    const nonce = nextNonce()
    const lease = createRefreshLease(deps.tabID, ts, leaseTtlMs, nonce)
    deps.writeLease(JSON.stringify(lease))

    if (leaseConfirmMs > 0) {
      await sleep(leaseConfirmMs)
    }

    // 确认窗口后二次校验：若已有其他标签覆写租约，主动让出避免双发。
    if (!ownsRefreshLease(deps.readLease(), deps.tabID, nonce)) {
      return null
    }
    if (tokenPairChanged(deps.readTokenPair(), previous)) {
      deps.clearLease()
      return null
    }
    return nonce
  }

  function releaseLease(nonce: string) {
    if (ownsRefreshLease(deps.readLease(), deps.tabID, nonce)) {
      deps.clearLease()
    }
  }

  async function requestFreshTokenPair(used: TokenPair): Promise<TokenPair> {
    try {
      const next = await deps.requestRefresh(used.refresh)
      if (!hasUsableTokenPair(next)) {
        throw new Error('refresh response did not include a token pair')
      }

      // 不覆盖在刷新期间由其他登录/刷新写入的更新会话。
      const current = deps.readTokenPair()
      if (!sameTokenPair(current, used)) {
        if (hasUsableTokenPair(current)) return current
        throw new Error('token state changed while refreshing')
      }

      deps.writeTokenPair(next)
      return next
    } catch (error) {
      const current = deps.readTokenPair()
      if (tokenPairChanged(current, used)) return current

      const status =
        (error as { response?: { status?: number }; status?: number })?.response?.status ??
        (error as { status?: number })?.status
      if (status === 401) {
        const updated = await deps.waitForTokenUpdate(used, recoveryWaitMs)
        if (updated) return updated
      }
      throw error
    }
  }

  async function refreshWithStorageLease(previous: TokenPair): Promise<TokenPair> {
    const deadline = now() + waitMs
    while (now() < deadline) {
      const current = deps.readTokenPair()
      if (tokenPairChanged(current, previous)) return current
      if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')

      const ownedNonce = await tryAcquireLeaseConfirmed(previous)
      if (ownedNonce) {
        try {
          const latest = deps.readTokenPair()
          if (tokenPairChanged(latest, previous)) return latest
          if (!hasUsableTokenPair(latest)) throw new Error('refresh token is missing')
          return await requestFreshTokenPair(latest)
        } finally {
          releaseLease(ownedNonce)
        }
      }

      const afterCheck = deps.readTokenPair()
      if (tokenPairChanged(afterCheck, previous)) return afterCheck

      const remaining = deadline - now()
      if (remaining <= 0) break
      const updated = await deps.waitForTokenUpdate(previous, Math.min(pollSliceMs, remaining))
      if (updated) return updated
    }
    throw new Error('timed out waiting for another page to refresh the session')
  }

  async function refreshAcrossContexts(previous: TokenPair): Promise<TokenPair> {
    const locks = deps.getWebLocks?.() ?? null
    if (locks) {
      return locks.request(lockName, async () => {
        const current = deps.readTokenPair()
        if (tokenPairChanged(current, previous)) return current
        if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')
        return requestFreshTokenPair(current)
      })
    }
    return refreshWithStorageLease(previous)
  }

  async function refreshTokenPair(previous: TokenPair): Promise<TokenPair> {
    const current = deps.readTokenPair()
    if (tokenPairChanged(current, previous)) return current

    if (inFlight) {
      if (sameTokenPair(inFlight.forPair, previous)) {
        return inFlight.promise
      }
      // 前序刷新针对的是旧 token 对：等其结束再重新判断当前会话是否已轮换。
      await inFlight.promise.catch(() => {})
      const latest = deps.readTokenPair()
      if (tokenPairChanged(latest, previous)) return latest
    }

    const promise = refreshAcrossContexts(previous)
    const entry = { forPair: previous, promise }
    inFlight = entry
    void promise.then(
      () => {
        if (inFlight === entry) inFlight = null
      },
      () => {
        if (inFlight === entry) inFlight = null
      },
    )
    return promise
  }

  return { refreshTokenPair }
}
