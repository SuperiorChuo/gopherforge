import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const composeURL = new URL('../docker-compose.yml', import.meta.url)
const compose = readFileSync(composeURL, 'utf8')
const envExample = readFileSync(new URL('../.env.example', import.meta.url), 'utf8')
const systemDockerfile = readFileSync(new URL('../services/system/Dockerfile', import.meta.url), 'utf8')

test('Compose 由 system 唯一接管 HTTP-01 并通过 file provider 动态加载证书', () => {
  assert.match(compose, /--entrypoints\.web\.allowACMEByPass=true/)
  assert.match(compose, /--providers\.file\.directory=\/var\/lib\/go-admin-kit\/edgecert\/traefik-dynamic/)
  assert.match(compose, /--providers\.file\.watch=true/)
  assert.doesNotMatch(compose, /--certificatesresolvers\.letsencrypt\.acme\./)
  assert.match(compose, /EDGE_CERT_HOST_DIR[^\n]*:\/var\/lib\/go-admin-kit\/edgecert:ro/)
})

test('system 与 gateway 共享 Compose-only 持久目录且不恢复旧双 owner overlay', () => {
  assert.match(compose, /system-service:[\s\S]*?EDGE_CERT_CURRENT_KEY_ID:[\s\S]*?EDGE_CERT_CLEAR_LEGACY_SECRETS:/)
  assert.match(compose, /system-service:[\s\S]*?EDGE_CERT_HOST_DIR[^\n]*:\/var\/lib\/go-admin-kit\/edgecert\n/)
  assert.match(systemDockerfile, /addgroup -S -g 10001 app/)
  assert.match(systemDockerfile, /adduser -S -D -H -u 10001 -G app app/)
  assert.match(systemDockerfile, /^USER app$/m)
  assert.doesNotMatch(envExample, /EDGE_CERT_.*SECRET_NAME|docker secret|Swarm/)
  assert.equal(existsSync(new URL('../docker-compose.gateway-edge-tls.example.yml', import.meta.url)), false)
  assert.equal(existsSync(new URL('../certs/traefik-dynamic-edge-tls.example.yml', import.meta.url)), false)
  assert.equal(existsSync(new URL('../docker-stack.edge-cert-rotation.yml', import.meta.url)), false)
})
