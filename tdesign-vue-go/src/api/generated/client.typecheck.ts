import { typedApi } from './client';

void typedApi.post('/api/v1/login', {
  body: {
    username: 'admin',
    password: 'admin123',
    captcha_id: 'captcha-id',
    captcha_code: '1234',
  },
  withToken: false,
});

void typedApi.get('/api/v1/users/{id}', {
  path: {
    id: 1,
  },
});

void typedApi.post('/api/v1/users/{id}/roles', {
  path: {
    id: 1,
  },
  body: {
    role_ids: [1, 2],
  },
});

void typedApi.post('/api/v1/login', {
  // @ts-expect-error missing username is rejected by generated contract
  body: {
    password: 'admin123',
    captcha_id: 'captcha-id',
    captcha_code: '1234',
  },
  withToken: false,
});

void typedApi.post('/api/v1/departments', {
  body: {
    name: '研发部',
    code: 'rd',
  },
});

void typedApi.put('/api/v1/notices/{id}/status', {
  path: {
    id: 1,
  },
  body: {
    status: 1,
  },
});

void typedApi.get('/api/v1/login-logs/trend', {
  query: {
    days: 7,
  },
});

// @ts-expect-error required query parameters must be provided
void typedApi.get('/api/v1/files/hash/check');

void typedApi.get('/api/v1/files/hash/check', {
  // @ts-expect-error required query fields must be provided
  query: {},
});

void typedApi.get('/api/v1/files/hash/check', {
  query: {
    hash: 'sha256',
  },
});

void typedApi.delete('/api/v1/online-users/{token_id}', {
  path: {
    token_id: 'token-id',
  },
});

void typedApi.post('/api/v1/departments', {
  // @ts-expect-error department name is required by generated contract
  body: {
    code: 'rd',
  },
});

void typedApi.put('/api/v1/notices/{id}/status', {
  path: {
    id: 1,
  },
  body: {
    // @ts-expect-error status must be a number
    status: 'enabled',
  },
});
