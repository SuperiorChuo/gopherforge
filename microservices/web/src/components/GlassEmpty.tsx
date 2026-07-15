import { InboxOutlined } from '@ant-design/icons'

// 全站空状态：玻璃托盘 + 呼吸光晕,经 ConfigProvider renderEmpty 注入,
// 表格/下拉/列表的空态都走这里,替代 antd 默认的灰色小人插画。
// 页面也可以直接 <GlassEmpty text="暂无公告" compact /> 使用。
export default function GlassEmpty({ text = '暂无数据', compact = false }: {
  text?: string
  compact?: boolean
}) {
  return (
    <div className={`glass-empty ${compact ? 'glass-empty-compact' : ''}`}>
      <div className="glass-empty-icon">
        <InboxOutlined />
      </div>
      <div className="glass-empty-text">{text}</div>
    </div>
  )
}
