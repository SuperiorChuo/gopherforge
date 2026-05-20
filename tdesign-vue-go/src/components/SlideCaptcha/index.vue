<template>
  <t-dialog
    v-model:visible="visible"
    header="请输入验证码"
    width="340px"
    :footer="false"
    :close-on-overlay-click="true"
    @close="handleClose"
  >
    <div class="text-captcha">
      <div v-if="loading" class="captcha-loading">
        <t-loading text="加载中..." />
      </div>

      <template v-else>
        <div class="captcha-panel">
          <strong v-if="captchaText" class="captcha-text">{{ captchaText }}</strong>
          <img v-else-if="captchaImage" :src="captchaImage" class="captcha-image" alt="验证码" />
          <t-button variant="text" :disabled="verifying" @click="refresh">换一张</t-button>
        </div>

        <t-input
          v-model="captchaCode"
          size="large"
          clearable
          autofocus
          maxlength="6"
          placeholder="输入图中验证码"
          @enter="verify"
        />

        <t-button block theme="primary" :loading="verifying" :disabled="!captchaCode.trim()" @click="verify">
          确认
        </t-button>
      </template>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { onMounted, ref, watch } from 'vue';

import { request } from '@/utils/request';

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(['update:show', 'success', 'close']);

const visible = ref(false);
const loading = ref(false);
const verifying = ref(false);
const captchaKey = ref('');
const captchaImage = ref('');
const captchaText = ref('');
const captchaCode = ref('');

watch(
  () => props.show,
  (val) => {
    visible.value = val;
    if (val) {
      refresh();
    }
  }
);

watch(visible, (val) => {
  emit('update:show', val);
  if (!val) {
    emit('close');
  }
});

const getCaptcha = async () => {
  loading.value = true;
  try {
    const res = await request.get<any>({ url: '/captcha' }, { withToken: false });
    captchaKey.value = res.key;
    captchaImage.value = toDataUrl(res.image, 'image/png');
    captchaText.value = res.code_hint || '';
    captchaCode.value = '';
  } catch {
    MessagePlugin.error('获取验证码失败');
  } finally {
    loading.value = false;
  }
};

const toDataUrl = (value: string, mimeType: string) => {
  if (!value) return '';
  return value.startsWith('data:') ? value : `data:${mimeType};base64,${value}`;
};

const refresh = () => {
  getCaptcha();
};

const verify = async () => {
  if (!captchaCode.value.trim() || verifying.value) return;
  verifying.value = true;
  try {
    emit('success', {
      captcha_id: captchaKey.value,
      captcha_code: captchaCode.value.trim().toUpperCase(),
    });
    visible.value = false;
  } finally {
    verifying.value = false;
  }
};

const handleClose = () => {
  visible.value = false;
};

onMounted(() => {
  if (props.show) {
    getCaptcha();
  }
});
</script>

<style lang="less" scoped>
.text-captcha {
  display: grid;
  gap: 14px;
  padding: 8px 0 2px;
}

.captcha-loading {
  min-height: 130px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.captcha-panel {
  width: 100%;
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  padding: 10px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.captcha-image {
  width: 120px;
  height: 42px;
  image-rendering: auto;
}

.captcha-text {
  min-width: 120px;
  height: 42px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  background: #f5f8ff;
  color: #1f4596;
  font-size: 24px;
  font-family: Consolas, 'Courier New', monospace;
  letter-spacing: 6px;
}

.refresh-text {
  color: var(--td-brand-color);
  font-size: 13px;
}
</style>
