import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Dropdown, Modal, Popconfirm, Space, Tooltip, type MenuProps } from 'antd'
import { MoreOutlined } from '@ant-design/icons'

export type TableRowAction = {
  key: string
  label: string
  icon?: ReactNode
  danger?: boolean
  disabled?: boolean
  loading?: boolean
  /** false 时不渲染该动作 */
  show?: boolean
  /** 点击前确认；菜单模式走 Modal.confirm */
  confirm?: string | { title: string; description?: string }
  onClick: () => void
}

export type TableRowActionsProps = {
  actions: TableRowAction[]
  /**
   * 行内最多展示几个图标按钮；超出进「更多」。
   * 默认 3：与用户/角色等三动作桌面列对齐。
   */
  maxInline?: number
  /** 全部收进「更多」（窄屏操作列） */
  menuOnly?: boolean
  /** 更多按钮 aria-label */
  ariaLabel?: string
  className?: string
}

function confirmTitle(confirm: NonNullable<TableRowAction['confirm']>) {
  return typeof confirm === 'string' ? confirm : confirm.title
}

function confirmDescription(confirm: NonNullable<TableRowAction['confirm']>) {
  return typeof confirm === 'string' ? undefined : confirm.description
}

/**
 * 表格行操作收敛：图标 + Tooltip；超出 maxInline 或 menuOnly 时进「更多」菜单。
 * 先例：公告/告警规则窄屏 Dropdown，桌面图标按钮。
 */
export default function TableRowActions({
  actions,
  maxInline = 3,
  menuOnly = false,
  ariaLabel,
  className,
}: TableRowActionsProps) {
  const { t } = useTranslation()
  const list = actions.filter((a) => a.show !== false)
  if (!list.length) return null

  const inline = menuOnly ? [] : list.slice(0, Math.max(0, maxInline))
  const overflow = menuOnly ? list : list.slice(Math.max(0, maxInline))

  const run = (action: TableRowAction) => {
    if (action.disabled) return
    if (action.confirm) {
      Modal.confirm({
        title: confirmTitle(action.confirm),
        content: confirmDescription(action.confirm),
        okButtonProps: action.danger ? { danger: true } : undefined,
        onOk: () => {
          action.onClick()
        },
      })
      return
    }
    action.onClick()
  }

  const renderInline = (action: TableRowAction) => {
    const btn = (
      <Button
        type="text"
        size="small"
        danger={action.danger}
        disabled={action.disabled}
        loading={action.loading}
        icon={action.icon}
        aria-label={action.label}
        onClick={action.confirm ? undefined : () => action.onClick()}
      />
    )
    const tipped = action.icon ? <Tooltip title={action.label}>{btn}</Tooltip> : btn
    if (action.confirm) {
      return (
        <Popconfirm
          key={action.key}
          title={confirmTitle(action.confirm)}
          description={confirmDescription(action.confirm)}
          onConfirm={() => action.onClick()}
          disabled={action.disabled}
        >
          {tipped}
        </Popconfirm>
      )
    }
    return <span key={action.key}>{tipped}</span>
  }

  const menuItems: MenuProps['items'] = overflow.map((action) => ({
    key: action.key,
    label: action.label,
    icon: action.icon,
    danger: action.danger,
    disabled: action.disabled || action.loading,
    onClick: () => run(action),
  }))

  return (
    <Space size={2} className={['table-actions', 'compact-table-actions', className].filter(Boolean).join(' ')}>
      {inline.map(renderInline)}
      {overflow.length > 0 && (
        <Dropdown trigger={['click']} menu={{ items: menuItems }} placement="bottomRight">
          <Button
            type="text"
            size="small"
            icon={<MoreOutlined />}
            aria-label={ariaLabel ?? t('更多操作')}
          />
        </Dropdown>
      )}
    </Space>
  )
}
