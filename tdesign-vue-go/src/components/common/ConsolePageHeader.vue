<template>
  <section class="console-page-header">
    <div class="console-page-header__main">
      <div class="console-page-header__title">
        <slot name="title">
          <h2>{{ title }}</h2>
          <t-tag v-if="statusText" :theme="statusTheme" variant="light">
            {{ statusText }}
          </t-tag>
        </slot>
      </div>
      <div v-if="$slots.meta || visibleMeta.length" class="console-page-header__meta">
        <slot name="meta">
          <span v-for="item in visibleMeta" :key="item">{{ item }}</span>
        </slot>
      </div>
    </div>
    <div v-if="$slots.actions" class="console-page-header__actions">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';

type TagTheme = 'default' | 'primary' | 'success' | 'warning' | 'danger';

const props = withDefaults(
  defineProps<{
    title?: string;
    statusText?: string;
    statusTheme?: TagTheme;
    meta?: Array<number | string | null | undefined>;
  }>(),
  {
    title: '',
    statusText: '',
    statusTheme: 'primary',
    meta: () => [],
  },
);

const visibleMeta = computed(() => props.meta.map((item) => String(item ?? '').trim()).filter(Boolean));
</script>

<style scoped lang="less">
.console-page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-l);
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background:
    radial-gradient(circle at 18% 0%, rgb(37 99 235 / 10%), transparent 28%),
    radial-gradient(circle at 92% 16%, rgb(20 184 166 / 12%), transparent 26%),
    #fff;
  box-shadow: 0 10px 24px rgb(15 23 42 / 5%);
}

.console-page-header__main {
  min-width: 0;
}

.console-page-header__title {
  display: flex;
  align-items: center;
  gap: 10px;

  h2 {
    margin: 0;
    color: #0f172a;
    font-size: 22px;
    font-weight: 800;
    line-height: 30px;
  }
}

.console-page-header__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-top: 6px;
  color: #52627a;
  font-size: 12px;

  span + span::before {
    margin-right: 10px;
    color: #c7d0df;
    content: '/';
  }
}

.console-page-header__actions {
  display: flex;
  flex-shrink: 0;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.console-page-header__actions :deep(.t-tag) {
  height: 32px;
  align-items: center;
}

.console-page-header__actions :deep(.t-button) {
  min-height: 32px;
}

.console-page-header__actions :deep(.console-page-header__select) {
  width: 118px;
}

.console-page-header__actions :deep(.console-page-header__auto-wrap) {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  background: #fff;
}

.console-page-header__actions :deep(.console-page-header__auto-wrap--active) {
  border-color: #bfdbfe;
  background: #eff6ff;
}

.console-page-header__actions :deep(.console-page-header__auto-label) {
  color: #334155;
  font-size: 12px;
  font-weight: 600;
}

.console-page-header__actions :deep(.console-page-header__refresh) {
  width: 32px;
  height: 32px;
}

@media (width <= 1200px) {
  .console-page-header__actions {
    width: 100%;
  }

  .console-page-header__actions :deep(.console-page-header__select) {
    width: 140px;
  }
}

@media (width <= 768px) {
  .console-page-header {
    align-items: stretch;
    flex-direction: column;
  }

  .console-page-header__actions {
    justify-content: flex-start;
  }

  .console-page-header__actions :deep(.console-page-header__select),
  .console-page-header__actions :deep(.console-page-header__auto-wrap) {
    width: 100%;
  }

  .console-page-header__actions :deep(.console-page-header__auto-wrap) {
    justify-content: space-between;
  }
}
</style>
