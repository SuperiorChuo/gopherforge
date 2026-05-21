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

async function mockFrontendLoginApis(page: Page) {
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
          access_token: 'e2e-access-token',
          refresh_token: 'e2e-refresh-token',
          user: e2eUser,
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
