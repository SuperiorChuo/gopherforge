import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const componentSource = readFileSync(fileURLToPath(new URL('./index.vue', import.meta.url)), 'utf8');

describe('slide captcha', () => {
  it('不依赖后端返回验证码明文提示字段', () => {
    expect(componentSource).not.toContain('code_hint');
    expect(componentSource).not.toContain('captchaText');
  });
});
