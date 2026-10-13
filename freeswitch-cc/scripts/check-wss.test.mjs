import assert from 'node:assert/strict'
import test from 'node:test'

import { parseEndpoint, websocketAccept } from './check-wss.mjs'

test('parseEndpoint accepts a production WSS URL', () => {
  assert.equal(parseEndpoint('wss://pbx.example.com:7443/sip').toString(), 'wss://pbx.example.com:7443/sip')
})

test('parseEndpoint rejects insecure or credential-bearing URLs', () => {
  assert.throws(() => parseEndpoint('ws://pbx.example.com:5066'), /wss:\/\//)
  assert.throws(() => parseEndpoint('wss://user:pass@pbx.example.com:7443'), /账号或密码/)
  assert.throws(() => parseEndpoint('wss://pbx.example.com:7443/?token=secret'), /查询参数/)
})

test('websocketAccept matches RFC 6455', () => {
  assert.equal(websocketAccept('dGhlIHNhbXBsZSBub25jZQ=='), 's3pPLMBiTxaQ9kYGzzhZRbK+xOo=')
})
