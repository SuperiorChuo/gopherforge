import assert from 'node:assert/strict'
import test from 'node:test'
import {
  canAcquireRefreshLease,
  createRefreshCoordinator,
  createRefreshLease,
  hasUsableTokenPair,
  ownsRefreshLease,
  parseRefreshLease,
  parseTokenPairSnapshot,
  resolveTokenPair,
  sameTokenPair,
  tokenPairChanged,
} from '../src/utils/request-refresh.ts'

test('token pair helpers distinguish usable, changed, and equal sessions', () => {
  const previous = { access: 'access-1', refresh: 'refresh-1' }
  assert.equal(hasUsableTokenPair(previous), true)
  assert.equal(hasUsableTokenPair({ access: '', refresh: 'refresh-1' }), false)
  assert.equal(tokenPairChanged(previous, previous), false)
  assert.equal(tokenPairChanged({ access: 'access-2', refresh: 'refresh-1' }, previous), true)
  assert.equal(tokenPairChanged({ access: '', refresh: 'refresh-2' }, previous), false)
  assert.equal(sameTokenPair(previous, { access: 'access-1', refresh: 'refresh-1' }), true)
})

test('resolveTokenPair prevents torn reads while honoring direct legacy overwrites', () => {
  const snap = JSON.stringify({ access: 'a2', refresh: 'r2' })
  assert.deepEqual(parseTokenPairSnapshot(snap), { access: 'a2', refresh: 'r2' })
  assert.equal(parseTokenPairSnapshot('not-json'), null)
  assert.equal(parseTokenPairSnapshot('{"access":"a2"}'), null)

  // Mid-write: snapshot & access_token updated, refresh_token still old → atomic snapshot wins
  assert.deepEqual(resolveTokenPair('a2', 'r1', snap), { access: 'a2', refresh: 'r2' })
  // External test/fixture overwrites both raw keys directly → raw keys win
  assert.deepEqual(resolveTokenPair('a0', 'r0', snap), { access: 'a0', refresh: 'r0' })
  // Cleared snapshot + empty raw keys → unusable empty pair
  assert.deepEqual(resolveTokenPair(null, null, null), { access: '', refresh: '' })
})

test('refresh lease parsing fails closed and acquisition honors owner, nonce, and expiry', () => {
  const lease = createRefreshLease('tab-a', 1_000, 20_000)
  assert.deepEqual(lease, { owner: 'tab-a', expiresAt: 21_000 })
  assert.deepEqual(parseRefreshLease(JSON.stringify(lease)), lease)
  assert.deepEqual(parseRefreshLease(lease), lease)
  assert.equal(parseRefreshLease('not-json'), null)
  assert.equal(parseRefreshLease('{"owner":"tab-a"}'), null)
  assert.equal(parseRefreshLease('{"owner":"tab-a","expiresAt":"later"}'), null)

  assert.equal(canAcquireRefreshLease(null, 'tab-b', 1_000), true)
  assert.equal(canAcquireRefreshLease(JSON.stringify(lease), 'tab-b', 20_999), false)
  assert.equal(canAcquireRefreshLease(JSON.stringify(lease), 'tab-a', 1_000), true)
  assert.equal(canAcquireRefreshLease(JSON.stringify(lease), 'tab-b', 21_000), true)
  assert.equal(ownsRefreshLease(JSON.stringify(lease), 'tab-a'), true)
  assert.equal(ownsRefreshLease(JSON.stringify(lease), 'tab-b'), false)

  const withNonce = createRefreshLease('tab-a', 1_000, 20_000, 'n-1')
  assert.equal(ownsRefreshLease(JSON.stringify(withNonce), 'tab-a', 'n-1'), true)
  assert.equal(ownsRefreshLease(JSON.stringify(withNonce), 'tab-a', 'n-2'), false)
})

function createSharedOriginSim(initial) {
  const store = new Map()
  const listeners = new Set()
  const setTokens = (pair) => {
    store.set('auth_token_pair', JSON.stringify(pair))
    store.set('access_token', pair.access)
    store.set('refresh_token', pair.refresh)
    for (const fn of [...listeners]) fn()
  }
  const readTokens = () =>
    resolveTokenPair(
      store.get('access_token') ?? null,
      store.get('refresh_token') ?? null,
      store.get('auth_token_pair') ?? null,
    )
  if (initial) setTokens(initial)

  const waitForTokenUpdate = (previous, timeoutMs) => {
    const cur = readTokens()
    if (tokenPairChanged(cur, previous)) return Promise.resolve(cur)
    return new Promise((resolve) => {
      let done = false
      const finish = () => {
        if (done) return
        done = true
        clearTimeout(timer)
        listeners.delete(onChange)
        const updated = readTokens()
        resolve(tokenPairChanged(updated, previous) ? updated : null)
      }
      const onChange = () => {
        if (tokenPairChanged(readTokens(), previous)) finish()
      }
      listeners.add(onChange)
      const timer = setTimeout(finish, timeoutMs)
    })
  }

  return { store, setTokens, readTokens, waitForTokenUpdate }
}

test('5 concurrent tabs without Web Locks execute only 1 refresh via contention-confirmed lease', async () => {
  const initial = { access: 'a-old', refresh: 'r-old' }
  const rotated = { access: 'a-new', refresh: 'r-new' }
  const origin = createSharedOriginSim(initial)
  let refreshCalls = 0
  let nonceSeq = 0

  const tabs = ['tab-1', 'tab-2', 'tab-3', 'tab-4', 'tab-5'].map((tabID) =>
    createRefreshCoordinator({
      tabID,
      readTokenPair: origin.readTokens,
      writeTokenPair: origin.setTokens,
      readLease: () => origin.store.get('lock') ?? null,
      writeLease: (raw) => origin.store.set('lock', raw),
      clearLease: () => origin.store.delete('lock'),
      waitForTokenUpdate: origin.waitForTokenUpdate,
      getWebLocks: () => null,
      nextNonce: () => `n-${++nonceSeq}`,
      leaseConfirmMs: 15,
      pollSliceMs: 30,
      waitMs: 2_000,
      requestRefresh: async (rt) => {
        refreshCalls += 1
        assert.equal(rt, 'r-old')
        await new Promise((r) => setTimeout(r, 40))
        return rotated
      },
    }),
  )

  const results = await Promise.all(tabs.map((t) => t.refreshTokenPair(initial)))
  assert.equal(refreshCalls, 1, 'fallback storage lease must only invoke /refresh once')
  for (const r of results) {
    assert.deepEqual(r, rotated)
  }
  assert.equal(origin.store.get('lock'), undefined, 'winning tab must release lease')
})

test('concurrent tabs with Web Locks + late-arriving peer execute only 1 refresh', async () => {
  const initial = { access: 'a-1', refresh: 'r-1' }
  const rotated = { access: 'a-2', refresh: 'r-2' }
  const origin = createSharedOriginSim(initial)
  let refreshCalls = 0

  // Simple serialized Web Lock implementation
  let lockChain = Promise.resolve()
  const webLocks = {
    request(_name, cb) {
      const next = lockChain.then(() => cb())
      lockChain = next.then(
        () => {},
        () => {},
      )
      return next
    },
  }

  const makeTab = (tabID) =>
    createRefreshCoordinator({
      tabID,
      readTokenPair: origin.readTokens,
      writeTokenPair: origin.setTokens,
      readLease: () => origin.store.get('lock') ?? null,
      writeLease: (raw) => origin.store.set('lock', raw),
      clearLease: () => origin.store.delete('lock'),
      waitForTokenUpdate: origin.waitForTokenUpdate,
      getWebLocks: () => webLocks,
      requestRefresh: async (rt) => {
        refreshCalls += 1
        assert.equal(rt, 'r-1')
        await new Promise((r) => setTimeout(r, 30))
        return rotated
      },
    })

  const tabA = makeTab('tab-a')
  const tabB = makeTab('tab-b')
  const tabLate = makeTab('tab-late')

  const [rA, rB] = await Promise.all([tabA.refreshTokenPair(initial), tabB.refreshTokenPair(initial)])
  // Late peer that captured `initial` before refresh finished arrives afterwards
  const rLate = await tabLate.refreshTokenPair(initial)

  assert.equal(refreshCalls, 1)
  assert.deepEqual(rA, rotated)
  assert.deepEqual(rB, rotated)
  assert.deepEqual(rLate, rotated)
})

test('same-tab coalescing separates stale pair from subsequent rotation', async () => {
  const pair1 = { access: 'a-1', refresh: 'r-1' }
  const pair2 = { access: 'a-2', refresh: 'r-2' }
  const pair3 = { access: 'a-3', refresh: 'r-3' }
  const origin = createSharedOriginSim(pair1)
  let calls = 0

  const coord = createRefreshCoordinator({
    tabID: 'single',
    readTokenPair: origin.readTokens,
    writeTokenPair: origin.setTokens,
    readLease: () => origin.store.get('lock') ?? null,
    writeLease: (raw) => origin.store.set('lock', raw),
    clearLease: () => origin.store.delete('lock'),
    waitForTokenUpdate: origin.waitForTokenUpdate,
    leaseConfirmMs: 0,
    requestRefresh: async (rt) => {
      calls += 1
      await new Promise((r) => setTimeout(r, 15))
      if (rt === 'r-1') return pair2
      if (rt === 'r-2') return pair3
      throw new Error(`unexpected refresh token ${rt}`)
    },
  })

  // Two parallel 401s on pair1 coalesce into 1 refresh
  const [res1a, res1b] = await Promise.all([
    coord.refreshTokenPair(pair1),
    coord.refreshTokenPair(pair1),
  ])
  assert.equal(calls, 1)
  assert.deepEqual(res1a, pair2)
  assert.deepEqual(res1b, pair2)

  // Later 401 on pair2 triggers a fresh rotation to pair3
  const res2 = await coord.refreshTokenPair(pair2)
  assert.equal(calls, 2)
  assert.deepEqual(res2, pair3)
})

test('401 recovery window adopts winning tab rotation instead of forcing logout', async () => {
  const initial = { access: 'a-1', refresh: 'r-1' }
  const rotated = { access: 'a-2', refresh: 'r-2' }
  const origin = createSharedOriginSim(initial)

  const coord = createRefreshCoordinator({
    tabID: 'loser',
    readTokenPair: origin.readTokens,
    writeTokenPair: origin.setTokens,
    readLease: () => origin.store.get('lock') ?? null,
    writeLease: (raw) => origin.store.set('lock', raw),
    clearLease: () => origin.store.delete('lock'),
    waitForTokenUpdate: origin.waitForTokenUpdate,
    leaseConfirmMs: 0,
    recoveryWaitMs: 300,
    requestRefresh: async () => {
      // Simulate winner publishing rotated pair 20ms after loser gets 401
      setTimeout(() => origin.setTokens(rotated), 20)
      const err = new Error('refresh token already used')
      err.response = { status: 401 }
      throw err
    },
  })

  const recovered = await coord.refreshTokenPair(initial)
  assert.deepEqual(recovered, rotated)
})

test('expired lease from crashed tab is taken over safely', async () => {
  const initial = { access: 'a-1', refresh: 'r-1' }
  const rotated = { access: 'a-2', refresh: 'r-2' }
  const origin = createSharedOriginSim(initial)
  // Dead tab left an expired lease behind
  origin.store.set('lock', JSON.stringify({ owner: 'dead-tab', expiresAt: 500, nonce: 'old' }))

  const coord = createRefreshCoordinator({
    tabID: 'survivor',
    readTokenPair: origin.readTokens,
    writeTokenPair: origin.setTokens,
    readLease: () => origin.store.get('lock') ?? null,
    writeLease: (raw) => origin.store.set('lock', raw),
    clearLease: () => origin.store.delete('lock'),
    waitForTokenUpdate: origin.waitForTokenUpdate,
    now: () => 1_000,
    leaseConfirmMs: 0,
    requestRefresh: async () => rotated,
  })

  const result = await coord.refreshTokenPair(initial)
  assert.deepEqual(result, rotated)
  assert.equal(origin.store.get('lock'), undefined)
})
