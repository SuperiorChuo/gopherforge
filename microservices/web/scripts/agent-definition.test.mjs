import assert from 'node:assert/strict'
import test from 'node:test'
import { canRunDefinition, definitionPayload, definitionTextLength } from '../src/pages/ai/agents/definition-form.ts'

// 白名单负控：即便表单来源夹带租户/标识/版本，也不能随 PATCH 传回。
test('定义编辑只传白名单字段，允许清空描述', () => {
  assert.deepEqual(definitionPayload({ name: '  新名称  ', description: ' ', status: 'disabled', tenant_id: 999, id: 18, agent_key: 'changed', latest_version_id: 91 }),
    { name: '新名称', description: '', status: 'disabled' })
  assert.deepEqual(definitionPayload({ name: '原名称', status: 'active' }), { name: '原名称', description: '', status: 'active' })
})

test('字符边界与 Go rune 计数一致，超限和未知状态拒绝', () => {
  assert.equal(definitionTextLength('  😀  '), 1)
  assert.equal(definitionPayload({ name: '😀'.repeat(100), description: '𠮷'.repeat(500), status: 'active' }).name, '😀'.repeat(100))
  for (const override of [{ name: ' ' }, { name: '😀'.repeat(101) }, { description: '𠮷'.repeat(501) }, { status: 'running' }, { status: null }, { name: 5 }, { description: {} }]) {
    assert.throws(() => definitionPayload({ name: '合法', description: '', status: 'active', ...override }), /invalid-agent-definition/)
  }
})

test('只有有权限的启用已发布定义可发起运行，禁用不等于取消已有Run', () => {
  for (const permission of [true, false]) for (const status of ['active', 'disabled', 'unknown']) for (const version of [undefined, 0, -1, 7, NaN, Infinity]) {
    assert.equal(canRunDefinition({ status, latest_version_id: version }, permission), permission && status === 'active' && version === 7)
  }
})
