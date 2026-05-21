import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { resolveProtectedRouteDecision } from './guard';

const permissionSource = readFileSync(fileURLToPath(new URL('../../permission.ts', import.meta.url)), 'utf8');
const mojibakeFragments = ['\u74BA', '\u947E', '\u6FE1', '\u509B', '\u7049', '\u9422', '\u3126'];

describe('route guard decisions', () => {
  it('redirects unmatched protected routes to the 404 result page', () => {
    const decision = resolveProtectedRouteDecision(
      { name: 'MissingRoute' },
      () => false,
      true,
    );

    expect(decision).toEqual({ path: '/result/404', replace: true });
  });

  it('redirects matched routes without permission to the 403 result page', () => {
    const decision = resolveProtectedRouteDecision(
      { name: 'AdminRoute' },
      () => true,
      false,
    );

    expect(decision).toEqual({ path: '/result/403', replace: true });
  });

  it('allows matched routes when permission checks pass', () => {
    const decision = resolveProtectedRouteDecision(
      { name: 'Dashboard' },
      () => true,
      true,
    );

    expect(decision).toBe(true);
  });
});

describe('route guard source text', () => {
  it('keeps permission guard comments and fallback messages readable', () => {
    expect(mojibakeFragments.some((fragment) => permissionSource.includes(fragment))).toBe(false);
    expect(permissionSource).toContain('Failed to fetch user information');
  });
});
