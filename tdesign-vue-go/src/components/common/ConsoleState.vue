<template>
  <section class="console-state" :class="stateClasses" role="status">
    <span class="console-state__icon" aria-hidden="true">
      <t-loading v-if="type === 'loading'" size="small" />
      <t-icon v-else :name="stateIcon" />
    </span>

    <div class="console-state__content">
      <strong>{{ displayTitle }}</strong>
      <p v-if="displayDescription">{{ displayDescription }}</p>
      <slot />
    </div>

    <div v-if="$slots.actions || actionText" class="console-state__actions">
      <slot name="actions">
        <t-button size="small" :theme="actionTheme" variant="outline" @click="emit('action')">
          {{ actionText }}
        </t-button>
      </slot>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';

type StateSize = 'compact' | 'normal' | 'large';
type StateType = 'empty' | 'error' | 'info' | 'loading' | 'permission' | 'success' | 'warning';
type ButtonTheme = 'default' | 'primary' | 'success' | 'warning' | 'danger';

const props = withDefaults(
  defineProps<{
    actionText?: string;
    actionTheme?: ButtonTheme;
    bordered?: boolean;
    description?: string;
    size?: StateSize;
    title?: string;
    type?: StateType;
  }>(),
  {
    actionText: '',
    actionTheme: 'primary',
    bordered: false,
    description: '',
    size: 'normal',
    title: '',
    type: 'empty',
  },
);

const emit = defineEmits<{
  action: [];
}>();

const stateMeta: Record<StateType, { icon: string; title: string }> = {
  empty: { icon: 'file-unknown', title: '暂无数据' },
  error: { icon: 'error-circle', title: '加载失败' },
  info: { icon: 'info-circle', title: '提示信息' },
  loading: { icon: 'loading', title: '正在加载' },
  permission: { icon: 'lock-on', title: '暂无权限' },
  success: { icon: 'check-circle', title: '状态正常' },
  warning: { icon: 'error-circle', title: '需要关注' },
};

const displayTitle = computed(() => props.title || stateMeta[props.type].title);
const displayDescription = computed(() => props.description.trim());
const stateIcon = computed(() => stateMeta[props.type].icon);

const stateClasses = computed(() => [
  `console-state--${props.type}`,
  `console-state--${props.size}`,
  {
    'console-state--bordered': props.bordered,
  },
]);
</script>

<style scoped lang="less">
.console-state {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 18px;
  color: #0f172a;
  text-align: left;
}

.console-state--compact {
  min-height: 92px;
  padding: 14px;
}

.console-state--normal {
  min-height: 132px;
}

.console-state--large {
  min-height: 200px;
  padding: 28px;
}

.console-state--bordered {
  border: 1px solid #e6edf7;
  border-radius: 12px;
  background:
    linear-gradient(135deg, rgb(255 255 255 / 82%), rgb(255 255 255 / 54%)),
    #fbfdff;
}

.console-state__icon {
  display: inline-flex;
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: #eef4ff;
  color: #2563eb;
  font-size: 20px;
  box-shadow: inset 0 0 0 1px rgb(37 99 235 / 8%);
}

.console-state__content {
  min-width: 0;
  max-width: 520px;
}

.console-state__content strong {
  display: block;
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
  line-height: 22px;
}

.console-state__content p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 12px;
  line-height: 18px;
  overflow-wrap: anywhere;
}

.console-state__actions {
  display: flex;
  flex-shrink: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.console-state--empty .console-state__icon,
.console-state--info .console-state__icon {
  background: #eef4ff;
  color: #2563eb;
}

.console-state--loading .console-state__icon {
  background: #f8fafc;
  color: #475569;
}

.console-state--success .console-state__icon {
  background: #dcfce7;
  color: #059669;
}

.console-state--warning .console-state__icon {
  background: #ffedd5;
  color: #d97706;
}

.console-state--error .console-state__icon {
  background: #fee2e2;
  color: #dc2626;
}

.console-state--permission .console-state__icon {
  background: #f1f5f9;
  color: #475569;
}

@media (width <= 640px) {
  .console-state {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
