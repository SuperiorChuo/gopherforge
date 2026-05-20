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
