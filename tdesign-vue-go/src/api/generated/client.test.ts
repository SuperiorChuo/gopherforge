import { beforeEach, describe, expect, it, vi } from 'vitest';

const request = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
  patch: vi.fn(),
}));

vi.mock('@/utils/request', () => ({
  request,
}));

import { buildApiPath, typedApi } from './client';

describe('generated typed API client runtime behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('encodes path parameters and reports missing parameters', () => {
    expect(buildApiPath('/api/v1/users/{id}/roles/{role_id}', { id: 'a/b c', role_id: 7 })).toBe(
      '/api/v1/users/a%2Fb%20c/roles/7',
    );

    expect(() => buildApiPath('/api/v1/users/{id}', {})).toThrow('Missing path parameter: id');
  });

  it('strips the api prefix and forwards path params, query, and request options', async () => {
    request.get.mockResolvedValue({ ok: true });

    await typedApi.get('/api/v1/login-logs/user/{user_id}', {
      path: {
        user_id: 7,
      },
      query: {
        page: 2,
        username: 'alice',
      },
      withToken: false,
    });

    expect(request.get).toHaveBeenCalledWith(
      {
        url: '/login-logs/user/7',
        params: {
          page: 2,
          username: 'alice',
        },
        data: undefined,
      },
      {
        withToken: false,
      },
    );
  });

  it('forwards json request bodies separately from request options', async () => {
    request.post.mockResolvedValue({ ok: true });

    const body = {
      username: 'admin',
      password: 'admin123',
      captcha_id: 'captcha-id',
      captcha_code: '1234',
    };

    await typedApi.post('/api/v1/login', {
      body,
      withToken: false,
    });

    expect(request.post).toHaveBeenCalledWith(
      {
        url: '/login',
        params: undefined,
        data: body,
      },
      {
        withToken: false,
      },
    );
  });
});
