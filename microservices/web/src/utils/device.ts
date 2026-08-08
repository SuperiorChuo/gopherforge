// 设备指纹：首访生成并持久化一个匿名 UUID（localStorage），此后所有请求经
// request.ts 全局注入 X-Device-ID。服务端据此识别"新设备登录"，供异地/新设备
// 风控通知使用。纯匿名标识，不采集任何设备属性。
const DEVICE_ID_KEY = 'device_id'

// 隐私模式兜底 ID：localStorage 抛错时退化为页面会话级（模块变量只生成一次），
// 不能每次调用都新生成——否则每次请求 ID 都不同，服务端会判定"每次都是新设备"
// 并频繁触发风控告警。
let fallbackID = ''

export function getDeviceID(): string {
  try {
    let id = localStorage.getItem(DEVICE_ID_KEY)
    if (!id) {
      id = crypto.randomUUID ? crypto.randomUUID() : `dev-${Date.now()}-${Math.random().toString(36).slice(2)}`
      localStorage.setItem(DEVICE_ID_KEY, id)
    }
    return id
  } catch {
    if (!fallbackID) {
      fallbackID = crypto.randomUUID ? crypto.randomUUID() : `dev-${Date.now()}-${Math.random().toString(36).slice(2)}`
    }
    return fallbackID
  }
}
