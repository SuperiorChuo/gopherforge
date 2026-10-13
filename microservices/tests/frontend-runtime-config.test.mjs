import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('../web/docker-entrypoint.d/40-runtime-config.sh', import.meta.url))

async function runConfig(t, value) {
  const directory = await mkdtemp(join(tmpdir(), 'gak-runtime-config-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const output = join(directory, 'runtime-config.js')
  const result = spawnSync('sh', [script], {
    encoding: 'utf8',
    env: {
      ...process.env,
      GAK_RUNTIME_CONFIG_PATH: output,
      CC_SIP_WSS_URL: value,
    },
  })
  return { result, output }
}

test('empty runtime WSS config keeps the browser fallback', async (t) => {
  const { result, output } = await runConfig(t, '')
  assert.equal(result.status, 0, result.stderr)
  assert.equal(await readFile(output, 'utf8'), 'window.__GAK_RUNTIME_CONFIG__ = Object.freeze({});\n')
})

test('valid runtime WSS config is emitted as immutable JavaScript data', async (t) => {
  const { result, output } = await runConfig(t, 'wss://pbx.example.com:7443/sip')
  assert.equal(result.status, 0, result.stderr)
  assert.equal(
    await readFile(output, 'utf8'),
    'window.__GAK_RUNTIME_CONFIG__ = Object.freeze({"ccSipWsUrl":"wss://pbx.example.com:7443/sip"});\n',
  )
})

test('insecure runtime WebSocket config fails closed', async (t) => {
  const { result } = await runConfig(t, 'ws://pbx.example.com:5066')
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /必须使用 wss:\/\//)
})

test('runtime config rejects JavaScript injection characters', async (t) => {
  const { result } = await runConfig(t, 'wss://pbx.example.com/";alert(1)//')
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /不支持的字符/)
})
