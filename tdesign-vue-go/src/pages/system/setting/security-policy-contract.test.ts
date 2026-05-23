import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const readFixture = (relativePath: string) =>
  readFileSync(fileURLToPath(new URL(relativePath, import.meta.url)), 'utf8');

describe('system setting security policy contract', () => {
  const securityPolicyKeys = [
    'password_max_age_days',
    'password_history_count',
    'login_limit_max_failures',
    'login_limit_window_minutes',
    'login_limit_lock_minutes',
    'rate_limit_rps',
  ];
  const emailTlsKeys = ['use_tls', 'start_tls'];
  const emailTemplateKeys = ['subject_template', 'body_template'];

  it('exposes every backend supported security.policy key in the settings form', () => {
    const source = readFixture('./index.vue');

    for (const key of securityPolicyKeys) {
      expect(source).toContain(`securityForm.${key}`);
    }

    expect(source).toContain("pushSettingIfDirty('security.policy'");
  });

  it('labels the login limit duration fields with minute based controls', () => {
    const source = readFixture('./index.vue');

    expect(source).toContain('登录失败统计窗口');
    expect(source).toContain('锁定时长');
    expect(source).toContain('suffix="分钟"');
  });

  it('exposes the email TLS toggles with readable Chinese labels', () => {
    const source = readFixture('./index.vue');

    for (const key of emailTlsKeys) {
      expect(source).toContain(`emailForm.${key}`);
    }

    expect(source).toContain('label="TLS 加密"');
    expect(source).toContain('label="STARTTLS 加密"');
  });

  it('exposes email template and recipient group fields in notification.email', () => {
    const source = readFixture('./index.vue');

    for (const key of emailTemplateKeys) {
      expect(source).toContain(`emailForm.${key}`);
    }

    expect(source).toContain('recipient_groups');
    expect(source).toContain('emailForm.value.recipient_groups');
    expect(source).toContain('label="主题模板"');
    expect(source).toContain('label="正文模板"');
    expect(source).toContain('label="收件组"');
    expect(source).toContain("pushSettingIfDirty('notification.email', 'email', emailForm.value, settingsToSave)");
  });

  it('only submits setting groups that were edited', () => {
    const source = readFixture('./index.vue');

    expect(source).toContain('dirtyGroups');
    expect(source).toContain("markDirty('email')");
    expect(source).toContain("pushSettingIfDirty('notification.email', 'email', emailForm.value, settingsToSave)");
  });
});
