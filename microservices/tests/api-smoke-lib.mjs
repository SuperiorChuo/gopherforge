import { inflateSync } from 'node:zlib';

const CAPTCHA_WIDTH = 120;
const CAPTCHA_HEIGHT = 42;
const CAPTCHA_LEFT = 12;
const CAPTCHA_TOP = 8;
const CAPTCHA_STEP = 26;
const CAPTCHA_LENGTH = 4;

const textCaptchaGlyphs = new Map([
  ['2', [0x1e, 0x01, 0x01, 0x1e, 0x10, 0x10, 0x1f]],
  ['3', [0x1e, 0x01, 0x01, 0x0e, 0x01, 0x01, 0x1e]],
  ['4', [0x12, 0x12, 0x12, 0x1f, 0x02, 0x02, 0x02]],
  ['5', [0x1f, 0x10, 0x10, 0x1e, 0x01, 0x01, 0x1e]],
  ['6', [0x0f, 0x10, 0x10, 0x1e, 0x11, 0x11, 0x0e]],
  ['7', [0x1f, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08]],
  ['8', [0x0e, 0x11, 0x11, 0x0e, 0x11, 0x11, 0x0e]],
  ['9', [0x0e, 0x11, 0x11, 0x0f, 0x01, 0x01, 0x1e]],
  ['A', [0x0e, 0x11, 0x11, 0x1f, 0x11, 0x11, 0x11]],
  ['B', [0x1e, 0x11, 0x11, 0x1e, 0x11, 0x11, 0x1e]],
  ['C', [0x0f, 0x10, 0x10, 0x10, 0x10, 0x10, 0x0f]],
  ['D', [0x1e, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1e]],
  ['E', [0x1f, 0x10, 0x10, 0x1e, 0x10, 0x10, 0x1f]],
  ['F', [0x1f, 0x10, 0x10, 0x1e, 0x10, 0x10, 0x10]],
  ['G', [0x0f, 0x10, 0x10, 0x13, 0x11, 0x11, 0x0f]],
  ['H', [0x11, 0x11, 0x11, 0x1f, 0x11, 0x11, 0x11]],
  ['J', [0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0e]],
  ['K', [0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11]],
  ['L', [0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1f]],
  ['M', [0x11, 0x1b, 0x15, 0x15, 0x11, 0x11, 0x11]],
  ['N', [0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11]],
  ['P', [0x1e, 0x11, 0x11, 0x1e, 0x10, 0x10, 0x10]],
  ['Q', [0x0e, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0d]],
  ['R', [0x1e, 0x11, 0x11, 0x1e, 0x14, 0x12, 0x11]],
  ['S', [0x0f, 0x10, 0x10, 0x0e, 0x01, 0x01, 0x1e]],
  ['T', [0x1f, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04]],
  ['U', [0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0e]],
  ['V', [0x11, 0x11, 0x11, 0x11, 0x0a, 0x0a, 0x04]],
  ['W', [0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0a]],
  ['X', [0x11, 0x11, 0x0a, 0x04, 0x0a, 0x11, 0x11]],
  ['Y', [0x11, 0x11, 0x0a, 0x04, 0x04, 0x04, 0x04]],
  ['Z', [0x1f, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1f]],
]);

export function statusMatches(expected, actual) {
  if (expected === '*') return true;

  const actualCode = String(actual);
  return String(expected)
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .includes(actualCode);
}

export function normalizeRunId(value) {
  return String(value).replace(/[^A-Za-z0-9_]/g, '_').replace(/_+/g, '_').replace(/^_+|_+$/g, '');
}

export function jsonObject(values) {
  return JSON.stringify(values);
}

export function getJsonPath(data, path) {
  let cursor = data;

  for (const part of path.split('.')) {
    if (cursor == null || !Object.prototype.hasOwnProperty.call(Object(cursor), part)) {
      throw new Error(`missing JSON path: ${path}`);
    }
    cursor = cursor[part];
  }

  if (cursor == null) {
    throw new Error(`empty JSON path: ${path}`);
  }

  return typeof cursor === 'object' ? JSON.stringify(cursor) : String(cursor);
}

export function buildConfig(env = process.env) {
  const apiBaseUrl = (env.API_BASE_URL || 'http://127.0.0.1:8081/api/v1').replace(/\/+$/, '');
  const timeoutSeconds = Number(env.SMOKE_TIMEOUT || 10);
  const runId = env.SMOKE_RUN_ID || `${new Date().toISOString().replace(/\D/g, '').slice(0, 14)}-${process.pid}`;

  return {
    apiBaseUrl,
    username: env.SMOKE_USERNAME || 'admin',
    password: env.SMOKE_PASSWORD || 'admin123',
    timeoutSeconds: Number.isFinite(timeoutSeconds) && timeoutSeconds > 0 ? timeoutSeconds : 10,
    runId,
    safeRunId: normalizeRunId(runId),
  };
}

export function decodeTextCaptchaCode(imageBase64) {
  const png = decodePng(Buffer.from(imageBase64, 'base64'));
  if (png.width !== CAPTCHA_WIDTH || png.height !== CAPTCHA_HEIGHT) {
    throw new Error(`unexpected captcha dimensions: ${png.width}x${png.height}`);
  }

  let code = '';
  for (let i = 0; i < CAPTCHA_LENGTH; i += 1) {
    const pattern = readCaptchaGlyphPattern(png, CAPTCHA_LEFT + i * CAPTCHA_STEP, CAPTCHA_TOP);
    code += matchCaptchaGlyph(pattern);
  }
  return code;
}

function readCaptchaGlyphPattern(png, left, top) {
  const pattern = [];
  for (let row = 0; row < 7; row += 1) {
    let bits = 0;
    for (let col = 0; col < 5; col += 1) {
      if (captchaGlyphBlockHasForeground(png, left + col * 4, top + row * 4)) {
        bits |= 1 << (4 - col);
      }
    }
    pattern.push(bits);
  }
  return pattern;
}

function matchCaptchaGlyph(pattern) {
  for (const [ch, glyph] of textCaptchaGlyphs) {
    if (glyph.length === pattern.length && glyph.every((row, index) => row === pattern[index])) {
      return ch;
    }
  }
  throw new Error(`unrecognized captcha glyph: ${pattern.map((row) => row.toString(16).padStart(2, '0')).join(',')}`);
}

function isCaptchaForegroundPixel({ r, g, b, a }) {
  return a > 200 && r < 80 && g < 120 && b < 200;
}

function captchaGlyphBlockHasForeground(png, left, top) {
  for (let y = top; y < top + 3; y += 1) {
    for (let x = left; x < left + 3; x += 1) {
      if (isCaptchaForegroundPixel(getPngPixel(png, x, y))) {
        return true;
      }
    }
  }
  return false;
}

function decodePng(buffer) {
  const signature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  if (buffer.length < signature.length || !buffer.subarray(0, signature.length).equals(signature)) {
    throw new Error('captcha image is not a PNG');
  }

  let offset = signature.length;
  let width = 0;
  let height = 0;
  let bitDepth = 0;
  let colorType = 0;
  const idatChunks = [];

  while (offset + 12 <= buffer.length) {
    const length = buffer.readUInt32BE(offset);
    const type = buffer.subarray(offset + 4, offset + 8).toString('ascii');
    const dataStart = offset + 8;
    const dataEnd = dataStart + length;
    if (dataEnd + 4 > buffer.length) {
      throw new Error(`invalid PNG chunk length for ${type}`);
    }
    const data = buffer.subarray(dataStart, dataEnd);

    if (type === 'IHDR') {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8];
      colorType = data[9];
      if (data[10] !== 0 || data[11] !== 0 || data[12] !== 0) {
        throw new Error('unsupported PNG compression, filter, or interlace mode');
      }
    } else if (type === 'IDAT') {
      idatChunks.push(data);
    } else if (type === 'IEND') {
      break;
    }
    offset = dataEnd + 4;
  }

  if (width <= 0 || height <= 0 || idatChunks.length === 0) {
    throw new Error('invalid PNG captcha payload');
  }
  if (bitDepth !== 8 || (colorType !== 2 && colorType !== 6)) {
    throw new Error(`unsupported PNG captcha format: bitDepth=${bitDepth}, colorType=${colorType}`);
  }

  const channels = colorType === 6 ? 4 : 3;
  const bytesPerPixel = channels;
  const stride = width * channels;
  const inflated = inflateSync(Buffer.concat(idatChunks));
  const pixels = Buffer.alloc(height * stride);
  let inputOffset = 0;

  for (let y = 0; y < height; y += 1) {
    const filter = inflated[inputOffset];
    inputOffset += 1;
    const row = inflated.subarray(inputOffset, inputOffset + stride);
    inputOffset += stride;
    const outStart = y * stride;
    const prevStart = y > 0 ? (y - 1) * stride : -1;

    for (let x = 0; x < stride; x += 1) {
      const left = x >= bytesPerPixel ? pixels[outStart + x - bytesPerPixel] : 0;
      const up = prevStart >= 0 ? pixels[prevStart + x] : 0;
      const upLeft = prevStart >= 0 && x >= bytesPerPixel ? pixels[prevStart + x - bytesPerPixel] : 0;
      let value;
      switch (filter) {
        case 0:
          value = row[x];
          break;
        case 1:
          value = row[x] + left;
          break;
        case 2:
          value = row[x] + up;
          break;
        case 3:
          value = row[x] + Math.floor((left + up) / 2);
          break;
        case 4:
          value = row[x] + paethPredictor(left, up, upLeft);
          break;
        default:
          throw new Error(`unsupported PNG filter: ${filter}`);
      }
      pixels[outStart + x] = value & 0xff;
    }
  }

  return { width, height, channels, pixels };
}

function getPngPixel(png, x, y) {
  const offset = (y * png.width + x) * png.channels;
  return {
    r: png.pixels[offset],
    g: png.pixels[offset + 1],
    b: png.pixels[offset + 2],
    a: png.channels === 4 ? png.pixels[offset + 3] : 255,
  };
}

function paethPredictor(left, up, upLeft) {
  const p = left + up - upLeft;
  const pa = Math.abs(p - left);
  const pb = Math.abs(p - up);
  const pc = Math.abs(p - upLeft);
  if (pa <= pb && pa <= pc) return left;
  if (pb <= pc) return up;
  return upLeft;
}
