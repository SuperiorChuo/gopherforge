import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App.tsx'
import './index.css'

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
