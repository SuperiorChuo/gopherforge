<template>
  <t-dialog
    v-model:visible="visible"
    header="两步验证"
    width="420px"
    :close-on-overlay-click="false"
    :close-btn="false"
    :footer="false"
  >
    <div class="totp-dialog" role="dialog" aria-label="两步验证" aria-modal="true">
      <t-input v-model="code" size="large" maxlength="20" placeholder="请输入验证码或恢复码" @keyup.enter="submit">
        <template #prefix-icon>
          <t-icon name="secured" />
        </template>
      </t-input>
      <div class="totp-dialog__actions">
        <t-button variant="outline" :disabled="loading" @click="cancel">取消</t-button>
        <t-button theme="primary" :loading="loading" @click="submit">验证</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { ref, watch } from 'vue';

defineProps<{
  loading?: boolean;
}>();

const emit = defineEmits<{
  submit: [code: string];
  cancel: [];
}>();

const visible = defineModel<boolean>('visible', { default: false });

const code = ref('');

watch(visible, (value) => {
  if (value) {
    code.value = '';
  }
});

const submit = () => {
  const value = code.value.trim();
  const normalizedRecoveryCode = value.replace(/[\s-]/g, '').toUpperCase();
  if (!/^\d{6}$/.test(value) && !/^[A-Z2-7]{15}$/.test(normalizedRecoveryCode)) {
    MessagePlugin.warning('请输入 6 位验证码或恢复码');
    return;
  }
  emit('submit', value);
};

const cancel = () => {
  emit('cancel');
  visible.value = false;
};
</script>

<style lang="less" scoped>
.totp-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.totp-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
