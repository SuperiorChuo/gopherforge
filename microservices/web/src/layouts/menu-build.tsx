import type { MenuProps } from 'antd'
import { UserOutlined } from '@ant-design/icons'
import type { MenuItem as ApiMenuItem } from '@/types'
import { ROUTE_PERMISSIONS } from '@/router/route-permissions'
import type { PaletteItem } from '@/components/common/CommandPalette'
import i18n from '@/i18n/init'
import { type MenuDef, iconOf } from './menu-defs'

type MenuItem2 = Required<MenuProps>['items'][number]

function makeItem(
  label: React.ReactNode,
  key: string,
  icon?: React.ReactNode,
  children?: MenuItem2[],
): MenuItem2 {
  return { label, key, icon, children } as MenuItem2
}

// /user/menus 树 → 侧栏定义。后端已按权限过滤，这里只做展示映射。
export function apiMenusToDefs(menus: ApiMenuItem[], topLevel = true): MenuDef[] {
  return [...menus]
    .filter((m) => m.hidden !== 1)
    .sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0))
    .map((m): MenuDef | null => {
      const kids = m.children?.length ? apiMenusToDefs(m.children, false) : []
      const isContainer = (m.children?.length ?? 0) > 0 || m.component === 'Layout'
      if (isContainer) {
        if (kids.length === 0) return null
        // 单子项容器折叠为叶子（如"仪表盘"Layout + 唯一 index 页），跳容器自身路径
        if (kids.length === 1 && topLevel) {
          return { label: m.title, key: m.path, icon: iconOf(m.icon), perm: kids[0].perm }
        }
        return { label: m.title, key: m.path, icon: iconOf(m.icon), children: kids }
      }
      return { label: m.title, key: m.path, icon: iconOf(m.icon), perm: m.permission || undefined }
    })
    .filter((d): d is MenuDef => d !== null)
}

function leafVisible(d: MenuDef, hasPerm: (code?: string) => boolean): boolean {
  return hasPerm(d.perm ?? ROUTE_PERMISSIONS[d.key])
}

export function buildMenuItems(defs: MenuDef[], hasPerm: (code?: string) => boolean): MenuItem2[] {
  return defs
    .map((d) => {
      if (d.children) {
        const children = buildMenuItems(d.children, hasPerm)
        return children.length > 0 ? makeItem(i18n.t(d.label), d.key, d.icon, children) : null
      }
      return leafVisible(d, hasPerm) ? makeItem(i18n.t(d.label), d.key, d.icon) : null
    })
    .filter((item): item is MenuItem2 => item !== null)
}

// 命令面板数据：与菜单同源、同权限过滤
export function buildPaletteItems(defs: MenuDef[], hasPerm: (code?: string) => boolean): PaletteItem[] {
  const result: PaletteItem[] = []
  const walk = (nodes: MenuDef[], group: string) => {
    nodes.forEach((d) => {
      if (d.children) {
        walk(d.children, i18n.t(d.label))
      } else if (leafVisible(d, hasPerm)) {
        result.push({ label: i18n.t(d.label), path: d.key, group, icon: d.icon })
      }
    })
  }
  walk(defs, i18n.t('导航'))
  result.push({ label: i18n.t('个人中心'), path: '/profile', group: i18n.t('导航'), icon: <UserOutlined /> })
  return result
}

