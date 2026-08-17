import assert from 'node:assert/strict'
import test from 'node:test'
import {
  canAcquireRefreshLease,
  createRefreshLease,
  hasUsableTokenPair,
  ownsRefreshLease,
  parseRefreshLease,
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

test('refresh lease parsing fails closed and acquisition honors owner and expiry', () => {
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
})
