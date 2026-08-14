import type { ReactNode } from 'react'
import { Card, Form, type FormProps } from 'antd'

export type ListFilterFormProps<Values = Record<string, unknown>> = Omit<FormProps<Values>, 'children' | 'layout'> & {
  children: ReactNode
}

/** 列表页统一筛选卡片：保持查询、重置与窄屏换行结构一致。 */
export default function ListFilterForm<Values = Record<string, unknown>>({
  children,
  className,
  ...formProps
}: ListFilterFormProps<Values>) {
  return (
    <Card className="list-filter-card" bordered={false}>
      <Form<Values>
        {...formProps}
        layout="inline"
        className={['list-filter-form', className].filter(Boolean).join(' ')}
      >
        {children}
      </Form>
    </Card>
  )
}
