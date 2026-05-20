export default {
  testDir: '.',
  timeout: 30000,
  use: {
    baseURL: process.env.FRONTEND_BASE_URL || 'http://127.0.0.1:3000',
    trace: 'retain-on-failure'
  }
};
