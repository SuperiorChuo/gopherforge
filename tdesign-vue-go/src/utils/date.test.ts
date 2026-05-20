import { describe, expect, it } from 'vitest';

import { formatDateOnly, formatDateTime } from './date';

describe('date utils', () => {
  it('格式化日期时间', () => {
    expect(formatDateTime('2026-05-20T12:34:56')).toBe('2026-05-20 12:34:56');
  });

  it('格式化日期', () => {
    expect(formatDateOnly('2026-05-20T12:34:56')).toBe('2026-05-20');
  });

  it('空值或非法日期返回占位符', () => {
    expect(formatDateTime()).toBe('-');
    expect(formatDateOnly('not-a-date')).toBe('-');
  });
});
