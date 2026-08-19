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
  sameTableQueryParams,
  stabilizeTableQueryParams,
} from '../src/hooks/table-query.ts'

test('参数稳定器复用等值普通对象并识别全部顶层自有键变化', () => {
  assert.equal(sameTableQueryParams({}, {}), true)
  assert.equal(sameTableQueryParams({ status: '', page: 1 }, { status: '', page: 1 }), true)
  assert.equal(sameTableQueryParams({ status: '', page: 1 }, { status: 'active', page: 1 }), false)
  assert.equal(sameTableQueryParams(null, null), true)
  assert.equal(sameTableQueryParams(null, {}), false)
  assert.equal(sameTableQueryParams(new Date(0), new Date(0)), false)

  const symbolKey = Symbol('query')
  assert.equal(sameTableQueryParams({ [symbolKey]: 1 }, { [symbolKey]: 2 }), false)
  const hiddenOne = {}
  Object.defineProperty(hiddenOne, 'cursor', { value: 1 })
  const hiddenTwo = {}
  Object.defineProperty(hiddenTwo, 'cursor', { value: 2 })
  assert.equal(sameTableQueryParams(hiddenOne, hiddenTwo), false)

  const previous = { status: '', page: 1 }
  assert.equal(stabilizeTableQueryParams(previous, { status: '', page: 1 }), previous)
  const changed = stabilizeTableQueryParams(previous, { status: 'active', page: 1 })
  assert.notEqual(changed, previous)
  assert.deepEqual(changed, { status: 'active', page: 1 })
})

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

test('树结果用 list 存节点，total 取节点数', () => {
  const current = createInitialTableQueryState()
  const tree = [{ id: 1, children: [{ id: 2 }] }]
  assert.deepEqual(
    applyTableQueryResult(current, 1, 1, { list: tree, total: tree.length }),
    { list: tree, total: 1, loading: false, error: false },
  )
})
