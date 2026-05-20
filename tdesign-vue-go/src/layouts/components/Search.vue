<template>
  <div v-if="layout === 'side'" class="header-menu-search">
    <t-input
      class="header-search"
      :class="[{ 'hover-active': isSearchFocus }]"
      :placeholder="t('layout.searchPlaceholder')"
      @blur="changeSearchFocus(false)"
      @focus="changeSearchFocus(true)"
    >
      <template #prefix-icon>
        <t-icon class="icon" name="search" size="16" />
      </template>
    </t-input>
  </div>

  <div v-else class="header-menu-search-left">
    <t-button
      :class="{ 'search-icon-hide': isSearchFocus }"
      theme="default"
      shape="square"
      variant="text"
      @click="changeSearchFocus(true)"
    >
      <t-icon name="search" />
    </t-button>
    <t-input
      v-model="searchData"
      class="header-search"
      :class="[{ 'width-zero': !isSearchFocus }]"
      placeholder="输入要搜索内容"
      :autofocus="isSearchFocus"
      @blur="changeSearchFocus(false)"
    >
      <template #prefix-icon>
        <t-icon name="search" size="16" />
      </template>
    </t-input>
  </div>
</template>
<script setup lang="ts">
import { ref } from 'vue';

import { t } from '@/locales';

defineProps({
  layout: {
    type: String,
    default: '',
  },
});

const isSearchFocus = ref(false);
const searchData = ref('');
const changeSearchFocus = (value: boolean) => {
  if (!value) {
    searchData.value = '';
  }
  isSearchFocus.value = value;
};
</script>
<style lang="less" scoped>
.header-menu-search {
  display: flex;
  margin-left: 4px;

  .hover-active {
    background: #f1f5fb;
  }

  .t-icon {
    color: var(--td-text-color-primary) !important;
  }

  .header-search {
    width: 300px;

    :deep(.t-input) {
      height: 34px;
      border: 1px solid transparent;
      border-radius: 999px;
      outline: none;
      background: #f6f8fc;
      box-shadow: none;
      transition: background @anim-duration-base linear;

      .t-input__inner {
        color: #0f172a;
        transition: background @anim-duration-base linear;
        background: none;
      }

      &:hover {
        border-color: #dbe5f2;
        background: #f1f5fb;

        .t-input__inner {
          background: transparent;
        }
      }

      &:focus-within {
        border-color: rgb(0 82 217 / 22%);
        background: #fff;
        box-shadow: 0 0 0 3px rgb(0 82 217 / 8%);
      }
    }
  }
}

.t-button {
  margin: 0;
  transition: opacity @anim-duration-base @anim-time-fn-easing;

  .t-icon {
    font-size: 20px;

    &.general {
      margin-right: 16px;
    }
  }
}

.search-icon-hide {
  opacity: 0;
}

.header-menu-search-left {
  display: flex;
  align-items: center;

  .header-search {
    width: 260px;
    transition: width @anim-duration-base @anim-time-fn-easing;

    :deep(.t-input) {
      height: 34px;
      border: 1px solid transparent;
      border-radius: 999px;
      background: #f6f8fc;

      &:focus-within {
        border-color: rgb(0 82 217 / 22%);
        background: #fff;
        box-shadow: 0 0 0 3px rgb(0 82 217 / 8%);
      }
    }

    &.width-zero {
      width: 0;
      opacity: 0;
    }
  }
}
</style>
