import { describe, expect, it } from 'vitest';

import { normalizeRedirectUrl } from './utils';

describe('normalizeRedirectUrl', () => {
  it('解码 TDesign Starter 风格的双重 redirect 参数', () => {
    expect(normalizeRedirectUrl('%252Fdashboard%252Fbase')).toBe('/dashboard/base');
  });

  it('保留站内普通路径', () => {
    expect(normalizeRedirectUrl('/system/user')).toBe('/system/user');
  });

  it('拒绝外部地址和协议相对地址', () => {
    expect(normalizeRedirectUrl('https://example.com/dashboard')).toBe('/dashboard/index');
    expect(normalizeRedirectUrl('//example.com/dashboard')).toBe('/dashboard/index');
  });

  it('空值回退到默认仪表盘', () => {
    expect(normalizeRedirectUrl()).toBe('/dashboard/index');
  });
});
