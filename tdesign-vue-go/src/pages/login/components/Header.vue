<template>
  <header class="login-header">
    <div class="login-brand" aria-label="后台管理系统">
      <span class="login-brand__text">后台管理系统</span>
    </div>
    <div class="operations-container">
      <t-tag theme="success" variant="light">运营后台</t-tag>
      <t-button theme="default" shape="square" variant="text" @click="toggleSettingPanel">
        <t-icon name="setting" class="icon" />
      </t-button>
    </div>
  </header>
</template>
<script setup lang="ts">
import { useSettingStore } from '@/store';

const settingStore = useSettingStore();
const toggleSettingPanel = () => {
  settingStore.updateConfig({
    showSettingPanel: true,
  });
};

</script>
<style lang="less" scoped>
.login-header {
  display: flex;
  position: relative;
  z-index: 2;
  height: 64px;
  align-items: center;
  justify-content: space-between;
  padding: 0 64px;
  background: transparent;
  color: var(--td-text-color-primary);
}

.login-brand {
  display: inline-flex;
  min-width: 0;
  height: 44px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.login-brand__text {
  color: #1f2937;
  font-size: 20px;
  font-weight: 800;
  font-style: italic;
  line-height: 28px;
}

.operations-container {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (width <= 768px) {
  .login-header {
    height: 64px;
    padding: 0 24px;
  }

  .login-brand {
    height: 40px;
  }

  .login-brand__text {
    font-size: 18px;
  }
}
</style>
