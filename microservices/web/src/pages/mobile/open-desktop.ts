/** 同域打开桌面控制台：JWT 已在 localStorage，MainLayout 直接认，不必再登录。 */
export function openDesktopConsole(path = '/dashboard') {
  const target = path.startsWith('/') ? path : `/${path}`
  window.location.assign(target)
}
