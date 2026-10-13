// 路由发现器：从 src/router/index.tsx 真源提取全部可扫描路径。
//
// 背景：layout-scan 曾用写死的 ALL 数组登记待扫页面，08 月中旬后新增的
// catalog/directory/me/ops/security/social 及 hermes/cc 一半新页等 ~80 条
// 路由从未进过回归扫描——「加页面忘登记」是制度性盲区。本模块直接读路由
// 定义提取路径：新增页面自动入扫，无需任何登记动作。
//
// 实现说明：项目 typescript 已升 v7（native），不再向 JS 暴露 parser API
// （ts.createSourceFile 为 undefined）。这里是零依赖窄词法扫描器，只识别本仓
// 路由文件的规整结构：对象字面量的 path/index/children 三键、字符串字面量、
// 注释与引号边界；其余 token（JSX 属性、函数参数等）整体忽略。
//
// 防回归锁：历史人工清单（LEGACY_BASELINE）必须被发现结果完整覆盖，否则
// 抛错阻断——真要删路由时须从基线移除条目并在提交说明理由（同
// rules-drift-check 哲学：覆盖面变化必须是显式决定）。
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export interface RouteDiscovery {
  /** 桌面壳可扫描绝对路径（不含动态参数段与显式排除项），已排序去重 */
  desktop: string[]
  /** 移动壳（/m 子树）可扫描绝对路径 */
  mobile: string[]
  /** 含 `:param` 的路由——扫描值无法判定，仅列出供人工对照 */
  paramOnly: string[]
}

// 显式排除台账（必须写理由；发现器永不静默丢路径）
const EXCLUDED = new Set([
  // OAuth 授权流端点页：依赖外部 client 会话状态，非管理界面
  '/oauth/authorize',
  // 外部 Grafana/Jaeger iframe 嵌入：iframe 可用性不是本仓布局质量口径
  '/monitor/grafana',
  '/monitor/jaeger',
])

// 2026-08-27 之前 layout-scan 手工维护的历史清单（66 条）。作用唯一：防止
// 解析器在某次重构里悄悄漏掉已知页面。新页面不需要加进来。
const LEGACY_BASELINE = [
  '/dashboard', '/system/user', '/system/role', '/system/permission', '/system/menu',
  '/system/department', '/system/post', '/system/tenant', '/system/tenant-packages',
  '/system/setting', '/system/notice', '/system/inbox', '/system/notify-templates',
  '/system/sms', '/system/login-log', '/system/operation-log', '/system/audit-log',
  '/system/online-user', '/system/dict', '/system/file', '/system/codegen',
  '/system/errcodes', '/system/ai-models', '/system/ai-usage', '/system/ai-quotas',
  '/system/security-events', '/monitor/server', '/monitor/mysql', '/monitor/redis',
  '/monitor/job', '/monitor/alerts', '/bpm/tasks', '/bpm/start', '/bpm/instances',
  '/bpm/definitions', '/crm/dashboard', '/crm/leads', '/crm/customers', '/crm/pipeline',
  '/crm/contracts', '/im/desk', '/mp/accounts', '/mp/messages', '/mp/articles',
  '/mp/menu', '/mp/fans', '/ai/assistant', '/ai/knowledge', '/ticket', '/pay',
  '/cc/workbench', '/cc/calls', '/cc/console', '/cc/extensions', '/cc/trunks',
  '/cc/ivr', '/cc/queues', '/cc/monitor', '/cc/billing', '/hermes/overview',
  '/hermes/office', '/hermes/mine', '/hermes/collaborations', '/profile',
  '/visibility/overview',
]

function isMobileShell(p: string): boolean {
  return p === '/m' || p.startsWith('/m/')
}

/** '/dashboard' + 'calls' → '/dashboard/calls'；'' + x → '/x' */
function joinPath(pfx: string, seg: string): string {
  if (pfx === '') return `/${seg}`
  return pfx.endsWith('/') ? `${pfx}${seg}` : `${pfx}/${seg}`
}

interface ObjFrame {
  kind: 'object'
  /** 直接包裹该对象的数组的绝对前缀（栈内最顶 array 帧的 prefix） */
  pfx: string
  curKey: 'path' | 'index' | 'children' | null
  /** 绝对路径型字面量（'/' 开头） */
  absPath?: string
  /** 相对路径型字面量（拼接父前缀） */
  relPath?: string
  /** 是否声明了 path 键（无论哪种形态） */
  hasPath?: boolean
  indexFlag?: boolean
  hasChildren?: boolean
}
interface ArrFrame {
  kind: 'array'
  pfx: string
}

function extractPaths(source: string): RouteDiscovery {
  const anchor = /const\s+routes\s*:[^=]*=\s*\[/.exec(source)
  if (!anchor) throw new Error('router 源里找不到 `const routes ... = [` 锚点')

  const desktop = new Set<string>()
  const mobile = new Set<string>()
  const paramOnly = new Set<string>()
  const emit = (abs: string): void => {
    ;(isMobileShell(abs) ? mobile : desktop).add(abs)
  }

  const stack: (ArrFrame | ObjFrame)[] = []
  const topArrayPrefix = (): string => {
    for (let k = stack.length - 1; k >= 0; k -= 1) if (stack[k].kind === 'array') return stack[k].pfx
    return ''
  }
  const topObject = (): ObjFrame | undefined => {
    for (let k = stack.length - 1; k >= 0; k -= 1) {
      const f = stack[k]
      return f.kind === 'object' ? f : undefined
    }
    return undefined
  }

  let depth = 0 // 全括号计数（含 '(' ')'），用于判定根数组闭合
  let started = false
  let readingString: string | null = null
  let strStart = -1 // 当前字符串字面量的起始引号下标
  let readingComment: 'line' | 'block' | null = null
  let curKeyAccepted = false // 已消费过值，等待逗号重置
  const i0 = anchor.index + anchor[0].length - 1

  for (let i = i0; i < source.length; i += 1) {
    const ch = source[i]
    if (readingString !== null) {
      if (ch === '\\') {
        i += 1
        continue
      }
      if (ch === readingString) {
        const str = source.slice(strStart + 1, i)
        readingString = null
        const obj = topObject()
        if (obj?.curKey === 'path') {
          if (str.startsWith('/')) obj.absPath = str
          else obj.relPath = str
          obj.hasPath = true
          // 关键：立即清键位。否则同一对象里后续的字符串字面量
          // （如 element: lazyLoad(() => import('@/pages/x')) 里的模块串）
          // 会再次命中 path 键并覆盖掉真实路由路径。
          obj.curKey = null
          curKeyAccepted = true
        }
      }
      continue
    }
    if (readingComment === 'line') {
      if (ch === '\n') readingComment = null
      continue
    }
    if (readingComment === 'block') {
      if (ch === '*' && source[i + 1] === '/') {
        readingComment = null
        i += 1
      }
      continue
    }

    if (ch === '"' || ch === "'") {
      readingString = ch
      strStart = i
      continue
    }
    if (ch === '/' && source[i + 1] === '/') {
      readingComment = 'line'
      continue
    }
    if (ch === '/' && source[i + 1] === '*') {
      readingComment = 'block'
      i += 1
      continue
    }

    if (ch === '[') {
      depth += 1
      const obj = topObject()
      if (!started) {
        // 锚点末尾的根数组
        started = true
        stack.push({ kind: 'array', pfx: '' })
        continue
      }
      if (obj?.curKey === 'children' && !curKeyAccepted) {
        // children 的真数组：其前缀 = 当前对象的解析态绝对路径
        obj.hasChildren = true
        const abs = resolveObjAbs(obj)
        stack.push({ kind: 'array', pfx: abs })
        curKeyAccepted = true
        continue
      }
      continue // 其余 [] （如 routeStyles 数组）只进深度计数，不建帧
    }
    if (ch === ']') {
      depth -= 1
      if (started && depth === 0) break // 根数组结束，后续代码不再关心
      if (stack.at(-1)?.kind === 'array') stack.pop()
      continue
    }
    if (ch === '{') {
      depth += 1
      // 进入新对象必须重置键位状态：children: [ 直接跟对象时上一次
      // 消费 children 键留下的 curKeyAccepted=true 若不清除，
      // 该对象的 index/path 键识别会被跳过（历史 bug：'/'与'/m'丢失）。
      curKeyAccepted = false
      stack.push({ kind: 'object', pfx: topArrayPrefix(), curKey: null })
      continue
    }
    if (ch === '}') {
      depth -= 1
      if (started && depth === 0) break
      const obj = stack.pop()
      if (obj?.kind === 'object') settleObject(obj)
      curKeyAccepted = false
      continue
    }
    if (ch === '(' || ch === ')') {
      depth += 1 * (ch === '(' ? 1 : -1)
      continue
    }
    if (ch === ',') {
      curKeyAccepted = false
      continue
    }
    // 标识符：仅当它可能是键（首个 token）或 index 的 true 时处理
    if (/[A-Za-z_$]/.test(ch)) {
      let j = i
      while (j < source.length && /[A-Za-z0-9_$]/.test(source[j])) j += 1
      const ident = source.slice(i, j)
      i = j - 1
      const obj = topObject()
      if (!obj) continue
      if (
        !curKeyAccepted &&
        (ident === 'path' || ident === 'index' || ident === 'children') &&
        peekColon(i)
      ) {
        obj.curKey = ident
        continue
      }
      if (!curKeyAccepted && obj.curKey === 'index' && ident === 'true') {
        obj.indexFlag = true
        obj.curKey = null
        curKeyAccepted = true
      }
      continue
    }
  }
  if (depth !== 0 || readingString !== null || readingComment !== null) {
    throw new Error('router 扫描异常终止：括号/字符串不平衡——router 结构变了请同步本扫描器')
  }

  function resolveObjAbs(obj: ObjFrame): string {
    if (obj.absPath !== undefined) return obj.absPath
    if (obj.relPath !== undefined) return joinPath(obj.pfx, obj.relPath)
    return obj.pfx
  }
  function settleObject(obj: ObjFrame): void {
    // 通配符兜底路由（path: '*'）：无固定形态不扫（其值会被存进 relPath）
    if (obj.relPath === '*' || obj.absPath === '*') return
    const abs = resolveObjAbs(obj)
    if (!abs || abs.includes(':')) {
      paramOnly.add(abs)
      return
    }
    if ((obj.indexFlag === true && !obj.hasPath) || (obj.hasPath && !obj.hasChildren)) {
      emit(abs)
    }
  }
  function peekColon(from: number): boolean {
    for (let k = from + 1; k < source.length; k += 1) {
      const c = source[k]
      if (c === ' ' || c === '\t' || c === '\n' || c === '\r') continue
      return c === ':'
    }
    return false
  }

  const visible = (p: string): boolean => !EXCLUDED.has(p)
  return {
    desktop: [...desktop].filter(visible).sort(),
    mobile: [...mobile].filter(visible).sort(),
    paramOnly: [...paramOnly].sort(),
  }
}

let cache: RouteDiscovery | null = null

export function discoverScanRoutes(): RouteDiscovery {
  cache ??= (() => {
    const src = readFileSync(path.resolve(__dirname, '../src/router/index.tsx'), 'utf8')
    const d = extractPaths(src)
    // 防回归锁：历史上被人眼确认过的扫描清单一条都不能丢。
    const all = new Set([...d.desktop, ...d.mobile])
    const lost = LEGACY_BASELINE.filter((p) => !all.has(p))
    if (lost.length) {
      throw new Error(
        `路由发现丢掉了 ${lost.length} 条历史扫描路径（若属有意下线，请从 LEGACY_BASELINE 删除条目并在提交说明理由）：${lost.join(', ')}`,
      )
    }
    return d
  })()
  return cache
}
