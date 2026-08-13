import type { HTMLAttributes, ReactNode } from 'react'
import { Card } from 'antd'

export type ListPageShellProps = {
  className?: string
  filter?: ReactNode
  toolbar: ReactNode
  children: ReactNode
  wrapperProps?: Omit<HTMLAttributes<HTMLDivElement>, 'className' | 'children'>
}

/** 列表页统一骨架：筛选卡片 → 工具栏/内容卡片。 */
export default function ListPageShell({
  className,
  filter,
  toolbar,
  children,
  wrapperProps,
}: ListPageShellProps) {
  return (
    <div
      {...wrapperProps}
      className={['page-list', className].filter(Boolean).join(' ')}
    >
      {filter}
      <Card className="list-main-card" bordered={false}>
        {toolbar}
        {children}
      </Card>
    </div>
  )
}
