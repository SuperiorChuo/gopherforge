<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'
import { SITE_META } from '../../site-meta'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
const changelogLink = computed(() => withBase(isEnglish.value ? '/en/changelog.html' : '/changelog.html'))
</script>

<template>
  <div class="home-release-shell">
    <div class="home-release-bar" :aria-label="isEnglish ? 'Release information' : '版本信息'">
      <a class="release-version" :href="SITE_META.release.url" target="_blank" rel="noopener noreferrer">
        <span />{{ SITE_META.release.tag }}
      </a>
      <span class="release-boundary">{{ isEnglish ? '0.x · APIs and schemas may change' : '0.x 阶段 · API 与表结构可能变化' }}</span>
      <span class="release-divider" aria-hidden="true" />
      <span class="release-demo">{{ isEnglish ? 'Live Demo uses mock data' : '在线 Demo 使用演示数据' }}</span>
      <a class="release-notes" :href="changelogLink">{{ isEnglish ? 'Release notes' : '查看更新日志' }} <span>→</span></a>
    </div>
  </div>
</template>
