<script setup lang="ts">
import { useData, withBase } from 'vitepress'
import { computed } from 'vue'

const { lang } = useData()
const isEnglish = computed(() => lang.value.startsWith('en'))
const pageLink = (link: string) => withBase(`${link}.html`)

const copy = computed(() => isEnglish.value ? {
  kicker: 'FROM ZERO TO PRODUCTION',
  title: 'From the first command,<br><span>to a running service.</span>',
  description: 'A scaffold is not just generated code. GopherForge wires architecture, security, observability and delivery together so every step is checkable.',
  pathTitle: 'Get going',
  pathDescription: 'Start it up, extend by domain, ship it.',
  steps: [
    { number: '01', title: 'Start', text: 'Bring up the stack and sign in within 15 minutes.', link: '/en/guide/getting-started', linkText: 'Quick start' },
    { number: '02', title: 'Understand', text: 'See how services are split, how the gateway authenticates and where data flows.', link: '/en/guide/architecture', linkText: 'View architecture' },
    { number: '03', title: 'Extend', text: 'Add business services without touching the platform core.', link: '/en/guide/extend', linkText: 'Extend the stack' },
  ],
  terminal: 'The local stack is the prod stack',
  commands: ['make compose-up', 'make smoke', 'make verify'],
  ready: 'READY',
} : {
  kicker: '从零到上线',
  title: '从第一条命令，<br><span>到真正跑在生产环境。</span>',
  description: '脚手架不只是一段生成代码。GopherForge 把架构、安全、可观测和交付放在同一套工程里，每一步都有据可查。',
  pathTitle: '上手路径',
  pathDescription: '先跑起来，再按域扩展，最后放心上线。',
  steps: [
    { number: '01', title: '启动', text: '15 分钟拉起技术栈并完成首次登录。', link: '/guide/getting-started', linkText: '快速上手' },
    { number: '02', title: '理解', text: '弄清服务怎么分、网关怎么鉴权、数据怎么流。', link: '/guide/architecture', linkText: '查看架构' },
    { number: '03', title: '扩展', text: '用领域服务加业务，不碰平台底座。', link: '/guide/extend', linkText: '二次开发' },
  ],
  terminal: '本地栈即生产栈',
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
