import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const projectRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

function readProjectFile(relativePath: string) {
  return readFileSync(path.join(projectRoot, relativePath), 'utf8');
}

describe('dev server guard contract', () => {
  it('uses the same default port as Vite when no explicit port is provided', () => {
    const viteConfig = readProjectFile('vite.config.ts');
    const devServerScript = readProjectFile('scripts/dev-server.mjs');

    expect(viteConfig).toContain('port: 3002');
    expect(devServerScript).toContain('DEFAULT_DEV_SERVER_PORT = 3002');
    expect(devServerScript).not.toContain(': 5173');
  });
});
