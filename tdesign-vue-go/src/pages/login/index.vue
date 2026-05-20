<template>
  <div class="login-wrapper">
    <login-header />

    <main class="login-main">
      <section class="login-panel" aria-labelledby="login-title">
        <div class="title-container">
          <p class="title-container__eyebrow">{{ type === 'register' ? '创建账号' : t('pages.login.loginTitle') }}</p>
          <h1 id="login-title">后台管理系统</h1>
          <div class="sub-title">
            <span>{{ type === 'register' ? t('pages.login.existAccount') : t('pages.login.noAccount') }}</span>
            <t-link theme="primary" hover="color" @click="switchType(type === 'register' ? 'login' : 'register')">
              {{ type === 'register' ? '返回登录' : t('pages.login.createAccount') }}
            </t-link>
          </div>
        </div>

        <login v-if="type === 'login'" />
        <register v-else @register-success="switchType('login')" />
      </section>
    </main>

    <tdesign-setting />

    <footer class="copyright">© 2021-2026 后台管理系统 版权所有</footer>
  </div>
</template>
<script setup lang="ts">
import { ref } from 'vue';

import TdesignSetting from '@/layouts/setting.vue';
import { t } from '@/locales';

import LoginHeader from './components/Header.vue';
import Login from './components/Login.vue';
import Register from './components/Register.vue';

defineOptions({
  name: 'LoginIndex',
});
const type = ref('login');
const switchType = (val: string) => {
  type.value = val;
};
</script>
<style lang="less" scoped>
@import './index.less';
</style>
