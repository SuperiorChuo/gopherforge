<template>
  <div :class="`${prefix}-footer`">© 2021-{{ new Date().getFullYear() }} 后台管理系统 版权所有</div>
</template>
<script setup lang="ts">
import { prefix } from '@/config/global';
</script>
<style lang="less" scoped>
.@{starter-prefix}-footer {
  display: flex;
  min-height: 36px;
  align-items: center;
  justify-content: center;
  color: #94a3b8;
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}
</style>
