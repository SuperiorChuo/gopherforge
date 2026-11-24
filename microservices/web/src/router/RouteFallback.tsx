// 路由懒加载兜底：玻璃卡片骨架，比孤零零的 Spin 更接近成品布局
export default function RouteFallback() {
  return (
    <div className="route-fallback">
      <div className="route-fallback-bar" />
      <div className="route-fallback-card" />
    </div>
  )
}
