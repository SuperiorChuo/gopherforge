import DefaultTheme from 'vitepress/theme'
import { withBase, type Theme } from 'vitepress'
import { h } from 'vue'
import GlassBackdrop from './components/GlassBackdrop.vue'
import HeroEyebrow from './components/HeroEyebrow.vue'
import HeroForge from './components/HeroForge.vue'
import HomeJourney from './components/HomeJourney.vue'
import { SITE_META } from '../site-meta'
import './styles/foundation.css'
import './styles/home.css'
import './styles/docs.css'
import './styles/responsive.css'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'layout-top': () => h(GlassBackdrop),
      'nav-bar-title-before': () =>
        h('img', {
          class: 'gopherforge-nav-mark',
          src: withBase('/brand/gopherforge-mark.svg'),
          alt: '',
          width: '34',
          height: '34',
        }),
      'nav-bar-title-after': () => h('span', { class: 'gopherforge-nav-badge' }, 'DOCS'),
      'home-hero-info-before': () => h(HeroEyebrow),
      'home-hero-image': () => h(HeroForge),
      'home-hero-actions-after': () =>
        h('div', { class: 'hero-stack' }, [
          h('span', `GO ${SITE_META.stack.go}`),
          h('span', `REACT ${SITE_META.stack.react}`),
          h('span', `ANT DESIGN ${SITE_META.stack.antd}`),
          h('span', `POSTGRESQL ${SITE_META.stack.postgres}`),
        ]),
      'home-features-after': () => h(HomeJourney),
    }),
} satisfies Theme
