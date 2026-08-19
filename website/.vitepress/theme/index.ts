import DefaultTheme from 'vitepress/theme'
import { withBase, type Theme } from 'vitepress'
import { h } from 'vue'
import GlassBackdrop from './components/GlassBackdrop.vue'
import HeroForge from './components/HeroForge.vue'
import HomeJourney from './components/HomeJourney.vue'
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
      'home-hero-info-before': () =>
        h('div', { class: 'hero-eyebrow' }, [
          h('span', { class: 'hero-eyebrow-dot' }),
          h('span', 'OPEN SOURCE · PRODUCTION READY'),
        ]),
      'home-hero-image': () => h(HeroForge),
      'home-hero-actions-after': () =>
        h('div', { class: 'hero-stack' }, [
          h('span', 'GO 1.26'),
          h('span', 'REACT 19'),
          h('span', 'ANT DESIGN 6'),
          h('span', 'POSTGRESQL 18'),
        ]),
      'home-features-after': () => h(HomeJourney),
    }),
} satisfies Theme
