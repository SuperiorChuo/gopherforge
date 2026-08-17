import assert from 'node:assert/strict'
import test from 'node:test'
import {
  applyTableQueryError,
  applyTableQueryResult,
  beginTableQueryReload,
  createInitialTableQueryState,
  isCurrentTableQueryRequest,
  nextTableQueryRequestID,
  patchTableQueryList,
} from '../src/hooks/table-query.ts'

test('nextTableQueryRequestID 递增，过期号不相等', () => {
  const first = nextTableQueryRequestID(0)
  const second = nextTableQueryRequestID(first)
  assert.equal(first, 1)
  assert.equal(second, 2)
  assert.equal(isCurrentTableQueryRequest(first, second), false)
  assert.equal(isCurrentTableQueryRequest(second, second), true)
})

test('beginTableQueryReload 非静默开 loading 并保留旧数据', () => {
  const current = { list: [{ id: 1 }], total: 1, loading: false, error: true }
  assert.deepEqual(beginTableQueryReload(current), {
    list: [{ id: 1 }],
    total: 1,
    loading: true,
    error: false,
  })
  assert.equal(beginTableQueryReload(current, { silent: true }), current)
})

test('applyTableQueryResult 丢掉过期响应，当前请求覆盖列表', () => {
  const current = { list: [{ id: 1 }], total: 1, loading: true, error: false }
  assert.equal(
    applyTableQueryResult(current, 1, 2, { list: [{ id: 99 }], total: 99 }),
    current,
  )
  assert.deepEqual(
    applyTableQueryResult(current, 2, 2, { list: [{ id: 2 }], total: 3 }),
    { list: [{ id: 2 }], total: 3, loading: false, error: false },
  )
})

test('applyTableQueryError 过期失败不改状态；静默失败保留原 error', () => {
  const current = { list: [{ id: 1 }], total: 1, loading: true, error: false }
  assert.equal(applyTableQueryError(current, 1, 2), current)
  assert.deepEqual(applyTableQueryError(current, 2, 2), {
    list: [{ id: 1 }],
    total: 1,
    loading: false,
    error: true,
  })
  assert.deepEqual(applyTableQueryError(current, 2, 2, { silent: true }), {
    list: [{ id: 1 }],
    total: 1,
    loading: false,
    error: false,
  })
})

test('patchTableQueryList 只改 list，给乐观更新用', () => {
  const current = { list: [{ id: 1, status: 0 }, { id: 2, status: 1 }], total: 2, loading: false, error: false }
  assert.deepEqual(
    patchTableQueryList(current, (list) => list.map((row) => (row.id === 1 ? { ...row, status: 1 } : row))),
    { list: [{ id: 1, status: 1 }, { id: 2, status: 1 }], total: 2, loading: false, error: false },
  )
})

test('createInitialTableQueryState 是空列表未加载', () => {
  assert.deepEqual(createInitialTableQueryState(), {
    list: [],
    total: 0,
    loading: false,
    error: false,
  })
})
