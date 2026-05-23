<template>
  <t-form
    ref="form"
    class="item-container"
    :class="[`login-${type}`]"
    :data="formData"
    :rules="FORM_RULES"
    label-width="0"
    @submit="onSubmit"
  >
    <template v-if="type === 'password'">
      <t-form-item name="account">
        <t-input v-model="formData.account" size="large" :placeholder="t('pages.login.input.account')">
          <template #prefix-icon>
            <t-icon name="user" />
          </template>
        </t-input>
      </t-form-item>

      <t-form-item name="password">
        <t-input
          v-model="formData.password"
          size="large"
          :type="showPsw ? 'text' : 'password'"
          clearable
          :placeholder="t('pages.login.input.password')"
        >
          <template #prefix-icon>
            <t-icon name="lock-on" />
          </template>
          <template #suffix-icon>
            <t-icon :name="showPsw ? 'browse' : 'browse-off'" @click="showPsw = !showPsw" />
          </template>
        </t-input>
      </t-form-item>

      <div class="check-container remember-pwd">
        <t-checkbox>{{ t('pages.login.remember') }}</t-checkbox>
        <span class="tip">{{ t('pages.login.forget') }}</span>
      </div>
    </template>

    <!-- 扫码登录 -->
    <template v-else-if="type === 'qrcode'">
      <div class="tip-container">
        <span class="tip">{{ t('pages.login.wechatLogin') }}</span>
        <span class="refresh">{{ t('pages.login.refresh') }} <t-icon name="refresh" /> </span>
      </div>
      <qrcode-vue value="" :size="160" level="H" />
    </template>

    <!-- 手机号登录 -->
    <template v-else>
      <t-form-item name="phone">
        <t-input v-model="formData.phone" size="large" :placeholder="t('pages.login.input.phone')">
          <template #prefix-icon>
            <t-icon name="mobile" />
          </template>
        </t-input>
      </t-form-item>

      <t-form-item class="verification-code" name="verifyCode">
        <t-input v-model="formData.verifyCode" size="large" :placeholder="t('pages.login.input.verification')" />
        <t-button size="large" variant="outline" :disabled="countDown > 0" @click="sendCode">
          {{ countDown === 0 ? t('pages.login.sendVerification') : `${countDown}秒后可重发` }}
        </t-button>
      </t-form-item>
    </template>

    <t-form-item v-if="type !== 'qrcode'" class="btn-container">
      <t-button block size="large" type="submit"> {{ t('pages.login.signIn') }} </t-button>
    </t-form-item>

    <div class="switch-container">
      <span v-if="type !== 'password'" class="tip" @click="switchType('password')">{{
        t('pages.login.accountLogin')
      }}</span>
      <span v-if="type !== 'qrcode'" class="tip" @click="switchType('qrcode')">{{ t('pages.login.wechatLogin') }}</span>
      <span v-if="type !== 'phone'" class="tip" @click="switchType('phone')">{{ t('pages.login.phoneLogin') }}</span>
    </div>
  </t-form>

  <slide-captcha v-model:show="showSlideCaptcha" @success="handleSlideSuccess" />
  <totp-verify-dialog
    v-model:visible="showTotpDialog"
    :loading="totpLoading"
    @submit="handleTotpSubmit"
    @cancel="handleTotpCancel"
  />
</template>
<script setup lang="ts">
import QrcodeVue from 'qrcode.vue';
import type { FormInstanceFunctions, FormRule, SubmitContext } from 'tdesign-vue-next';
import { MessagePlugin } from 'tdesign-vue-next';
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { useCounter } from '@/hooks';
import { t } from '@/locales';
import { useUserStore } from '@/store';
import { verifyTotpLogin } from '@/api/auth';
import SlideCaptcha from '@/components/SlideCaptcha/index.vue';
import TotpVerifyDialog from './TotpVerifyDialog.vue';
import { normalizeRedirectUrl } from '../utils';

const userStore = useUserStore();

const INITIAL_DATA = {
  phone: '',
  account: '',
  password: '',
  verifyCode: '',
  checked: false,
};

const FORM_RULES: Record<string, FormRule[]> = {
  phone: [{ required: true, message: t('pages.login.required.phone'), type: 'error' }],
  account: [{ required: true, message: t('pages.login.required.account'), type: 'error' }],
  password: [{ required: true, message: t('pages.login.required.password'), type: 'error' }],
  verifyCode: [{ required: true, message: t('pages.login.required.verification'), type: 'error' }],
};

const type = ref('password');

const form = ref<FormInstanceFunctions>();
const formData = ref({ ...INITIAL_DATA });
const showPsw = ref(false);
const showSlideCaptcha = ref(false);
const showTotpDialog = ref(false);
const totpLoading = ref(false);
const pendingTotpChallengeId = ref('');

const [countDown, handleCounter] = useCounter();

const switchType = (val: string) => {
  type.value = val;
};

const router = useRouter();
const route = useRoute();

/**
 * 发送验证码
 */
const sendCode = () => {
  form.value?.validate({ fields: ['phone'] }).then((e) => {
    if (e === true) {
      handleCounter();
    }
  });
};

const onSubmit = async (ctx: SubmitContext) => {
  if (ctx.validateResult === true) {
    if (type.value === 'password') {
      showSlideCaptcha.value = true;
    } else {
      MessagePlugin.warning('当前环境仅启用账号密码登录');
    }
  }
};

const handleSlideSuccess = async ({ captcha_id, captcha_code }: { captcha_id: string; captcha_code: string }) => {
  try {
    const loginResp = await userStore.login({
      username: formData.value.account,
      password: formData.value.password,
      captcha_id: captcha_id,
      captcha_code: captcha_code,
    });
    if (loginResp.requires_totp && loginResp.totp_challenge_id) {
      pendingTotpChallengeId.value = loginResp.totp_challenge_id;
      showTotpDialog.value = true;
      return;
    }

    await finishLogin();
  } catch (error: any) {
    MessagePlugin.error(error?.message || '登录失败，请检查用户名和密码');
  }
};

const finishLogin = async () => {
    await userStore.getUserInfo();

    MessagePlugin.success('登录成功');
    const redirect = route.query.redirect as string;
    const redirectUrl = userStore.userInfo.mustChangePassword
      ? '/profile/index?force_change_password=1'
      : redirect
        ? normalizeRedirectUrl(redirect)
        : '/dashboard/index';
    router.push(redirectUrl);
};

const handleTotpSubmit = async (code: string) => {
  if (!pendingTotpChallengeId.value) return;
  totpLoading.value = true;
  try {
    const loginResp = await verifyTotpLogin({
      challenge_id: pendingTotpChallengeId.value,
      code,
    });
    userStore.applyLoginSession(loginResp);
    showTotpDialog.value = false;
    pendingTotpChallengeId.value = '';
    await finishLogin();
  } catch (error: any) {
    MessagePlugin.error(error?.message || '两步验证码校验失败');
  } finally {
    totpLoading.value = false;
  }
};

const handleTotpCancel = () => {
  pendingTotpChallengeId.value = '';
};

onMounted(() => {
  userStore.logout();
});
</script>
<style lang="less" scoped>
@import '../index.less';
</style>
