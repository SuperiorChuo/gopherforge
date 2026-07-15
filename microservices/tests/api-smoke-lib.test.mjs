import assert from 'node:assert/strict';
import { deflateSync } from 'node:zlib';
import test from 'node:test';

import {
  buildConfig,
  decodeTextCaptchaCode,
  getJsonPath,
  jsonObject,
  normalizeRunId,
  statusMatches,
} from './api-smoke-lib.mjs';

test('statusMatches supports wildcard and comma-separated HTTP codes', () => {
  assert.equal(statusMatches('*', 503), true);
  assert.equal(statusMatches('200,201', 200), true);
  assert.equal(statusMatches('200,201', 404), false);
});

test('normalizeRunId keeps only portable request id characters', () => {
  assert.equal(normalizeRunId('2026-05-20 smoke#1'), '2026_05_20_smoke_1');
});

test('jsonObject builds JSON with string values', () => {
  assert.equal(jsonObject({ nickname: '管理后台', role: 'admin' }), '{"nickname":"管理后台","role":"admin"}');
});

test('getJsonPath reads nested values and rejects missing paths', () => {
  const data = { code: 200, data: { user: { username: 'admin' } } };

  assert.equal(getJsonPath(data, 'data.user.username'), 'admin');
  assert.throws(() => getJsonPath(data, 'data.user.email'), /missing JSON path/);
});

test('buildConfig reads environment overrides', () => {
  const config = buildConfig({
    API_BASE_URL: 'http://127.0.0.1:8081/api/v1/',
    SMOKE_USERNAME: 'tester',
    SMOKE_PASSWORD: 'secret',
    SMOKE_TIMEOUT: '5',
    SMOKE_RUN_ID: 'run-id',
  });

  assert.deepEqual(config, {
    apiBaseUrl: 'http://127.0.0.1:8081/api/v1',
    username: 'tester',
    password: 'secret',
    timeoutSeconds: 5,
    runId: 'run-id',
    safeRunId: 'run_id',
  });
});

test('decodeTextCaptchaCode reads deterministic block glyph PNGs', () => {
  assert.equal(decodeTextCaptchaCode(createCaptchaFixture('A7K9')), 'A7K9');
});

test('decodeTextCaptchaCode tolerates captcha noise over glyph pixels', () => {
  assert.equal(decodeTextCaptchaCode(createCaptchaFixture('NNNN', { drawNoise: true })), 'NNNN');
});

function createCaptchaFixture(code, options = {}) {
  const width = 120;
  const height = 42;
  const rgba = Buffer.alloc(width * height * 4);
  for (let offset = 0; offset < rgba.length; offset += 4) {
    rgba[offset] = 245;
    rgba[offset + 1] = 248;
    rgba[offset + 2] = 255;
    rgba[offset + 3] = 255;
  }

  let left = 12;
  for (const ch of code) {
    drawFixtureGlyph(rgba, width, left, 8, ch);
    left += 26;
  }
  if (options.drawNoise) {
    for (let x = 0; x < width; x += 9) {
      const y = (x * 7) % height;
      const offset = (y * width + x) * 4;
      rgba[offset] = 120;
      rgba[offset + 1] = 150;
      rgba[offset + 2] = 210;
      rgba[offset + 3] = 120;
    }
  }

  const scanlines = Buffer.alloc(height * (1 + width * 4));
  for (let y = 0; y < height; y += 1) {
    const srcStart = y * width * 4;
    const dstStart = y * (1 + width * 4);
    scanlines[dstStart] = 0;
    rgba.copy(scanlines, dstStart + 1, srcStart, srcStart + width * 4);
  }

  const chunks = [
    pngChunk('IHDR', Buffer.concat([
      uint32(width),
      uint32(height),
      Buffer.from([8, 6, 0, 0, 0]),
    ])),
    pngChunk('IDAT', deflateSync(scanlines)),
    pngChunk('IEND', Buffer.alloc(0)),
  ];
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), ...chunks]).toString('base64');
}

function drawFixtureGlyph(rgba, width, left, top, ch) {
  const pattern = {
    A: [0x0e, 0x11, 0x11, 0x1f, 0x11, 0x11, 0x11],
    K: [0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11],
    N: [0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11],
    7: [0x1f, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08],
    9: [0x0e, 0x11, 0x11, 0x0f, 0x01, 0x01, 0x1e],
  }[ch];

  for (let row = 0; row < pattern.length; row += 1) {
    for (let col = 0; col < 5; col += 1) {
      if ((pattern[row] & (1 << (4 - col))) === 0) continue;
      for (let dy = 0; dy < 3; dy += 1) {
        for (let dx = 0; dx < 3; dx += 1) {
          const x = left + col * 4 + dx;
          const y = top + row * 4 + dy;
          const offset = (y * width + x) * 4;
          rgba[offset] = 30;
          rgba[offset + 1] = 70;
          rgba[offset + 2] = 160;
          rgba[offset + 3] = 255;
        }
      }
    }
  }
}

function pngChunk(type, data) {
  return Buffer.concat([uint32(data.length), Buffer.from(type), data, uint32(0)]);
}

function uint32(value) {
  const buffer = Buffer.alloc(4);
  buffer.writeUInt32BE(value);
  return buffer;
}
