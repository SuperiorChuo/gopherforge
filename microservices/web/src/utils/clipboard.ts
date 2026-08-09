// 复制文本到剪贴板：优先 Clipboard API（仅 HTTPS/localhost 安全上下文可用），
// HTTP 内网环境（18100/13100 明文）navigator.clipboard 不存在或拒绝时降级
// document.execCommand('copy')（textarea + 选中复制）。返回是否成功——
// 调用方按返回值提示，禁止静默报成功（cc/prompts 曾用 ?. 静默吞失败前车）。
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Clipboard API 存在但被拒（权限/焦点），走降级
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
