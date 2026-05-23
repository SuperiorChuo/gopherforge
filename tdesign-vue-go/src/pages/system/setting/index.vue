<template>
  <div class="setting-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>系统设置</h2>
        <t-tag :theme="dirty ? 'warning' : 'success'" variant="light">
          {{ dirty ? '存在未保存修改' : '配置已同步' }}
        </t-tag>
      </template>
      <template #meta>
        <span>站点信息</span>
        <span>邮件通知</span>
        <span>安全策略</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-button theme="primary" :loading="saving" @click="handleSave">
          <template #icon><t-icon name="save" /></template>
          保存设置
        </t-button>
        <t-button variant="outline" :loading="loading" @click="loadSettings">
          <template #icon><t-icon name="refresh" /></template>
          刷新
        </t-button>
      </template>
    </console-page-header>

    <div class="summary-grid">
      <section v-for="item in summaryItems" :key="item.label" class="summary-panel" :class="`summary-panel--${item.tone}`">
        <div class="summary-panel__main">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
        <span class="summary-panel__icon"><t-icon :name="item.icon" /></span>
      </section>
    </div>

    <t-card :bordered="false" class="settings-card">
      <t-tabs v-model="activeTab" class="settings-tabs">
        <t-tab-panel value="site" label="站点信息">
          <t-form :data="siteForm" label-width="108px" class="settings-form">
            <div class="form-grid">
              <t-form-item label="站点名称">
                <t-input v-model="siteForm.site_name" placeholder="后台管理系统" @change="markDirty('site')" />
              </t-form-item>
              <t-form-item label="Logo 地址">
                <t-input v-model="siteForm.logo_url" placeholder="https://example.com/logo.png" @change="markDirty('site')" />
              </t-form-item>
              <t-form-item class="form-grid__full" label="页脚文案">
                <t-input v-model="siteForm.footer_text" placeholder="© 2021-2026 后台管理系统" @change="markDirty('site')" />
              </t-form-item>
            </div>
          </t-form>
        </t-tab-panel>

        <t-tab-panel value="email" label="邮件通知">
          <t-form :data="emailForm" label-width="108px" class="settings-form">
            <div class="form-grid">
              <t-form-item label="启用邮件">
                <t-switch v-model="emailForm.enabled" @change="markDirty('email')" />
              </t-form-item>
              <t-form-item label="TLS 加密">
                <t-switch v-model="emailForm.use_tls" @change="handleUseTlsChange" />
              </t-form-item>
              <t-form-item label="STARTTLS 加密">
                <t-switch v-model="emailForm.start_tls" @change="handleStartTlsChange" />
              </t-form-item>
              <t-form-item label="SMTP 主机">
                <t-input v-model="emailForm.smtp_host" placeholder="smtp.example.com" @change="markDirty('email')" />
              </t-form-item>
              <t-form-item label="发件人">
                <t-input v-model="emailForm.sender" placeholder="admin@example.com" @change="markDirty('email')" />
              </t-form-item>
              <t-form-item label="告警收件人">
                <t-input v-model="emailForm.alert_receiver" placeholder="ops@example.com" @change="markDirty('email')" />
              </t-form-item>
              <t-form-item class="form-grid__full" label="主题模板">
                <t-textarea
                  v-model="emailForm.subject_template"
                  :autosize="{ minRows: 2, maxRows: 4 }"
                  placeholder="系统通知：{{title}}"
                  @change="markDirty('email')"
                />
              </t-form-item>
              <t-form-item class="form-grid__full" label="正文模板">
                <t-textarea
                  v-model="emailForm.body_template"
                  :autosize="{ minRows: 4, maxRows: 8 }"
                  placeholder="{{content}}"
                  @change="markDirty('email')"
                />
              </t-form-item>
              <t-form-item class="form-grid__full" label="收件组">
                <t-textarea
                  v-model="recipientGroupsText"
                  :autosize="{ minRows: 3, maxRows: 7 }"
                  placeholder='{ "notice": ["ops@example.com"] }'
                  @change="markDirty('email')"
                />
              </t-form-item>
            </div>
          </t-form>
        </t-tab-panel>

        <t-tab-panel value="security" label="安全策略">
          <t-form :data="securityForm" label-width="132px" class="settings-form">
            <div class="form-grid">
              <t-form-item label="密码最长有效期">
                <t-input-number
                  v-model="securityForm.password_max_age_days"
                  :min="0"
                  :max="365"
                  suffix="天"
                  @change="markDirty('security')"
                />
              </t-form-item>
              <t-form-item label="密码历史数量">
                <t-input-number v-model="securityForm.password_history_count" :min="0" :max="20" @change="markDirty('security')" />
              </t-form-item>
              <t-form-item label="登录失败阈值">
                <t-input-number v-model="securityForm.login_limit_max_failures" :min="1" :max="20" @change="markDirty('security')" />
              </t-form-item>
              <t-form-item label="登录失败统计窗口">
                <t-input-number
                  v-model="securityForm.login_limit_window_minutes"
                  :min="1"
                  :max="1440"
                  suffix="分钟"
                  @change="markDirty('security')"
                />
              </t-form-item>
              <t-form-item label="锁定时长">
                <t-input-number
                  v-model="securityForm.login_limit_lock_minutes"
                  :min="1"
                  :max="1440"
                  suffix="分钟"
                  @change="markDirty('security')"
                />
              </t-form-item>
              <t-form-item label="接口限流阈值">
                <t-input-number v-model="securityForm.rate_limit_rps" :min="1" :max="1000" suffix="次/秒" @change="markDirty('security')" />
              </t-form-item>
            </div>
          </t-form>
        </t-tab-panel>
      </t-tabs>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import { batchUpdateSystemSettings, getSystemSettings, type SystemSettingItem } from '@/api/system/setting';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';
import { formatDateTime } from '@/utils/date';

type SummaryTone = 'blue' | 'green' | 'orange' | 'red';
type SettingGroup = 'site' | 'email' | 'security';
interface SettingPayload {
  setting_key: string;
  value_json: any;
}

defineOptions({
  name: 'SystemSetting',
});

const activeTab = ref('site');
const loading = ref(false);
const saving = ref(false);
const dirty = ref(false);
const dirtyGroups = ref(new Set<SettingGroup>());
const settings = ref<SystemSettingItem[]>([]);

const siteForm = ref({
  site_name: '后台管理系统',
  logo_url: '',
  footer_text: '© 2021-2026 后台管理系统',
});

const emailForm = ref({
  enabled: false,
  use_tls: false,
  start_tls: false,
  smtp_host: '',
  sender: '',
  alert_receiver: '',
  subject_template: '',
  body_template: '',
  recipient_groups: { notice: [] as string[] },
});

const recipientGroupsText = ref(JSON.stringify(emailForm.value.recipient_groups, null, 2));

const securityForm = ref({
  password_max_age_days: 90,
  password_history_count: 5,
  login_limit_max_failures: 5,
  login_limit_window_minutes: 15,
  login_limit_lock_minutes: 30,
  rate_limit_rps: 100,
});

const lastUpdatedAt = computed(() => {
  const latest = settings.value
    .map((item) => item.updated_at)
    .filter(Boolean)
    .sort()
    .at(-1);
  return latest ? formatDateTime(latest) : '';
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '站点名称',
    value: siteForm.value.site_name || '未设置',
    hint: siteForm.value.logo_url ? '已配置 Logo' : '未配置 Logo',
    icon: 'desktop',
    tone: 'blue',
  },
  {
    label: '邮件通知',
    value: emailForm.value.enabled ? '已启用' : '未启用',
    hint: emailForm.value.sender || '未配置发件人',
    icon: 'mail',
    tone: emailForm.value.enabled ? 'green' : 'orange',
  },
  {
    label: '密码有效期',
    value: `${securityForm.value.password_max_age_days} 天`,
    hint: `历史 ${securityForm.value.password_history_count} 次不可复用`,
    icon: 'lock-on',
    tone: 'green',
  },
  {
    label: '登录保护',
    value: securityForm.value.login_limit_max_failures,
    hint: `${securityForm.value.login_limit_window_minutes} 分钟内失败后锁定 ${securityForm.value.login_limit_lock_minutes} 分钟`,
    icon: 'secured',
    tone: 'blue',
  },
  {
    label: '接口限流',
    value: `${securityForm.value.rate_limit_rps} 次/秒`,
    hint: '运行时安全策略',
    icon: 'chart-bubble',
    tone: 'orange',
  },
]);

const markDirty = (group: SettingGroup) => {
  dirty.value = true;
  dirtyGroups.value = new Set(dirtyGroups.value).add(group);
};

const handleUseTlsChange = (value: unknown) => {
  if (value === true) {
    emailForm.value.start_tls = false;
  }
  markDirty('email');
};

const handleStartTlsChange = (value: unknown) => {
  if (value === true) {
    emailForm.value.use_tls = false;
  }
  markDirty('email');
};

const valueOf = (key: string) => settings.value.find((item) => item.setting_key === key)?.value_json;

const stringifyRecipientGroups = (value: unknown) => JSON.stringify(value || { notice: [] }, null, 2);

const mergeSettings = () => {
  const site = valueOf('site.profile');
  if (site) siteForm.value = { ...siteForm.value, ...site };

  const email = valueOf('notification.email');
  if (email) {
    emailForm.value = { ...emailForm.value, ...email };
    recipientGroupsText.value = stringifyRecipientGroups(emailForm.value.recipient_groups);
  }

  const security = valueOf('security.policy');
  if (security) securityForm.value = { ...securityForm.value, ...security };
};

const loadSettings = async () => {
  loading.value = true;
  try {
    settings.value = await getSystemSettings();
    mergeSettings();
    dirtyGroups.value = new Set();
    dirty.value = false;
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载系统设置失败');
  } finally {
    loading.value = false;
  }
};

const handleSave = async () => {
  if (dirtyGroups.value.has('email') && !syncRecipientGroupsFromText()) {
    return;
  }

  const settingsToSave: SettingPayload[] = [];
  pushSettingIfDirty('site.profile', 'site', siteForm.value, settingsToSave);
  pushSettingIfDirty('notification.email', 'email', emailForm.value, settingsToSave);
  pushSettingIfDirty('security.policy', 'security', securityForm.value, settingsToSave);
  if (!settingsToSave.length) {
    MessagePlugin.info('没有需要保存的修改');
    return;
  }

  saving.value = true;
  try {
    mergeUpdatedSettings(await batchUpdateSystemSettings({ settings: settingsToSave }));
    dirtyGroups.value = new Set();
    dirty.value = false;
    MessagePlugin.success('系统设置已保存');
  } catch (error: any) {
    MessagePlugin.error(error.message || '保存系统设置失败');
  } finally {
    saving.value = false;
  }
};

const pushSettingIfDirty = (settingKey: string, group: SettingGroup, valueJSON: any, target: SettingPayload[]) => {
  if (dirtyGroups.value.has(group)) {
    target.push({ setting_key: settingKey, value_json: valueJSON });
  }
};

const syncRecipientGroupsFromText = () => {
  try {
    const parsed = JSON.parse(recipientGroupsText.value || '{}');
    emailForm.value.recipient_groups = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : { notice: [] };
    recipientGroupsText.value = stringifyRecipientGroups(emailForm.value.recipient_groups);
    return true;
  } catch {
    MessagePlugin.error('收件组 JSON 格式不正确');
    return false;
  }
};

const mergeUpdatedSettings = (updated: SystemSettingItem[]) => {
  const byKey = new Map(settings.value.map((item) => [item.setting_key, item]));
  for (const item of updated) {
    byKey.set(item.setting_key, item);
  }
  settings.value = Array.from(byKey.values());
};

onMounted(() => {
  loadSettings();
});
</script>

<style lang="less" scoped>
@import '@/style/system-management.less';

.setting-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.settings-card {
  border: 1px solid #e8edf5;
  border-radius: 10px;
  box-shadow: 0 10px 24px rgb(15 23 42 / 5%);
}

.settings-tabs :deep(.t-tabs__content) {
  padding-top: 18px;
}

.settings-form {
  max-width: 920px;
}

.form-grid {
  display: grid;
  gap: 14px 18px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.form-grid__full {
  grid-column: 1 / -1;
}

.setting-page :deep(.t-input-number) {
  width: 100%;
}

@media (width <= 768px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
