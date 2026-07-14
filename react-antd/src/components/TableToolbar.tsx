import type { ReactNode } from 'react'

interface TableToolbarProps {
  title: string
  total?: number
  extra?: ReactNode
}

export default function TableToolbar({ title, total, extra }: TableToolbarProps) {
  return (
    <div className="table-toolbar">
      <div className="table-toolbar-title">
        {title}
        {typeof total === 'number' && <span className="table-count">{total}</span>}
      </div>
      {extra && <div className="table-toolbar-extra">{extra}</div>}
    </div>
  )
}
