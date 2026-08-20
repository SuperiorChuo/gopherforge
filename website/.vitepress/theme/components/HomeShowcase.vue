<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'
import { SITE_META } from '../../site-meta'

const { isDark, lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
const screenshot = computed(() => withBase(isDark.value ? '/screenshots/dashboard.png' : '/screenshots/dashboard-light.png'))
</script>

<template>
  <section class="home-showcase">
    <div class="home-showcase-heading">
      <span class="section-kicker">{{ isEnglish ? 'REAL INTERFACE' : '真实界面' }}</span>
      <h2>{{ isEnglish ? 'The console, not a mockup' : '这里展示的就是控制台' }}</h2>
      <p>{{ isEnglish ? 'The screenshot follows the current light or dark theme. Open the demo to explore the same routes with mock data.' : '截图会跟随当前亮暗主题切换；在线 Demo 使用演示数据，但页面、路由和交互与控制台一致。' }}</p>
    </div>

    <figure class="showcase-frame">
      <a :href="SITE_META.urls.demo" target="_blank" rel="noopener noreferrer">
        <img :src="screenshot" :alt="isEnglish ? 'GopherForge dashboard' : 'GopherForge 系统概览'" width="1432" height="895" loading="lazy" decoding="async" />
      </a>
      <figcaption>
        <span>{{ isEnglish ? 'Dashboard · theme-aware screenshot' : '系统概览 · 跟随站点主题' }}</span>
        <a :href="SITE_META.urls.demo" target="_blank" rel="noopener noreferrer">{{ isEnglish ? 'Open Live Demo' : '打开在线 Demo' }} <span>→</span></a>
      </figcaption>
    </figure>

    <ul class="showcase-facts" :aria-label="isEnglish ? 'Interface facts' : '界面说明'">
      <li>{{ isEnglish ? 'Real application routes' : '真实应用路由' }}</li>
      <li>{{ isEnglish ? 'Mock data is clearly labelled' : '演示数据明确标注' }}</li>
      <li>{{ isEnglish ? 'Light and dark themes' : '亮暗双主题' }}</li>
    </ul>
  </section>
</template>
