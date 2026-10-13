#!/usr/bin/env node

import { createHash, randomBytes } from 'node:crypto'
import { isIP } from 'node:net'
import { resolve } from 'node:path'
import { connect } from 'node:tls'
import { fileURLToPath } from 'node:url'

const websocketGUID = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'

export function parseEndpoint(raw) {
  if (!raw) throw new Error('请传入 wss:// 端点，或设置 CC_SIP_WSS_URL')

  let endpoint
  try {
    endpoint = new URL(raw)
  } catch {
    throw new Error('WSS 端点格式无效')
  }
  if (endpoint.protocol !== 'wss:') throw new Error('生产端点必须使用 wss://')
  if (endpoint.username || endpoint.password) throw new Error('WSS 端点不得包含账号或密码')
  if (endpoint.search || endpoint.hash) throw new Error('WSS 端点不得包含查询参数或片段')

  return endpoint
}

export function websocketAccept(key) {
  return createHash('sha1').update(key + websocketGUID).digest('base64')
}

function headerMap(lines) {
  const headers = new Map()
  for (const line of lines) {
    const separator = line.indexOf(':')
    if (separator < 1) continue
    headers.set(line.slice(0, separator).trim().toLowerCase(), line.slice(separator + 1).trim())
  }
  return headers
}

export async function checkWss(raw, timeoutMs = 8000) {
  const endpoint = parseEndpoint(raw)
  const hostname = endpoint.hostname.replace(/^\[|\]$/g, '')
  const port = Number(endpoint.port || 443)
  const key = randomBytes(16).toString('base64')

  return await new Promise((resolveCheck, rejectCheck) => {
    let response = ''
    let settled = false
    let certificate

    const finish = (error, result) => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      socket.destroy()
      if (error) rejectCheck(error)
      else resolveCheck(result)
    }

    const socket = connect({
      host: hostname,
      port,
      servername: isIP(hostname) ? undefined : hostname,
      rejectUnauthorized: true,
    })
    const timer = setTimeout(() => finish(new Error(`连接超时（${timeoutMs}ms）`)), timeoutMs)

    socket.once('secureConnect', () => {
      certificate = socket.getPeerCertificate()
      const hostHeader = endpoint.port ? endpoint.host : endpoint.hostname
      const path = endpoint.pathname || '/'
      socket.write([
        `GET ${path} HTTP/1.1`,
        `Host: ${hostHeader}`,
        'Connection: Upgrade',
        'Upgrade: websocket',
        `Sec-WebSocket-Key: ${key}`,
        'Sec-WebSocket-Version: 13',
        'Sec-WebSocket-Protocol: sip',
        '',
        '',
      ].join('\r\n'))
    })

    socket.on('data', (chunk) => {
      response += chunk.toString('latin1')
      const headerEnd = response.indexOf('\r\n\r\n')
      if (headerEnd < 0) return

      const [statusLine, ...headerLines] = response.slice(0, headerEnd).split('\r\n')
      const headers = headerMap(headerLines)
      if (!/^HTTP\/1\.[01] 101(?: |$)/.test(statusLine)) {
        finish(new Error(`WebSocket 升级失败：${statusLine || '无 HTTP 状态行'}`))
        return
      }
      if (headers.get('upgrade')?.toLowerCase() !== 'websocket') {
        finish(new Error('WebSocket 升级响应缺少 Upgrade: websocket'))
        return
      }
      if (headers.get('sec-websocket-accept') !== websocketAccept(key)) {
        finish(new Error('WebSocket 握手校验值不匹配'))
        return
      }
      if (headers.get('sec-websocket-protocol')?.toLowerCase() !== 'sip') {
        finish(new Error('端点未协商 SIP WebSocket 子协议'))
        return
      }

      finish(undefined, {
        endpoint: endpoint.toString(),
        tlsProtocol: socket.getProtocol(),
        cipher: socket.getCipher()?.name,
        subject: certificate?.subject?.CN,
        issuer: certificate?.issuer?.CN,
        validTo: certificate?.valid_to,
      })
    })
    socket.once('error', (error) => finish(error))
    socket.once('end', () => finish(new Error('服务端在 WebSocket 握手完成前关闭连接')))
  })
}

async function main() {
  const raw = process.argv[2] || process.env.CC_SIP_WSS_URL
  try {
    const result = await checkWss(raw)
    console.log(`[cc-wss] endpoint=${result.endpoint}`)
    console.log(`[cc-wss] tls=${result.tlsProtocol} cipher=${result.cipher}`)
    console.log(`[cc-wss] certificate subject=${result.subject} issuer=${result.issuer} valid_to=${result.validTo}`)
    console.log('[cc-wss] websocket=101 protocol=sip')
  } catch (error) {
    console.error(`[cc-wss] FAILED: ${error instanceof Error ? error.message : String(error)}`)
    process.exitCode = 1
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main()
}
