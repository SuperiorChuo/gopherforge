import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@/i18n/init'
import App from './App.tsx'
import './index.css'
import './styles/global-components.css'
import './styles/typography.css'
import './styles/dashboard.css'
import './styles/overlays.css'
import './styles/log-pages.css'
import './styles/theme-light.css'
import './styles/glass-system.css'
import './styles/auth-pages.css'
import './styles/app-layout.css'
import './list-pages.css'
import './styles/feedback.css'
import './styles/interaction-polish.css'
import './styles/profile.css'
import './styles/monitor.css'
import './styles/result-pages.css'
import './styles/file-pages.css'

async function bootstrap() {
  // 演示模式（GitHub Pages）：装假数据 adapter；正常构建时此分支被静态消除
  if (import.meta.env.VITE_DEMO === '1') {
    const { installDemoAdapter } = await import('./demo')
    installDemoAdapter()
  }
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}

void bootstrap()
