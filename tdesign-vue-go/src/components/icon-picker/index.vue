<template>
  <t-select
    v-model="selectedIcon"
    :options="iconOptions"
    placeholder="请选择图标"
    filterable
    clearable
    :filter="filterIcon"
    :popup-props="{ overlayClassName: 'icon-picker-popup' }"
    @change="handleChange"
  >
    <template #prefixIcon>
      <t-icon v-if="selectedIcon" :name="selectedIcon" />
    </template>
    <template #option="{ option }">
      <div class="icon-option">
        <t-icon :name="option.value" size="18px" />
        <span class="icon-option-label">{{ option.label }}</span>
      </div>
    </template>
  </t-select>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  modelValue?: string;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
}>();

// 常用 TDesign 图标列表
const iconList = [
  // 导航类
  'home', 'dashboard', 'menu-application', 'menu-fold', 'menu-unfold',
  // 用户类
  'user', 'user-circle', 'usergroup', 'user-add', 'user-blocked', 'user-checked',
  // 系统类
  'setting', 'tools', 'system-setting', 'control-platform', 'server',
  // 安全类
  'lock-on', 'lock-off', 'secured', 'root-list',
  // 文件类
  'folder', 'folder-open', 'file', 'file-add', 'file-copy', 'file-excel',
  // 数据类
  'chart-pie', 'chart-bar', 'chart-line', 'chart-radial', 'chart-bubble',
  // 通用类
  'add', 'delete', 'edit', 'browse', 'search', 'refresh', 'download', 'upload',
  'print', 'save', 'close', 'check', 'check-circle', 'clear', 'help-circle',
  // 通知类
  'notification', 'mail', 'chat', 'chat-bubble', 'sound', 'sound-mute',
  // 时间类
  'time', 'calendar', 'history',
  // 链接类
  'link', 'link-unlink', 'attach', 'share',
  // 显示类
  'view-list', 'view-module', 'view-column', 'layout',
  // 箭头类
  'arrow-up', 'arrow-down', 'arrow-left', 'arrow-right',
  'chevron-up', 'chevron-down', 'chevron-left', 'chevron-right',
  // 其他
  'star', 'star-filled', 'heart', 'heart-filled', 'flag', 'bookmark',
  'pin', 'location', 'map', 'cart', 'shop', 'gift', 'money-circle',
  'wallet', 'creditcard', 'discount', 'coupon', 'service', 'call',
  'mobile', 'desktop', 'laptop', 'cloud', 'cloud-upload', 'cloud-download',
  'backup', 'rollback', 'swap', 'filter', 'sort-ascending', 'sort-descending',
  'zoom-in', 'zoom-out', 'fullscreen', 'fullscreen-exit', 'play-circle',
  'pause-circle', 'stop-circle', 'image', 'image-add', 'video', 'audio',
  'code', 'terminal', 'bug', 'bug-report', 'app', 'component', 'layers',
  'adjustment', 'precise-monitor', 'data-base', 'storage', 'task',
];

const iconOptions = iconList.map(icon => ({
  label: icon,
  value: icon,
}));

const selectedIcon = computed({
  get: () => props.modelValue || '',
  set: (val) => emit('update:modelValue', val || ''),
});

const filterIcon = (keyword: string, option: any) => {
  return String(option?.label || '').toLowerCase().includes(keyword.toLowerCase());
};

const handleChange = (val: any) => {
  emit('update:modelValue', String(val || ''));
};
</script>

<style lang="less" scoped>
.icon-option {
  display: flex;
  align-items: center;
  gap: 8px;

  .icon-option-label {
    color: var(--td-text-color-primary);
  }
}
</style>

<style lang="less">
.icon-picker-popup {
  .t-select-option {
    padding: 8px 12px;
  }
}
</style>
