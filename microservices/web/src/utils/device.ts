// 设备指纹：首访生成并持久化一个匿名 UUID（localStorage），此后所有请求经
// request.ts 全局注入 X-Device-ID。服务端据此识别"新设备登录"，供异地/新设备
// 风控通知使用。纯匿名标识，不采集任何设备属性。
const DEVICE_ID_KEY = 'device_id'

export function getDeviceID(): string {
  try {
    let id = localStorage.getItem(DEVICE_ID_KEY)
    if (!id) {
      id = crypto.randomUUID ? crypto.randomUUID() : `dev-${Date.now()}-${Math.random().toString(36).slice(2)}`
      localStorage.setItem(DEVICE_ID_KEY, id)
    }
    return id
  } catch {
    // localStorage 不可用（隐私模式等）：每次生成一个会话级 ID，尽力而为
    return `dev-${Date.now()}-${Math.random().toString(36).slice(2)}`
  }
}
