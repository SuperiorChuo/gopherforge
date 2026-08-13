import type { ReactNode } from 'react'
import { Drawer, type DrawerProps } from 'antd'

export type EntityDetailDrawerProps = Omit<DrawerProps, 'children' | 'open' | 'title' | 'onClose'> & {
  title: ReactNode
  open: boolean
  onClose: () => void
  children: ReactNode
}

/** 统一实体详情抽屉：收敛详情页宽度、销毁与关闭行为的公共入口。 */
export default function EntityDetailDrawer({
  title,
  open,
  onClose,
  children,
  width = 'min(640px, 100vw)',
  destroyOnHidden = true,
  ...drawerProps
}: EntityDetailDrawerProps) {
  return (
    <Drawer
      {...drawerProps}
      title={title}
      open={open}
      onClose={onClose}
      width={width}
      destroyOnHidden={destroyOnHidden}
    >
      {children}
    </Drawer>
  )
}
