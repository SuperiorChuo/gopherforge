import { describe, expect, it } from 'vitest';

import { resolveProtectedRouteDecision } from './guard';

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
