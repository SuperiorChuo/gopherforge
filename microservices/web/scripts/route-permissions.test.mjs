import assert from 'node:assert/strict'
import test from 'node:test'
import { ROUTE_PERMISSIONS, ROUTE_PERMISSION_EXEMPT } from '../src/router/route-permissions.ts'

const permissionPattern = /^[a-z][a-z0-9-]*(?::[a-z0-9-]+)+$/

test('route permission tables contain valid, disjoint entries', () => {
  const guarded = Object.entries(ROUTE_PERMISSIONS)
  const exempt = Object.entries(ROUTE_PERMISSION_EXEMPT)
  assert.ok(guarded.length > 0)
  assert.ok(exempt.length > 0)

  const guardedPaths = new Set(Object.keys(ROUTE_PERMISSIONS))
  for (const [path, permission] of guarded) {
    assert.match(path, /^\//)
    assert.match(permission, permissionPattern)
  }
  for (const [path, reason] of exempt) {
    assert.match(path, /^\//)
    assert.ok(reason.trim())
    assert.equal(guardedPaths.has(path), false)
  }

  assert.equal(ROUTE_PERMISSIONS['/system/user'], 'system:user:list')
  assert.equal(ROUTE_PERMISSIONS['/bpm/definitions'], 'bpm:definition:list')
  assert.equal(ROUTE_PERMISSION_EXEMPT['/dashboard/index']?.length > 0, true)
})
