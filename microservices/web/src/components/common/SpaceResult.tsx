import type { ReactNode } from 'react'

interface SpaceResultProps {
  code: string
  title: string
  description: string
  /** 可选补充信息（如出错的路径），渲染为玻璃小胶囊 */
  detail?: ReactNode
  actions: ReactNode
}

// 深空+玻璃拟态的结果页面板（403/404/500 共用），与登录页同一套视觉语言
export default function SpaceResult({ code, title, description, detail, actions }: SpaceResultProps) {
  return (
    <div className="space-result">
      <div className="space-result-aurora space-result-aurora-1" />
      <div className="space-result-aurora space-result-aurora-2" />
      <div className="space-result-grid" />
      <div className="space-result-card">
        <div className="space-result-code">{code}</div>
        <div className="space-result-title">{title}</div>
        <div className="space-result-desc">{description}</div>
        {detail && <div className="space-result-detail">{detail}</div>}
        <div className="space-result-actions">{actions}</div>
      </div>
    </div>
  )
}
