<script setup lang="ts">
import { useData } from 'vitepress'
import { computed } from 'vue'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
const label = computed(() => (isEnglish.value ? 'OPEN SOURCE · MIT LICENSE' : '开源项目 · MIT 协议'))
</script>

<template>
  <div class="hero-eyebrow">
    <span class="hero-eyebrow-dot" />
    <span>{{ label }}</span>
  </div>
</template>
