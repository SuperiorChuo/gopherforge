export type TokenPair = {
  access: string
  refresh: string
}

export type RefreshLease = {
  owner: string
  expiresAt: number
}

export const hasUsableTokenPair = (pair: TokenPair) => Boolean(pair.access && pair.refresh)

export const tokenPairChanged = (current: TokenPair, previous: TokenPair) =>
  hasUsableTokenPair(current) &&
  (current.access !== previous.access || current.refresh !== previous.refresh)

export const sameTokenPair = (left: TokenPair, right: TokenPair) =>
  left.access === right.access && left.refresh === right.refresh

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
  return { owner: lease.owner, expiresAt: lease.expiresAt }
}

export const createRefreshLease = (owner: string, now: number, ttlMs: number): RefreshLease => ({
  owner,
  expiresAt: now + ttlMs,
})

export const canAcquireRefreshLease = (value: unknown, owner: string, now: number) => {
  const current = parseRefreshLease(value)
  return !current || current.owner === owner || current.expiresAt <= now
}

export const ownsRefreshLease = (value: unknown, owner: string) =>
  parseRefreshLease(value)?.owner === owner
