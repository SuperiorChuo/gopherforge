<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
const pageLink = (link: string) => withBase(`${link}.html`)

const copy = computed(() => isEnglish.value ? {
  kicker: 'BUILD WITH CONFIDENCE',
  title: 'From first command to production,<br><span>one coherent path.</span>',
  description: 'A scaffold should not stop at generated code. GopherForge connects architecture, security, observability and delivery into a verifiable engineering loop.',
  pathTitle: 'Your delivery path',
  pathDescription: 'Start small, extend by domain, ship with confidence.',
  steps: [
    { number: '01', title: 'Start', text: 'Bring up the full stack and sign in within 15 minutes.', link: '/en/guide/getting-started', linkText: 'Quick start' },
    { number: '02', title: 'Understand', text: 'Learn service boundaries, gateway auth and data flow.', link: '/en/guide/architecture', linkText: 'View architecture' },
    { number: '03', title: 'Extend', text: 'Add a business service without coupling it to the platform.', link: '/en/guide/extend', linkText: 'Extend the stack' },
  ],
  terminal: 'Production-minded from day one',
  commands: ['make compose-up', 'make smoke', 'make verify'],
  ready: 'READY',
} : {
  kicker: 'BUILD WITH CONFIDENCE',
  title: '从第一条命令到生产环境，<br><span>始终沿着一条清晰路径。</span>',
  description: '脚手架不该止步于生成代码。GopherForge 将架构、安全、可观测与交付串成一套可验证的工程闭环。',
  pathTitle: '你的交付路径',
  pathDescription: '小步启动、按域扩展、带着信心交付。',
  steps: [
    { number: '01', title: '启动', text: '15 分钟拉起完整技术栈并完成首次登录。', link: '/guide/getting-started', linkText: '快速上手' },
    { number: '02', title: '理解', text: '掌握服务边界、网关鉴权与核心数据流。', link: '/guide/architecture', linkText: '查看架构' },
    { number: '03', title: '扩展', text: '以领域服务扩展业务，不污染平台基础能力。', link: '/guide/extend', linkText: '二次开发' },
  ],
  terminal: '从第一天就面向生产',
  commands: ['make compose-up', 'make smoke', 'make verify'],
  ready: 'READY',
})
</script>

<template>
  <section class="home-journey">
    <div class="home-journey-heading">
      <span class="section-kicker">{{ copy.kicker }}</span>
      <h2 v-html="copy.title" />
      <p>{{ copy.description }}</p>
    </div>

    <div class="journey-glass">
      <div class="journey-copy">
        <span class="journey-label">WORKFLOW / 01—03</span>
        <h3>{{ copy.pathTitle }}</h3>
        <p>{{ copy.pathDescription }}</p>
        <ol>
          <li v-for="step in copy.steps" :key="step.number">
            <span class="journey-number">{{ step.number }}</span>
            <div>
              <strong>{{ step.title }}</strong>
              <p>{{ step.text }}</p>
              <a :href="pageLink(step.link)">{{ step.linkText }} <span>→</span></a>
            </div>
          </li>
        </ol>
      </div>

      <div class="journey-terminal">
        <div class="terminal-top">
          <span><i /><i /><i /></span>
          <small>gopherforge — zsh</small>
        </div>
        <div class="terminal-body">
          <span class="terminal-badge">{{ copy.terminal }}</span><span class="terminal-cursor" aria-hidden="true" />
          <div v-for="(command, index) in copy.commands" :key="command" class="terminal-command">
            <span class="terminal-prompt">❯</span>
            <code>{{ command }}</code>
            <span class="terminal-result"><i />{{ index === copy.commands.length - 1 ? copy.ready : 'OK' }}</span>
          </div>
          <div class="terminal-chart" aria-hidden="true">
            <span style="--bar: 42%" /><span style="--bar: 64%" /><span style="--bar: 52%" />
            <span style="--bar: 78%" /><span style="--bar: 69%" /><span style="--bar: 92%" />
            <span style="--bar: 84%" /><span style="--bar: 100%" /><span style="--bar: 94%" />
          </div>
          <div class="terminal-status">
            <span><i /> API</span><span><i /> WEB</span><span><i /> DATA</span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
