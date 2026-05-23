import { expect, type Page, test } from '@playwright/test';

const username = process.env.E2E_USERNAME || 'admin';
const password = process.env.E2E_PASSWORD || 'admin123';
const e2eCaptchaKey = 'e2e-captcha-key';
const e2eCaptchaCode = '1234';
const e2eUser = {
  id: 1,
  username,
  nickname: 'E2E Admin',
  status: 1,
  must_change_password: false,
  roles: [{ id: 1, name: 'Super Admin', code: 'super_admin', status: 1 }],
  permissions: ['*:*:*'],
};
const e2ePng =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lw9kGQAAAABJRU5ErkJggg==';

async function mockFrontendLoginApis(page: Page, options: { requiresTotp?: boolean; expectedTotpCode?: string } = {}) {
  const challengeID = 'e2e-totp-challenge';
  const expectedTotpCode = options.expectedTotpCode || '123456';

  await page.route('**/api/v1/captcha**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: { key: e2eCaptchaKey, image: e2ePng },
      }),
    });
  });

  await page.route('**/api/v1/login', async (route) => {
    const body = route.request().postDataJSON();
    expect(body.username).toBe(username);
    expect(body.password).toBe(password);
    expect(body.captcha_id).toBe(e2eCaptchaKey);
    expect(body.captcha_code).toBe(e2eCaptchaCode);

    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: {
          access_token: options.requiresTotp ? undefined : 'e2e-access-token',
          refresh_token: options.requiresTotp ? undefined : 'e2e-refresh-token',
          requires_totp: !!options.requiresTotp,
          totp_challenge_id: options.requiresTotp ? challengeID : undefined,
          user: { ...e2eUser, totp_enabled: !!options.requiresTotp },
        },
      }),
    });
  });

  await page.route('**/api/v1/login/2fa/verify', async (route) => {
    const body = route.request().postDataJSON();
    expect(body.challenge_id).toBe(challengeID);
    expect(body.code).toBe(expectedTotpCode);

    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: {
          access_token: 'e2e-access-token',
          refresh_token: 'e2e-refresh-token',
          requires_totp: false,
          user: { ...e2eUser, totp_enabled: true },
        },
      }),
    });
  });

  await page.route('**/api/v1/user/me**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, message: 'ok', data: e2eUser }),
    });
  });

  await page.route('**/api/v1/user/menus**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: [
          {
            id: 1,
            path: '/e2e-menu',
            name: 'E2EMenu',
            component: 'LAYOUT',
            title: 'E2E',
            sort: 1,
            status: 1,
            hidden: 1,
            children: [
              {
                id: 2,
                parent_id: 1,
                path: 'dashboard',
                name: 'E2EMenuDashboard',
                component: '/dashboard/index',
                title: 'Dashboard',
                sort: 1,
                status: 1,
                hidden: 1,
              },
            ],
          },
        ],
      }),
    });
  });

  await page.route('**/api/v1/ws/notifications/ticket', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, message: 'ok', data: { ticket: 'e2e-notification-ticket' } }),
    });
  });

  await page.route('**/api/v1/health**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: {
          status: 'ok',
          runtime: { go_version: 'go1.26.3', os: 'linux', arch: 'amd64', compiler: 'gc' },
        },
      }),
    });
  });
}

async function loginByPassword(page: Page, redirect = '/dashboard/index') {
  await page.goto(`/login?redirect=${encodeURIComponent(encodeURIComponent(redirect))}`);
  await page.getByPlaceholder('请输入账号').fill(username);
  await page.getByPlaceholder('请输入登录密码').fill(password);
  await page.getByRole('button', { name: /^登录$/ }).click();
  await page.getByPlaceholder('输入图中验证码').fill(e2eCaptchaCode);
  await page.getByRole('button', { name: /^确认$/ }).click();
  await expect(page).toHaveURL(new RegExp(redirect.replaceAll('/', '\\/')));
}

test.beforeEach(async ({ context, page }) => {
  await context.clearCookies();
  await page.addInitScript(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });
});

test('登录页保持中文控制台文案', async ({ page }) => {
  await page.goto('/login?redirect=%252Fdashboard%252Findex');

  await expect(page).toHaveURL(/\/login/);
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN');
  await expect(page.locator('h1')).toHaveText('后台管理系统');
  await expect(page.getByPlaceholder('请输入账号')).toBeVisible();
  await expect(page.getByPlaceholder('请输入登录密码')).toBeVisible();
  await expect(page.getByPlaceholder('请输入账号')).toHaveValue('');
  await expect(page.getByPlaceholder('请输入登录密码')).toHaveValue('');
  await expect(page.getByPlaceholder(/admin123/i)).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(/Go Admin Kit|MANAGEMENT CONSOLE|Login in|Sign in/);
});

test('账号密码登录后进入系统概览', async ({ page }) => {
  await mockFrontendLoginApis(page);
  await page.goto('/login?redirect=%252Fdashboard%252Findex');

  await page.getByPlaceholder('请输入账号').fill(username);
  await page.getByPlaceholder('请输入登录密码').fill(password);
  await page.getByRole('button', { name: /^登录$/ }).click();
  await expect(page.getByText('请输入验证码')).toBeVisible();
  await expect(page.locator('.text-captcha')).toBeVisible();
  await expect(page.locator('.captcha-image')).toBeVisible();

  await page.getByPlaceholder('输入图中验证码').fill(e2eCaptchaCode);
  await page.getByRole('button', { name: /^确认$/ }).click();

  await expect(page).toHaveURL(/\/dashboard\/index/);
  await expect(page.getByRole('heading', { name: '后台管理系统' })).toBeVisible();
  await expect(page.getByText('后端能力')).toBeVisible();
  await expect(page.getByText('前端能力')).toBeVisible();
  await expect(page.locator('body')).not.toContainText(/Go Admin Kit|MANAGEMENT CONSOLE/);
});

test('账号密码登录触发两步验证后进入系统概览', async ({ page }) => {
  await mockFrontendLoginApis(page, { requiresTotp: true });
  await page.goto('/login?redirect=%252Fdashboard%252Findex');

  await page.getByPlaceholder('请输入账号').fill(username);
  await page.getByPlaceholder('请输入登录密码').fill(password);
  await page.getByRole('button', { name: /^登录$/ }).click();

  await page.getByPlaceholder('输入图中验证码').fill(e2eCaptchaCode);
  await page.getByRole('button', { name: /^确认$/ }).click();

  await expect(page.getByRole('dialog', { name: '两步验证' })).toBeVisible();
  await page.getByPlaceholder('请输入验证码或恢复码').fill('123456');
  await page.getByRole('button', { name: /^验证$/ }).click();

  await expect(page).toHaveURL(/\/dashboard\/index/);
  await expect(page.getByRole('heading', { name: '后台管理系统' })).toBeVisible();
});

test('系统设置页可加载并批量保存配置', async ({ page }) => {
  await mockFrontendLoginApis(page);

  let savedSettings: any[] = [];
  await page.route('**/api/v1/system-settings**', async (route) => {
    const url = route.request().url();
    if (url.includes('/batch')) {
      const body = route.request().postDataJSON();
      savedSettings = body.settings;
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'ok', data: savedSettings }),
      });
      return;
    }

    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'ok',
        data: [
          { setting_key: 'site.profile', value_json: { site_name: 'Go Admin Kit', logo_url: '', footer_text: '© 2026' } },
          { setting_key: 'notification.email', value_json: { enabled: true, smtp_host: 'smtp.example.com', sender: 'ops@example.com' } },
          {
            setting_key: 'security.policy',
            value_json: { password_max_age_days: 90, password_history_count: 5, login_limit_max_failures: 5, rate_limit_rps: 100 },
          },
        ],
      }),
    });
  });

  await loginByPassword(page, '/system/setting');

  await expect(page.getByRole('heading', { name: '系统设置' })).toBeVisible();
  await expect(page.getByText('Go Admin Kit')).toBeVisible();
  await page.getByRole('textbox', { name: '后台管理系统', exact: true }).fill('Go Admin Kit Pro');
  await page.getByRole('button', { name: /^保存设置$/ }).click();

  await expect.poll(() => savedSettings.length).toBe(3);
  expect(savedSettings.find((item) => item.setting_key === 'site.profile')?.value_json.site_name).toBe('Go Admin Kit Pro');
});

test('账号密码登录可使用恢复码完成两步验证', async ({ page }) => {
  await mockFrontendLoginApis(page, { requiresTotp: true, expectedTotpCode: 'ABCDE-FGHIJ-KLMNO' });
  await page.goto('/login?redirect=%252Fdashboard%252Findex');

  await page.getByPlaceholder('请输入账号').fill(username);
  await page.getByPlaceholder('请输入登录密码').fill(password);
  await page.getByRole('button', { name: /^登录$/ }).click();

  await page.getByPlaceholder('输入图中验证码').fill(e2eCaptchaCode);
  await page.getByRole('button', { name: /^确认$/ }).click();

  await expect(page.getByRole('dialog', { name: '两步验证' })).toBeVisible();
  await page.getByPlaceholder('请输入验证码或恢复码').fill('ABCDE-FGHIJ-KLMNO');
  await page.getByRole('button', { name: /^验证$/ }).click();

  await expect(page).toHaveURL(/\/dashboard\/index/);
  await expect(page.getByRole('heading', { name: '后台管理系统' })).toBeVisible();
});
