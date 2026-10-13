import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const microservicesRoot = new URL('../', import.meta.url)

function configuredServices() {
  const output = execFileSync(
    'docker',
    ['compose', '--env-file', '.env.example', 'config', '--services'],
    { cwd: microservicesRoot, encoding: 'utf8' },
  )
  return new Set(output.trim().split(/\s+/).filter(Boolean))
}

test('示例环境默认启用完整微服务应用栈', () => {
  const services = configuredServices()

  for (const service of [
    'migrate',
    'gateway',
    'frontend',
    'auth-service',
    'identity-service',
    'system-service',
    'audit-service',
    'file-service',
    'ai-service',
    'im-service',
    'cc-service',
    'crm-service',
    'visibility-service',
    'ticket-service',
    'pay-service',
    'bpm-service',
    'notify-service',
    'mp-service',
  ]) {
    assert.ok(services.has(service), `Compose 未启用服务: ${service}`)
  }
})

test('静态前端入口把 API、上传与 IM WebSocket 请求代理到共享 gateway', () => {
  const nginx = readFileSync(new URL('../web/nginx.conf', import.meta.url), 'utf8')

  assert.match(nginx, /resolver\s+127\.0\.0\.11\s+valid=\d+s\s+ipv6=off\s*;/)
  assert.match(nginx, /set\s+\$gateway_upstream\s+http:\/\/go-admin-kit-gateway:80\s*;/)

  for (const path of ['/api/', '/uploads/']) {
    const escapedPath = path.replaceAll('/', '\\/')
    assert.match(
      nginx,
      new RegExp(`location\\s+${escapedPath}\\s*\\{[\\s\\S]*?proxy_pass\\s+\\$gateway_upstream\\s*;`),
      `Nginx 缺少 ${path} 到 gateway 的代理`,
    )
  }

  assert.match(
    nginx,
    /location\s*=\s*\/im\/ws\s*\{[\s\S]*?proxy_pass\s+\$gateway_upstream\s*;/,
  )
  assert.doesNotMatch(nginx, /location\s+\/im\/\s*\{/, '不能把 IM 管理页整体代理到后端')
  assert.match(nginx, /proxy_http_version\s+1\.1\s*;/)
  assert.match(nginx, /proxy_set_header\s+Host\s+\$http_host\s*;/)
  assert.match(nginx, /proxy_set_header\s+X-Real-IP\s+\$remote_addr\s*;/)
  assert.match(nginx, /proxy_set_header\s+X-Forwarded-For\s+\$proxy_add_x_forwarded_for\s*;/)
  assert.match(nginx, /proxy_set_header\s+X-Forwarded-Proto\s+\$scheme\s*;/)
  assert.match(nginx, /proxy_set_header\s+Upgrade\s+\$http_upgrade\s*;/)
  assert.match(nginx, /proxy_set_header\s+Connection\s+\$connection_upgrade\s*;/)
})

test('根目录快速开始包含共享网络和独立 infra 栈前置步骤', () => {
  const readme = readFileSync(new URL('../../README.md', import.meta.url), 'utf8')
  const start = readme.indexOf('### 🧩 微服务版')
  const end = readme.indexOf('| 入口', start)
  const quickStart = readme.slice(start, end)

  assert.match(quickStart, /docker network inspect go-admin-kit-net/)
  assert.match(quickStart, /docker compose -p go-admin-kit-infra -f docker-compose\.infra\.yml up -d/)
  assert.match(quickStart, /docker compose --profile compose-fallback up -d --build/)
})

test('Makefile 的完整启动入口包含共享网络和独立 infra 栈前置步骤', () => {
  const makefile = readFileSync(new URL('../Makefile', import.meta.url), 'utf8')

  assert.match(makefile, /docker network inspect go-admin-kit-net/)
  assert.match(makefile, /docker compose -p go-admin-kit-infra -f docker-compose\.infra\.yml up -d/)
  assert.match(makefile, /docker compose --profile compose-fallback up -d --build/)
})

test('远端割接脚本显式启用 compose-fallback profile', () => {
  const deployScripts = [
    '../../scripts/ops/infra-split-cutover.sh',
    '../../scripts/ops/overlay-net-cutover.sh',
  ].map((path) => readFileSync(new URL(path, import.meta.url), 'utf8'))

  for (const script of deployScripts) {
    assert.match(script, /APP_PROFILES=\(\x27?--profile compose-fallback/)
  }
})

test('独立 infra 栈入口先创建 external 共享网络', () => {
  const entrypoints = [
    '../../.github/workflows/ci.yml',
    '../../LOCAL_SETUP.md',
    '../../docs/deployment.md',
    '../../microservices/services/monitor/README.md',
  ]

  for (const path of entrypoints) {
    const content = readFileSync(new URL(path, import.meta.url), 'utf8')
    assert.match(
      content,
      /docker network inspect go-admin-kit-net[\s\S]*docker compose -p go-admin-kit-infra -f docker-compose\.infra\.yml up -d/,
      `${path} 缺少共享网络前置步骤`,
    )
  }
})

test('Swarm 主入口先保证 infra 与 gateway，再通过 stack 更新业务服务', () => {
  const script = readFileSync(new URL('../../scripts/swarm-redeploy.sh', import.meta.url), 'utf8')

  assert.match(script, /docker network create[\s\S]*--driver overlay[\s\S]*--attachable/)
  assert.match(
    script,
    /docker compose[\s\S]*docker-compose\.infra\.yml[\s\S]*--profile storage[\s\S]*up -d/,
  )
  assert.match(
    script,
    /docker compose[\s\S]*--profile compose-fallback[\s\S]*up -d[\s\S]*gateway/,
  )
  assert.match(script, /docker stack deploy/)
  assert.match(script, /docker service update --force/)
  assert.doesNotMatch(
    script,
    /docker compose --profile compose-fallback up -d --build "\$\{SVCS\[@\]\}"/,
  )
})

test('本机远端部署入口把业务服务交给 Swarm、把 gateway 留在 Compose', () => {
  const script = readFileSync(new URL('../../scripts/deploy.sh', import.meta.url), 'utf8')

  assert.match(script, /scripts\/swarm-redeploy\.sh/)
  assert.match(script, /COMPOSE_SVCS=" gateway prometheus grafana "/)
  assert.match(script, /docker compose --profile compose-fallback up -d --build/)
})

test('边缘证书由 system 唯一接管 HTTP-01 并通过 file provider 动态加载', () => {
  const compose = readFileSync(new URL('../docker-compose.yml', import.meta.url), 'utf8')
  const stack = readFileSync(new URL('../docker-stack.yml', import.meta.url), 'utf8')

  assert.match(compose, /--entrypoints\.web\.allowACMEByPass=true/)
  assert.match(compose, /--providers\.file\.directory=\/var\/lib\/go-admin-kit\/edgecert\/traefik-dynamic/)
  assert.match(compose, /--providers\.file\.watch=true/)
  assert.doesNotMatch(compose, /--certificatesresolvers\.letsencrypt\.acme\./)
  assert.match(compose, /EDGE_CERT_HOST_DIR[^\n]*:\/var\/lib\/go-admin-kit\/edgecert:ro/)
  assert.match(stack, /source:\s*edge_cert_encryption_key[\s\S]*target:\s*edge_cert_encryption_key/)
  assert.match(stack, /edge_cert_encryption_key:[\s\S]*external:\s*true/)
  assert.match(stack, /system-service:[\s\S]*?EDGE_CERT_CLEAR_LEGACY_SECRETS:[\s\S]*?update_config:[\s\S]*?order:\s*stop-first/)
})

test('Swarm 重部署在滚动 system 前强制检查密钥和持久目录', () => {
  const script = readFileSync(new URL('../../scripts/swarm-redeploy.sh', import.meta.url), 'utf8')
  const rotation = readFileSync(new URL('../docker-stack.edge-cert-rotation.yml', import.meta.url), 'utf8')

  assert.match(script, /ensure_edge_cert_storage/)
  assert.match(script, /load_edge_cert_selectors/)
  assert.match(script, /EDGE_CERT_SELECTOR_KEYS=/)
  assert.match(script, /while IFS='=' read -r key value/)
  assert.doesNotMatch(script, /(?:^|\n)\s*(?:source|\.)\s+(?:"?\.env"?|<\()/)
  assert.match(script, /install -d -m 0700 -o 10001 -g 10001/)
  assert.match(script, /docker secret inspect "\$EDGE_CERT_ENCRYPTION_SECRET_NAME"/)
  assert.match(script, /EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME/)
  assert.match(script, /docker-stack\.edge-cert-rotation\.yml/)
  assert.match(rotation, /target:\s*edge_cert_previous_encryption_key/)
})

test('visibility 重部署复用 Swarm 主入口，不启动 Compose 业务副本', () => {
  const script = readFileSync(new URL('../../scripts/redeploy-visibility.sh', import.meta.url), 'utf8')

  assert.match(script, /scripts\/swarm-redeploy\.sh/)
  assert.doesNotMatch(script, /docker compose[\s\S]*up -d --build \$SERVICES/)
})

test('回滚入口按已存在的 Swarm 服务优先回滚，否则保留 Compose fallback', () => {
  const script = readFileSync(new URL('../../scripts/ops/rollback.sh', import.meta.url), 'utf8')

  assert.match(script, /docker service inspect/)
  assert.match(script, /docker service update --image/)
  assert.match(script, /docker compose --profile compose-fallback/)
})
