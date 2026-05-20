import { expect, test } from '@playwright/test';

const username = process.env.E2E_USERNAME || 'admin';
const password = process.env.E2E_PASSWORD || 'admin123';

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
  await page.goto('/login?redirect=%252Fdashboard%252Findex');

  await page.getByPlaceholder('请输入账号').fill(username);
  await page.getByPlaceholder('请输入登录密码').fill(password);
  await page.getByRole('button', { name: /^登录$/ }).click();

  await expect(page.getByText('请输入验证码')).toBeVisible();
  const captchaText = page.locator('.captcha-text');
  await expect(captchaText).toBeVisible();

  const captchaCode = ((await captchaText.textContent()) || '').replace(/\s/g, '');
  await page.getByPlaceholder('输入图中验证码').fill(captchaCode);
  await page.getByRole('button', { name: /^确认$/ }).click();

  await expect(page).toHaveURL(/\/dashboard\/index/);
  await expect(page.getByRole('heading', { name: '后台管理系统' })).toBeVisible();
  await expect(page.getByText('后端能力')).toBeVisible();
  await expect(page.getByText('前端能力')).toBeVisible();
  await expect(page.locator('body')).not.toContainText(/Go Admin Kit|MANAGEMENT CONSOLE/);
});
