import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { chromium } from '../../microservices/web/node_modules/playwright/index.mjs'

const host = '127.0.0.1'
const port = 4177
const origin = `http://${host}:${port}`
const base = `${origin}/gopherforge/docs`
const server = spawn(process.execPath, ['node_modules/vitepress/bin/vitepress.js', 'preview', '--host', host, '--port', String(port)], {
  cwd: new URL('..', import.meta.url),
  detached: process.platform !== 'win32',
  stdio: 'ignore',
})

const stopServer = () => {
  if (!server.pid) return
  try {
    if (process.platform === 'win32') server.kill('SIGTERM')
    else process.kill(-server.pid, 'SIGTERM')
  } catch {
    // The preview process may already have exited.
  }
}

const waitForServer = async () => {
  for (let attempt = 0; attempt < 60; attempt += 1) {
    try {
      const response = await fetch(`${base}/`)
      if (response.ok) return
    } catch {
      // Preview is still starting.
    }
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error('VitePress preview did not become ready')
}

const cases = [
  { name: 'zh-home-dark-desktop', path: '/', width: 1280, height: 900, scheme: 'dark', home: true },
  { name: 'zh-home-light-mobile', path: '/', width: 390, height: 844, scheme: 'light', home: true },
  { name: 'en-home-light-desktop', path: '/en/', width: 1280, height: 900, scheme: 'light', home: true },
  { name: 'en-home-dark-mobile', path: '/en/', width: 390, height: 844, scheme: 'dark', home: true },
  { name: 'zh-doc-light-desktop', path: '/guide/getting-started.html', width: 1280, height: 900, scheme: 'light', home: false },
  { name: 'zh-doc-dark-wide', path: '/guide/getting-started.html', width: 1440, height: 900, scheme: 'dark', home: false },
  { name: 'en-doc-dark-mobile', path: '/en/guide/getting-started.html', width: 390, height: 844, scheme: 'dark', home: false },
]

let browser
try {
  await waitForServer()
  browser = await chromium.launch({ headless: true })
  const results = []
  for (const entry of cases) {
    const context = await browser.newContext({
      viewport: { width: entry.width, height: entry.height },
      colorScheme: entry.scheme,
      reducedMotion: 'reduce',
    })
    const page = await context.newPage()
    const issues = []
    page.on('console', message => {
      if (['error', 'warning'].includes(message.type())) issues.push(`${message.type()}: ${message.text()}`)
    })
    page.on('pageerror', error => issues.push(`pageerror: ${error.message}`))
    const response = await page.goto(`${base}${entry.path}`, { waitUntil: 'networkidle', timeout: 60_000 })
    assert.equal(response?.status(), 200, `${entry.name}: HTTP status`)
    if (entry.home) {
      await page.locator('.showcase-frame').scrollIntoViewIfNeeded()
      await page.locator('.showcase-frame img').evaluate(image => image.complete
        ? undefined
        : new Promise(resolve => image.addEventListener('load', resolve, { once: true })))
    }
    const metrics = await page.evaluate(home => {
      const missingAlt = [...document.images].filter(image => !image.hasAttribute('alt')).length
      const iframeWithoutTitle = [...document.querySelectorAll('iframe')].filter(frame => !frame.title).length
      return {
        clientWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
        h1: document.querySelectorAll('h1').length,
        missingAlt,
        iframeWithoutTitle,
        release: home ? Boolean(document.querySelector('.home-release-bar')) : null,
        commands: home ? [...document.querySelectorAll('.terminal-command code')].map(node => node.textContent?.trim()) : [],
        screenshotWidth: home ? document.querySelector('.showcase-frame img')?.naturalWidth : null,
        floats: home ? getComputedStyle(document.querySelector('.hero-float-card')).display : null,
        articleFilter: home ? null : getComputedStyle(document.querySelector('.VPDoc .content-container')).backdropFilter,
        articleWidth: home ? null : Math.round(document.querySelector('.VPDoc .content-container').getBoundingClientRect().width),
      }
    }, entry.home)
    assert.equal(metrics.scrollWidth, metrics.clientWidth, `${entry.name}: horizontal overflow`)
    assert.equal(metrics.h1, 1, `${entry.name}: exactly one h1`)
    assert.equal(metrics.missingAlt, 0, `${entry.name}: image alt`)
    assert.equal(metrics.iframeWithoutTitle, 0, `${entry.name}: iframe title`)
    assert.deepEqual(issues, [], `${entry.name}: console/page errors`)
    if (entry.home) {
      assert.equal(metrics.release, true, `${entry.name}: release bar`)
      assert.deepEqual(metrics.commands, ['make compose-up', 'make smoke-api', 'make test'])
      assert.equal(metrics.screenshotWidth, 1432, `${entry.name}: showcase loaded`)
      if (entry.width === 390) assert.equal(metrics.floats, 'none', `${entry.name}: simplified hero`)
    } else {
      assert.ok(metrics.articleFilter === 'none' || metrics.articleFilter.includes('blur(14px)'), `${entry.name}: article filter`)
      if (entry.width === 1440) assert.equal(metrics.articleWidth, 760, `${entry.name}: wide article`)
    }
    results.push({ name: entry.name, ...metrics })
    await context.close()
  }
  console.log(`browser smoke: ${results.length}/${cases.length} passed`)
  for (const result of results) console.log(`- ${result.name}: ${result.clientWidth}px, no overflow`)
} finally {
  await browser?.close()
  stopServer()
}
