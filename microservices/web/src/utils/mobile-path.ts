/** 管理员移动端入口前缀。C 端不走这里。 */
export const MOBILE_BASE = '/m'
export const MOBILE_LOGIN = '/m/login'
export const DESKTOP_LOGIN = '/login'

export function isMobileAppPath(pathname = window.location.pathname): boolean {
  return pathname === MOBILE_BASE || pathname.startsWith(`${MOBILE_BASE}/`)
}

/** 移动壳内的登录地址；已在登录页则不再套 redirect。 */
export function mobileLoginPath(from?: string): string {
  const current = from ?? `${window.location.pathname}${window.location.search}`
  const path = current.split('?')[0] ?? ''
  if (!isMobileAppPath(path) || path === MOBILE_LOGIN) return MOBILE_LOGIN
  return `${MOBILE_LOGIN}?redirect=${encodeURIComponent(current)}`
}

/** 401 / 掉登录时按当前壳回跳，桌面仍回 /login，避免把管理台会话送进 /m。 */
export function loginPathForCurrentLocation(from?: string): string {
  const current = from ?? `${window.location.pathname}${window.location.search}`
  const path = current.split('?')[0] ?? ''
  return isMobileAppPath(path) ? mobileLoginPath(current) : DESKTOP_LOGIN
}
