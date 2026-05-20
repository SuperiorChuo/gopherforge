import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import loginLocale from './lang/zh_CN/pages/login';

const currentDir = path.dirname(fileURLToPath(import.meta.url));
const srcRoot = path.resolve(currentDir, '..');

describe('中文化契约', () => {
  it('只保留简体中文语言包', () => {
    expect(existsSync(path.join(currentDir, 'lang/en_US'))).toBe(false);
  });

  it('登录页占位文案不展示默认账号或默认密码', () => {
    expect(loginLocale.input.account).toBe('请输入账号');
    expect(loginLocale.input.password).toBe('请输入登录密码');
    expect(`${loginLocale.input.account} ${loginLocale.input.password}`).not.toMatch(/admin/i);
  });

  it('登录表单不预填默认账号或默认密码', () => {
    const loginComponent = readFileSync(path.join(srcRoot, 'pages/login/components/Login.vue'), 'utf8');
    expect(loginComponent).not.toMatch(/account:\s*['"]admin['"]/);
    expect(loginComponent).not.toMatch(/password:\s*['"]admin123['"]/);
  });

  it('顶栏不提供语言切换入口', () => {
    const header = readFileSync(path.join(srcRoot, 'layouts/components/Header.vue'), 'utf8');
    expect(header).not.toContain('TranslateIcon');
    expect(header).not.toContain('langList');
    expect(header).not.toContain('<translate-icon');
  });
});
