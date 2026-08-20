<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'
import { SITE_META } from '../../site-meta'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
</script>

<template>
  <div class="hero-forge" aria-hidden="true">
    <div class="hero-forge-window">
      <div class="hero-window-rim" />
      <div class="hero-window-top">
        <div class="hero-window-lights"><i /><i /><i /></div>
        <div class="hero-window-path"><span>gopherforge / platform</span><em class="hero-version">{{ SITE_META.release.tag }}</em></div>
        <div class="hero-window-live">OSS</div>
      </div>
      <div class="hero-window-content">
        <div class="hero-brand-core">
          <img :src="withBase('/brand/gopherforge-mark.svg')" alt="" width="62" height="62" />
          <div>
            <span>{{ isEnglish ? 'REFERENCE STACK' : '参考架构' }}</span>
            <strong>{{ isEnglish ? `${SITE_META.architecture.goServices} Go services · 1 gateway` : `${SITE_META.architecture.goServices} 个 Go 服务 · 1 个网关` }}</strong>
          </div>
        </div>

        <div class="hero-service-map">
          <div class="hero-map-line hero-map-line-one" />
          <div class="hero-map-line hero-map-line-two" />
          <div class="hero-map-line hero-map-line-three" />
          <div class="hero-map-line hero-map-line-four" />
          <div class="hero-service hero-service-gateway"><i />TRAEFIK</div>
          <div class="hero-service hero-service-auth"><i />AUTH</div>
          <div class="hero-service hero-service-system"><i />SYSTEM</div>
          <div class="hero-service hero-service-bpm"><i />BPM</div>
          <div class="hero-service hero-service-data"><i />DATA</div>
          <div class="hero-service-core"><span>GF</span><i /></div>
        </div>

        <div class="hero-metrics">
          <div><span>{{ isEnglish ? 'Go services' : 'Go 服务' }}</span><strong>{{ SITE_META.architecture.goServices }}</strong><i>{{ isEnglish ? 'by domain' : '按域拆分' }}</i></div>
          <div><span>{{ isEnglish ? 'Release images' : '发布镜像' }}</span><strong>{{ SITE_META.architecture.releaseImages }}</strong><i>{{ isEnglish ? 'per release' : '每个版本' }}</i></div>
          <div><span>{{ isEnglish ? 'License' : '许可证' }}</span><strong>MIT</strong><i>{{ isEnglish ? 'open source' : '开源' }}</i></div>
        </div>
      </div>
    </div>
    <div class="hero-float-card hero-float-security">
      <span class="hero-float-icon">✓</span>
      <div><strong>RBAC</strong><small>{{ isEnglish ? 'Gateway enforced' : '网关统一鉴权' }}</small></div>
    </div>
    <div class="hero-float-card hero-float-telemetry">
      <span class="hero-wave"><i /><i /><i /><i /></span>
      <div><strong>OTel</strong><small>{{ isEnglish ? 'Optional tracing' : '可选链路追踪' }}</small></div>
    </div>
  </div>
</template>
