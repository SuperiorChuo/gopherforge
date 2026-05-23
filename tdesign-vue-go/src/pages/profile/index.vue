<template>
  <div class="profile-page">
    <section class="profile-head">
      <div class="profile-head__main">
        <div class="profile-head__title">
          <h2>个人中心</h2>
          <t-tag :theme="accountStatusTheme" variant="light">{{ accountStatusLabel }}</t-tag>
          <t-tag v-if="mustChangePassword" theme="warning" variant="light">需要修改密码</t-tag>
        </div>
        <div class="profile-head__meta">
          <span>账号资料</span>
          <span>安全密码</span>
          <span>登录记录</span>
          <span>注册于 {{ formatDate(userInfo.created_at) }}</span>
        </div>
      </div>
      <t-space class="profile-head__actions" size="small" break-line>
        <t-button variant="outline" :loading="historyLoading" @click="loadLoginHistory">
          <template #icon><t-icon name="refresh" /></template>
          刷新记录
        </t-button>
      </t-space>
    </section>

    <div class="summary-grid">
      <section v-for="item in summaryItems" :key="item.label" class="summary-panel" :class="`summary-panel--${item.tone}`">
        <div class="summary-panel__main">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
        <span class="summary-panel__icon">
          <t-icon :name="item.icon" />
        </span>
      </section>
    </div>

    <div class="profile-grid">
      <aside class="profile-side">
        <section class="profile-card">
          <div class="avatar-section">
            <t-avatar class="profile-avatar" size="72px" :image="userInfo.avatar">
              <template #icon>
                <span>{{ avatarLetter }}</span>
              </template>
            </t-avatar>
            <div class="profile-name">
              <strong>{{ userInfo.nickname || userInfo.username || '未命名用户' }}</strong>
              <span>{{ userInfo.username || '-' }}</span>
            </div>
            <t-tag :theme="accountStatusTheme" variant="light">{{ accountStatusLabel }}</t-tag>
          </div>

          <div class="info-list">
            <div class="info-item">
              <span class="info-item__icon"><t-icon name="mail" /></span>
              <div>
                <small>邮箱</small>
                <strong>{{ userInfo.email || '暂无邮箱' }}</strong>
              </div>
            </div>
            <div class="info-item">
              <span class="info-item__icon"><t-icon name="mobile" /></span>
              <div>
                <small>手机号</small>
                <strong>{{ userInfo.phone || '暂无手机号' }}</strong>
              </div>
            </div>
            <div class="info-item">
              <span class="info-item__icon"><t-icon name="time" /></span>
              <div>
                <small>最近登录</small>
                <strong>{{ lastLoginText }}</strong>
              </div>
            </div>
          </div>

          <div class="role-block">
            <div class="role-block__head">
              <span>角色权限</span>
              <t-tag variant="light">共 {{ roleList.length }} 个</t-tag>
            </div>
            <div class="role-tags">
              <t-tag v-for="role in roleList" :key="role.id" theme="primary" variant="light">
                {{ role.name }}
              </t-tag>
              <t-tag v-if="!roleList.length" variant="light">暂无角色</t-tag>
            </div>
          </div>
        </section>
      </aside>

      <main class="profile-main">
        <section class="workspace-card">
          <t-tabs v-model="activeTab" class="profile-tabs">
            <t-tab-panel value="info" label="基本资料">
              <div class="panel-head">
                <div>
                  <h3>基本资料</h3>
                  <p>维护展示名称、邮箱、手机号和头像地址</p>
                </div>
                <t-tag theme="primary" variant="light">用户 ID：{{ userInfo.id || '-' }}</t-tag>
              </div>
              <t-form
                ref="infoFormRef"
                :data="infoForm"
                :rules="infoFormRules"
                label-width="92px"
                class="profile-form"
              >
                <div class="form-grid">
                  <t-form-item label="用户名">
                    <t-input v-model="infoForm.username" disabled />
                  </t-form-item>
                  <t-form-item label="昵称" name="nickname">
                    <t-input v-model="infoForm.nickname" placeholder="请输入昵称" />
                  </t-form-item>
                  <t-form-item label="邮箱" name="email">
                    <t-input v-model="infoForm.email" placeholder="请输入邮箱" />
                  </t-form-item>
                  <t-form-item label="手机号" name="phone">
                    <t-input v-model="infoForm.phone" placeholder="请输入手机号" />
                  </t-form-item>
                  <t-form-item class="form-grid__full" label="头像" name="avatar">
                    <t-input v-model="infoForm.avatar" placeholder="请输入头像 URL" />
                  </t-form-item>
                </div>
                <div class="form-actions">
                  <t-button theme="primary" :loading="infoLoading" @click="handleUpdateInfo">
                    <template #icon><t-icon name="save" /></template>
                    保存修改
                  </t-button>
                </div>
              </t-form>
            </t-tab-panel>

            <t-tab-panel value="password" label="安全密码">
              <div class="panel-head">
                <div>
                  <h3>安全密码</h3>
                  <p>密码需至少 8 位，并包含大小写字母和数字</p>
                </div>
                <t-tag :theme="mustChangePassword ? 'warning' : 'success'" variant="light">
                  {{ mustChangePassword ? '待更新' : '已满足策略' }}
                </t-tag>
              </div>
              <div v-if="mustChangePassword" class="force-banner">
                <t-icon name="lock-on" />
                <span>当前账号被要求更新密码，完成修改后可继续使用控制台能力。</span>
              </div>
              <t-form
                ref="pwdFormRef"
                :data="pwdForm"
                :rules="pwdFormRules"
                label-width="92px"
                class="profile-form password-form"
              >
                <t-form-item label="当前密码" name="old_password">
                  <t-input v-model="pwdForm.old_password" type="password" placeholder="请输入当前密码" />
                </t-form-item>
                <t-form-item label="新密码" name="new_password">
                  <t-input v-model="pwdForm.new_password" type="password" placeholder="请输入新密码" />
                </t-form-item>
                <t-form-item label="确认密码" name="confirm_password">
                  <t-input v-model="pwdForm.confirm_password" type="password" placeholder="请再次输入新密码" />
                </t-form-item>
                <div class="form-actions">
                  <t-button theme="primary" :loading="pwdLoading" @click="handleChangePassword">
                    <template #icon><t-icon name="lock-on" /></template>
                    修改密码
                  </t-button>
                </div>
              </t-form>

              <div class="totp-panel">
                <div class="totp-panel__head">
                  <div>
                    <h4>两步验证</h4>
                    <p>使用认证器应用生成的一次性验证码保护账号登录。</p>
                  </div>
                  <t-tag :theme="userInfo.totp_enabled ? 'success' : 'warning'" variant="light">
                    {{ userInfo.totp_enabled ? '已开启' : '未开启' }}
                  </t-tag>
                </div>

                <template v-if="!userInfo.totp_enabled">
                  <div v-if="totpSetup" class="totp-setup">
                    <qrcode-vue :value="totpSetup.otp_auth_url" :size="144" level="M" />
                    <div class="totp-setup__main">
                      <t-input :value="totpSetup.secret" readonly />
                      <div class="totp-setup__verify">
                        <t-input v-model="totpCurrentPassword" type="password" placeholder="当前密码" />
                        <t-input v-model="totpCode" maxlength="6" placeholder="请输入 6 位验证码" />
                        <t-button theme="primary" :loading="totpLoading" @click="handleEnableTotp">启用</t-button>
                      </div>
                    </div>
                  </div>
                  <div v-else class="totp-bind">
                    <t-input v-model="totpCurrentPassword" type="password" placeholder="当前密码" />
                    <t-button theme="primary" :loading="totpLoading" @click="handleGenerateTotp">
                    <template #icon><t-icon name="secured" /></template>
                    绑定认证器
                  </t-button>
                  </div>
                </template>

                <div v-else class="totp-disable">
                  <t-input v-model="totpCurrentPassword" type="password" placeholder="当前密码" />
                  <t-input v-model="totpCode" maxlength="6" placeholder="输入当前验证码" />
                  <t-button variant="outline" :loading="totpLoading" @click="handleRegenerateRecoveryCodes">
                    重新生成恢复码
                  </t-button>
                  <t-button theme="danger" :loading="totpLoading" @click="handleDisableTotp">关闭两步验证</t-button>
                </div>

                <div v-if="totpRecoveryCodes.length" class="totp-recovery">
                  <div class="totp-recovery__head">
                    <h5>恢复码</h5>
                    <t-tag size="small" theme="warning" variant="light">仅展示一次</t-tag>
                  </div>
                  <div class="totp-recovery__grid">
                    <span v-for="code in totpRecoveryCodes" :key="code" class="totp-recovery__code">{{ code }}</span>
                  </div>
                </div>
              </div>
            </t-tab-panel>

            <t-tab-panel value="login-history" label="登录记录">
              <div class="panel-head">
                <div>
                  <h3>登录记录</h3>
                  <p>最近 10 次登录行为，便于确认账号安全状态</p>
                </div>
                <t-tag :theme="loginFailedCount > 0 ? 'danger' : 'success'" variant="light">
                  失败 {{ loginFailedCount }} 次
                </t-tag>
              </div>
              <t-table
                class="login-table"
                :data="loginHistory"
                :columns="loginColumns"
                :loading="historyLoading"
                :pagination="undefined"
                row-key="id"
                table-layout="fixed"
                hover
              >
                <template #empty>
                  <t-empty :description="historyLoading ? '正在加载登录记录' : '暂无登录记录'" />
                </template>
                <template #ip="{ row }">
                  <span class="mono-text">{{ row.ip || '-' }}</span>
                </template>
                <template #location="{ row }">
                  <span>{{ row.location || '未知地点' }}</span>
                </template>
                <template #browser="{ row }">
                  <span>{{ row.browser || '未知浏览器' }}</span>
                </template>
                <template #os="{ row }">
                  <span>{{ row.os || '未知系统' }}</span>
                </template>
                <template #status="{ row }">
                  <t-tag :theme="row.status === 1 ? 'success' : 'danger'" size="small" variant="light">
                    {{ row.status === 1 ? '成功' : '失败' }}
                  </t-tag>
                </template>
                <template #message="{ row }">
                  <span class="message-text" :title="row.message">{{ row.message || '-' }}</span>
                </template>
                <template #created_at="{ row }">
                  <span class="mono-text">{{ formatDateTime(row.created_at) }}</span>
                </template>
              </t-table>
            </t-tab-panel>
          </t-tabs>
        </section>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import QrcodeVue from 'qrcode.vue';
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import {
  changePassword,
  disableTotp,
  enableTotp,
  generateTotpSetup,
  getCurrentUser,
  regenerateTotpRecoveryCodes,
  updateProfile,
  type TOTPRecoveryCodesResponse,
  type TOTPSetupResponse,
  type UserInfo,
} from '@/api/auth';
import { getMyLoginLogs, type LoginLogItem } from '@/api/system/loginLog';
import { useUserStore } from '@/store';
import { formatDateTime } from '@/utils/date';

type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';
type SummaryTone = 'blue' | 'green' | 'orange' | 'red';

defineOptions({
  name: 'Profile',
});

const userStore = useUserStore();
const route = useRoute();
const router = useRouter();

const activeTab = ref(route.query.force_change_password ? 'password' : 'info');
const userInfo = ref<Partial<UserInfo>>({});
const infoLoading = ref(false);
const pwdLoading = ref(false);
const totpLoading = ref(false);
const historyLoading = ref(false);
const loginHistory = ref<LoginLogItem[]>([]);
const totpSetup = ref<TOTPSetupResponse | null>(null);
const totpCode = ref('');
const totpCurrentPassword = ref('');
const totpRecoveryCodes = ref<string[]>([]);

const infoFormRef = ref();
const pwdFormRef = ref();

const infoForm = ref({
  username: '',
  nickname: '',
  email: '',
  phone: '',
  avatar: '',
});

const pwdForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: '',
});

const infoFormRules = {
  email: [{ email: true, message: '请输入正确的邮箱地址' }],
  phone: [
    {
      validator: (val: string) => !val || /^\+?[\d\s\-()]{5,20}$/.test(val),
      message: '请输入正确的手机号',
    },
  ],
};

const pwdFormRules = {
  old_password: [{ required: true, message: '请输入当前密码' }],
  new_password: [
    { required: true, message: '请输入新密码' },
    { min: 8, message: '密码长度不能少于8位' },
    {
      pattern: /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).+$/,
      message: '密码需包含大小写字母和数字',
    },
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (val: string) => val === pwdForm.value.new_password,
      message: '两次输入的密码不一致',
    },
  ],
};

const loginColumns = [
  { colKey: 'ip', title: 'IP 地址', minWidth: 150 },
  { colKey: 'location', title: '登录地点', minWidth: 140 },
  { colKey: 'browser', title: '浏览器', minWidth: 120 },
  { colKey: 'os', title: '操作系统', minWidth: 120 },
  { colKey: 'status', title: '状态', width: 86 },
  { colKey: 'message', title: '说明', minWidth: 170 },
  { colKey: 'created_at', title: '登录时间', width: 180 },
];

const roleList = computed(() => userInfo.value.roles || []);
const roleNames = computed(() => {
  if (!roleList.value.length) {
    return '暂无角色';
  }
  return roleList.value.map((r) => r.name).join(', ');
});

const formatDate = (dateStr?: string) => formatDateTime(dateStr, 'YYYY-MM-DD');

const accountStatusLabel = computed(() => {
  if (userInfo.value.status === 1) return '启用中';
  if (userInfo.value.status === 0) return '已停用';
  return '状态未知';
});

const accountStatusTheme = computed<TagTheme>(() => {
  if (userInfo.value.status === 1) return 'success';
  if (userInfo.value.status === 0) return 'danger';
  return 'default';
});

const mustChangePassword = computed(() => Boolean(route.query.force_change_password || userInfo.value.must_change_password));

const avatarLetter = computed(() => {
  const source = userInfo.value.nickname || userInfo.value.username || 'U';
  return source.slice(0, 1).toUpperCase();
});

const lastLogin = computed(() => loginHistory.value[0]);
const lastLoginText = computed(() => {
  if (!lastLogin.value?.created_at) return '暂无记录';
  return formatDateTime(lastLogin.value.created_at, 'YYYY-MM-DD HH:mm');
});

const loginSuccessCount = computed(() => loginHistory.value.filter((item) => item.status === 1).length);
const loginFailedCount = computed(() => loginHistory.value.filter((item) => item.status !== 1).length);
const loginRate = computed(() => {
  if (!loginHistory.value.length) return '0%';
  return `${Math.round((loginSuccessCount.value / loginHistory.value.length) * 100)}%`;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '账号状态',
    value: accountStatusLabel.value,
    hint: userInfo.value.username ? `登录名 ${userInfo.value.username}` : '等待加载用户资料',
    icon: 'user-circle',
    tone: userInfo.value.status === 1 ? 'green' : 'red',
  },
  {
    label: '角色数量',
    value: roleList.value.length,
    hint: roleNames.value,
    icon: 'usergroup',
    tone: 'blue',
  },
  {
    label: '登录成功率',
    value: loginRate.value,
    hint: `最近 ${loginHistory.value.length} 次记录`,
    icon: 'chart-line',
    tone: loginFailedCount.value > 0 ? 'orange' : 'green',
  },
  {
    label: '密码策略',
    value: mustChangePassword.value ? '待更新' : '正常',
    hint: '大小写字母 + 数字',
    icon: 'lock-on',
    tone: mustChangePassword.value ? 'orange' : 'blue',
  },
]);

// 加载用户信息
const loadUserInfo = async () => {
  try {
    const res = await getCurrentUser();
    userInfo.value = res;
    infoForm.value = {
      username: res.username,
      nickname: res.nickname || '',
      email: res.email || '',
      phone: res.phone || '',
      avatar: res.avatar || '',
    };
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载用户信息失败');
  }
};

// 加载登录历史
const loadLoginHistory = async () => {
  historyLoading.value = true;
  try {
    const res = await getMyLoginLogs({ page: 1, page_size: 10 });
    loginHistory.value = res.list || [];
  } catch (error: any) {
    console.error('加载登录历史失败:', error);
  } finally {
    historyLoading.value = false;
  }
};

// 更新个人信息
const handleUpdateInfo = async () => {
  const valid = await infoFormRef.value?.validate();
  if (!valid) return;

  infoLoading.value = true;
  try {
    const res = await updateProfile({
      nickname: infoForm.value.nickname,
      email: infoForm.value.email,
      phone: infoForm.value.phone,
      avatar: infoForm.value.avatar,
    });
    userInfo.value = res;
    infoForm.value = {
      username: res.username,
      nickname: res.nickname || '',
      email: res.email || '',
      phone: res.phone || '',
      avatar: res.avatar || '',
    };
    MessagePlugin.success('个人信息更新成功');
    await userStore.getUserInfo();
  } catch (error: any) {
    MessagePlugin.error(error.message || '更新失败');
  } finally {
    infoLoading.value = false;
  }
};

// 修改密码
const handleChangePassword = async () => {
  const valid = await pwdFormRef.value?.validate();
  if (!valid) return;

  pwdLoading.value = true;
  try {
    await changePassword({
      old_password: pwdForm.value.old_password,
      new_password: pwdForm.value.new_password,
    });
    MessagePlugin.success('密码修改成功');
    await userStore.getUserInfo();
    await loadUserInfo();
    if (route.query.force_change_password) {
      const query = { ...route.query };
      delete query.force_change_password;
      await router.replace({ path: route.path, query });
    }
    pwdForm.value = {
      old_password: '',
      new_password: '',
      confirm_password: '',
    };
  } catch (error: any) {
    MessagePlugin.error(error.message || '修改密码失败');
  } finally {
    pwdLoading.value = false;
  }
};

const validTotpCode = () => {
  if (!/^\d{6}$/.test(totpCode.value.trim())) {
    MessagePlugin.warning('请输入 6 位数字验证码');
    return false;
  }
  return true;
};

const validTotpCurrentPassword = () => {
  if (!totpCurrentPassword.value) {
    MessagePlugin.warning('请输入当前密码');
    return false;
  }
  return true;
};

const applyTotpRecoveryCodes = (res: TOTPRecoveryCodesResponse) => {
  totpRecoveryCodes.value = res.recovery_codes || [];
};

const handleGenerateTotp = async () => {
  if (!validTotpCurrentPassword()) return;
  totpLoading.value = true;
  try {
    totpSetup.value = await generateTotpSetup({ current_password: totpCurrentPassword.value });
    totpCode.value = '';
    totpRecoveryCodes.value = [];
  } catch (error: any) {
    MessagePlugin.error(error.message || '生成两步验证配置失败');
  } finally {
    totpLoading.value = false;
  }
};

const handleEnableTotp = async () => {
  if (!validTotpCode()) return;
  if (!validTotpCurrentPassword()) return;
  totpLoading.value = true;
  try {
    const res = await enableTotp({ code: totpCode.value.trim(), current_password: totpCurrentPassword.value });
    applyTotpRecoveryCodes(res);
    MessagePlugin.success('两步验证已开启');
    totpSetup.value = null;
    totpCode.value = '';
    totpCurrentPassword.value = '';
    await loadUserInfo();
    await userStore.getUserInfo();
  } catch (error: any) {
    MessagePlugin.error(error.message || '开启两步验证失败');
  } finally {
    totpLoading.value = false;
  }
};

const handleRegenerateRecoveryCodes = async () => {
  if (!validTotpCode()) return;
  if (!validTotpCurrentPassword()) return;
  totpLoading.value = true;
  try {
    const res = await regenerateTotpRecoveryCodes({ code: totpCode.value.trim(), current_password: totpCurrentPassword.value });
    applyTotpRecoveryCodes(res);
    MessagePlugin.success('恢复码已重新生成');
    totpCode.value = '';
    totpCurrentPassword.value = '';
  } catch (error: any) {
    MessagePlugin.error(error.message || '重新生成恢复码失败');
  } finally {
    totpLoading.value = false;
  }
};

const handleDisableTotp = async () => {
  if (!validTotpCode()) return;
  if (!validTotpCurrentPassword()) return;
  totpLoading.value = true;
  try {
    await disableTotp({ code: totpCode.value.trim(), current_password: totpCurrentPassword.value });
    MessagePlugin.success('两步验证已关闭');
    totpCode.value = '';
    totpCurrentPassword.value = '';
    totpSetup.value = null;
    totpRecoveryCodes.value = [];
    await loadUserInfo();
    await userStore.getUserInfo();
  } catch (error: any) {
    MessagePlugin.error(error.message || '关闭两步验证失败');
  } finally {
    totpLoading.value = false;
  }
};

onMounted(() => {
  loadUserInfo();
  loadLoginHistory();
});
</script>

<style lang="less" scoped>
.profile-page {
  --profile-bg: #f5f7fb;
  --profile-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --profile-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --profile-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--profile-bg);
  color: var(--td-text-color-primary);
  font-family: var(--profile-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.profile-page :deep(.t-card),
.profile-page :deep(.t-table),
.profile-page :deep(.t-form),
.profile-page :deep(.t-button),
.profile-page :deep(.t-tag),
.profile-page :deep(.t-tabs),
.profile-page :deep(.t-input),
.profile-page :deep(.t-empty) {
  font-family: var(--profile-font);
}

.profile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-l);
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: linear-gradient(120deg, rgb(255 255 255 / 96%) 0%, #f3f7ff 58%, #fff7ed 100%);
  box-shadow: 0 8px 20px rgb(15 23 42 / 4%);
  backdrop-filter: blur(8px);
}

.profile-head__main {
  min-width: 0;
}

.profile-head__title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.profile-head__title h2 {
  margin: 0;
  color: #101828;
  font-size: 21px;
  font-weight: 800;
  line-height: 28px;
}

.profile-head__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  margin-top: 4px;
  color: #667085;
  font-size: 12px;
  line-height: 20px;
}

.profile-head__actions {
  flex-shrink: 0;
  justify-content: flex-end;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.summary-panel {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 104px;
  overflow: hidden;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--td-comp-margin-m);
  padding: 16px;
  border-radius: 14px;
  background: var(--summary-bg);
  box-shadow: var(--profile-card-shadow);
}

.summary-panel::after {
  position: absolute;
  right: -26px;
  bottom: -32px;
  width: 84px;
  height: 84px;
  border-radius: 50%;
  background: rgb(255 255 255 / 42%);
  content: '';
}

.summary-panel--blue {
  --summary-bg: linear-gradient(135deg, #d9e9ff 0%, #bdd7ff 100%);
  --summary-accent: #0052d9;
}

.summary-panel--green {
  --summary-bg: linear-gradient(135deg, #d9f8e6 0%, #bdf0d0 100%);
  --summary-accent: #008858;
}

.summary-panel--orange {
  --summary-bg: linear-gradient(135deg, #ffe8cf 0%, #ffd2a8 100%);
  --summary-accent: #b24b00;
}

.summary-panel--red {
  --summary-bg: linear-gradient(135deg, #ffe0e5 0%, #ffc2cc 100%);
  --summary-accent: #b8272d;
}

.summary-panel__main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
  z-index: 1;
}

.summary-panel__main span {
  color: #475467;
  font-size: 12px;
  font-weight: 750;
}

.summary-panel__main strong {
  overflow: hidden;
  color: #101828;
  font-family: var(--profile-number-font);
  font-size: 28px;
  font-weight: 800;
  line-height: 34px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-panel__main small {
  overflow: hidden;
  color: #667085;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-panel__icon {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgb(255 255 255 / 42%);
  color: var(--summary-accent);
  font-size: 18px;
  z-index: 1;
}

.profile-grid {
  display: grid;
  min-width: 0;
  align-items: start;
  gap: 14px;
  grid-template-columns: 320px minmax(0, 1fr);
}

.profile-side,
.profile-main {
  min-width: 0;
}

.profile-card,
.workspace-card {
  min-width: 0;
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 14px;
  background: rgb(255 255 255 / 94%);
  box-shadow: var(--profile-card-shadow);
  backdrop-filter: blur(10px);
}

.profile-card {
  padding: 18px;
}

.avatar-section {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 12px 0 18px;
  text-align: center;
}

.profile-avatar {
  border: 4px solid #fff;
  background: linear-gradient(135deg, #2f6bff 0%, #22c55e 100%);
  box-shadow: 0 12px 24px rgb(47 107 255 / 18%);
  color: #fff;
  font-size: 26px;
  font-weight: 800;
}

.profile-name {
  display: flex;
  max-width: 100%;
  flex-direction: column;
  gap: 3px;
}

.profile-name strong {
  overflow: hidden;
  color: #101828;
  font-size: 18px;
  font-weight: 800;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-name span {
  overflow: hidden;
  color: #667085;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 0;
  border-top: 1px solid #edf1f7;
  border-bottom: 1px solid #edf1f7;
}

.info-item {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  background: #f8fafc;
}

.info-item__icon {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #eef6ff;
  color: #0052d9;
  font-size: 16px;
}

.info-item div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.info-item small {
  color: #667085;
  font-size: 12px;
  line-height: 18px;
}

.info-item strong {
  overflow: hidden;
  color: #101828;
  font-size: 13px;
  font-weight: 700;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.role-block {
  padding-top: 14px;
}

.role-block__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-s);
  color: #475467;
  font-size: 12px;
  font-weight: 750;
}

.role-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.workspace-card {
  padding: 0 18px 18px;
}

.profile-tabs :deep(.t-tabs__nav) {
  min-height: 54px;
}

.profile-tabs :deep(.t-tabs__bar) {
  height: 3px;
  border-radius: 2px;
}

.profile-tabs :deep(.t-tab__label) {
  font-weight: 700;
}

.profile-tabs :deep(.t-tabs__content) {
  padding-top: 0;
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--td-comp-margin-m);
  padding: 16px 0 14px;
  border-bottom: 1px solid #edf1f7;
}

.panel-head h3 {
  margin: 0;
  color: #101828;
  font-size: 17px;
  font-weight: 800;
  line-height: 24px;
}

.panel-head p {
  margin: 3px 0 0;
  color: #667085;
  font-size: 12px;
  line-height: 20px;
}

.profile-form {
  padding-top: 18px;
}

.profile-form :deep(.t-form__label) {
  color: #667085;
  font-size: 12px;
  font-weight: 650;
}

.profile-form :deep(.t-input) {
  border-radius: 8px;
}

.form-grid {
  display: grid;
  gap: 2px 18px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.form-grid__full {
  grid-column: 1 / -1;
}

.password-form {
  max-width: 560px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 6px;
  padding-top: 12px;
  border-top: 1px solid #edf1f7;
}

.force-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 14px 0 0;
  padding: 12px 14px;
  border: 1px solid #ffe1bd;
  border-radius: 10px;
  background: #fff7ed;
  color: #b24b00;
  font-size: 13px;
  line-height: 20px;
}

.totp-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  max-width: 640px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid #edf1f7;
}

.totp-panel__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.totp-panel__head h4 {
  margin: 0;
  color: #101828;
  font-size: 15px;
  font-weight: 800;
  line-height: 22px;
}

.totp-panel__head p {
  margin: 2px 0 0;
  color: #667085;
  font-size: 12px;
  line-height: 20px;
}

.totp-setup {
  display: grid;
  align-items: start;
  gap: 16px;
  grid-template-columns: 144px minmax(0, 1fr);
}

.totp-setup__main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 10px;
}

.totp-setup__verify,
.totp-bind,
.totp-disable {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.totp-setup__verify :deep(.t-input),
.totp-bind :deep(.t-input),
.totp-disable :deep(.t-input) {
  max-width: 220px;
}

.totp-recovery {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid #ffe1bd;
  border-radius: 8px;
  background: #fffaf3;
}

.totp-recovery__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.totp-recovery__head h5 {
  margin: 0;
  color: #101828;
  font-size: 13px;
  font-weight: 800;
  line-height: 20px;
}

.totp-recovery__grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.totp-recovery__code {
  min-width: 0;
  padding: 7px 8px;
  border: 1px solid #fed7aa;
  border-radius: 6px;
  background: #fff;
  color: #9a3412;
  font-family: var(--profile-number-font);
  font-size: 13px;
  font-weight: 750;
  letter-spacing: 0;
  text-align: center;
}

.login-table {
  margin-top: 14px;
}

.login-table :deep(.t-table__header th) {
  height: 46px;
  background: #f8fafc;
  color: #475467;
  font-size: 12px;
  font-weight: 750;
}

.login-table :deep(.t-table__body td) {
  padding: 13px 16px;
  color: #1d2939;
  font-size: 13px;
  line-height: 20px;
  vertical-align: top;
}

.login-table :deep(.t-table__body tr:hover td) {
  background: #f7fbff;
}

.mono-text {
  overflow: hidden;
  color: #475467;
  font-family: var(--profile-number-font);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-text {
  display: -webkit-box;
  overflow: hidden;
  color: #667085;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

@media (width <= 1180px) {
  .profile-grid {
    grid-template-columns: 1fr;
  }

  .profile-side {
    order: 2;
  }
}

@media (width <= 960px) {
  .summary-grid,
  .form-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .profile-head,
  .panel-head {
    align-items: stretch;
    flex-direction: column;
  }
}

@media (width <= 640px) {
  .profile-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .summary-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .workspace-card,
  .profile-card {
    padding-right: 12px;
    padding-left: 12px;
  }

  .form-actions {
    justify-content: stretch;
  }

  .form-actions :deep(.t-button) {
    width: 100%;
  }

  .totp-setup {
    grid-template-columns: 1fr;
  }

  .totp-setup__verify,
  .totp-bind,
  .totp-disable {
    flex-direction: column;
  }

  .totp-setup__verify :deep(.t-input),
  .totp-bind :deep(.t-input),
  .totp-disable :deep(.t-input) {
    max-width: none;
  }

  .totp-recovery__grid {
    grid-template-columns: 1fr;
  }
}
</style>
