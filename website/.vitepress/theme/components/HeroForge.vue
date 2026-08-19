<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
</script>

<template>
  <div class="hero-forge" aria-hidden="true">
    <div class="hero-forge-window">
      <div class="hero-window-rim" />
      <div class="hero-window-top">
        <div class="hero-window-lights"><i /><i /><i /></div>
        <div class="hero-window-path"><span>gopherforge / platform</span><em class="hero-version">v0.6.0</em></div>
        <div class="hero-window-live"><i />LIVE</div>
      </div>
      <div class="hero-window-content">
        <div class="hero-brand-core">
          <img :src="withBase('/brand/gopherforge-mark.svg')" alt="" width="62" height="62" />
          <div>
            <span>{{ isEnglish ? 'SYSTEM STATUS' : '系统状态' }}</span>
            <strong>{{ isEnglish ? 'All systems operational' : '全系统运行正常' }}</strong>
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
          <div><span>{{ isEnglish ? 'Services' : '基础服务' }}</span><strong>8</strong><i>healthy</i></div>
          <div><span>{{ isEnglish ? 'Isolation' : '租户隔离' }}</span><strong>100%</strong><i>guarded</i></div>
          <div><span>{{ isEnglish ? 'Gateway' : '统一网关' }}</span><strong>1</strong><i>secured</i></div>
        </div>
      </div>
    </div>
    <div class="hero-float-card hero-float-security">
      <span class="hero-float-icon">✓</span>
      <div><strong>RBAC</strong><small>{{ isEnglish ? 'Policy enforced' : '策略已生效' }}</small></div>
    </div>
    <div class="hero-float-card hero-float-telemetry">
      <span class="hero-wave"><i /><i /><i /><i /></span>
      <div><strong>OTel</strong><small>{{ isEnglish ? 'Trace connected' : '链路已连接' }}</small></div>
    </div>
  </div>
</template>
