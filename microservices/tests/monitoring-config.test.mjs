import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8')
const compose = read('../docker-compose.yml')
const prometheus = read('../../platform/deploy/prometheus/prometheus.yml')
const swarm = read('../../platform/deploy/prometheus/prometheus.swarm.yml')
const dashboards = '../../platform/deploy/grafana/provisioning/dashboards/'
const services = {
  auth: 8082, identity: 8083, system: 8084, audit: 8085, file: 8086,
  ai: 8087, im: 8088, cc: 8089, crm: 8091, mp: 8092, visibility: 8093,
  ticket: 8094, notify: 8095, bpm: 8096, pay: 8097,
}

test('default monitoring targets actual Compose names with stable service labels', () => {
  assert.doesNotMatch(prometheus, /tasks\./)
  for (const [service, port] of Object.entries(services)) {
    const name = `go-admin-kit-${service}`
    const config = compose.split(`  ${service}-service:`)[1]?.split(/\n  [\w-]+:/)[0]
    assert.ok(config?.includes(`container_name: ${name}`), `${service} container name`)
    assert.match(prometheus, new RegExp(`targets: \\["${name}:${port}"\\]\\s+labels: \\{ service: ${service} \\}`))
  }
  assert.match(compose, /\$\{PROMETHEUS_CONFIG_FILE:-\.\.\/platform\/deploy\/prometheus\/prometheus\.yml\}/)
})

test('Swarm keeps per-replica discovery in an explicitly selectable configuration', () => {
  assert.match(swarm, /dns_sd_configs:/)
  for (const [service, port] of Object.entries(services)) {
    assert.ok(swarm.includes(`{ names: ["tasks.gak_${service}-service"], type: A, port: ${port} }`), service)
  }
  assert.match(swarm, /target_label: service/)
  assert.ok(swarm.includes('regex: tasks\\.gak_(.+)-service'))
})

test('public dashboard UID and all application metric panels remain usable', () => {
  const dashboard = JSON.parse(read(`${dashboards}services-overview.json`))
  const metrics = read('../services/shared/pkg/metrics/metrics.go')
  assert.equal(dashboard.uid, 'gak-services')
  const expressions = dashboard.panels.flatMap(panel => panel.targets.map(target => target.expr))
  for (const metric of [
    'go_admin_kit_http_requests_total', 'go_admin_kit_http_request_errors_total',
    'go_admin_kit_http_request_duration_seconds_bucket', 'go_admin_kit_http_in_flight_requests',
    'go_admin_kit_go_goroutines', 'go_admin_kit_go_memory_alloc_bytes',
    'go_admin_kit_db_in_use_connections', 'go_admin_kit_db_open_connections',
  ]) {
    assert.ok(expressions.some(expr => expr.includes(metric)), `missing panel: ${metric}`)
    assert.ok(metrics.includes(metric), `not exported: ${metric}`)
  }
  assert.ok(expressions.includes('up{job="go-admin-kit-services"}'))
  const errors = dashboard.panels.find(panel => panel.id === 3)
  assert.equal(errors.fieldConfig.defaults.unit, 'percent')
  assert.match(errors.targets[0].expr, /^100 \* .* \/ .*http_requests_total/)
  assert.ok(expressions.some(expr => expr.includes('histogram_quantile(0.95')))
})

test('platform dashboard retains infrastructure panels and correct runtime metric names', () => {
  const dashboard = JSON.parse(read(`${dashboards}platform-overview.json`))
  const expressions = dashboard.panels.flatMap(panel => panel.targets.map(target => target.expr)).join('\n')
  assert.notEqual(dashboard.uid, 'gak-services')
  for (const prefix of ['pg_', 'redis_', 'node_', 'ALERTS']) assert.ok(expressions.includes(prefix), prefix)
  assert.match(expressions, /go_admin_kit_go_goroutines/)
  assert.match(expressions, /go_admin_kit_go_memory_alloc_bytes/)
  assert.doesNotMatch(expressions, /\bgo_goroutines\b|\bgo_memstats_/)
})
