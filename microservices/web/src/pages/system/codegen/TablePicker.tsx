import { useMemo, useState } from 'react'
import { Button, Input, Space, Table, Tag, Tooltip } from 'antd'
import { ReloadOutlined, SearchOutlined, SettingOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { CodegenTable } from '@/api/system/codegen'
import GlassEmpty from '@/components/common/GlassEmpty'

type Props = {
  tables: CodegenTable[]
  loading: boolean
  onRefresh: () => void
  onConfigure: (table: CodegenTable) => void
}

export default function TablePicker({ tables, loading, onRefresh, onConfigure }: Props) {
  const [keyword, setKeyword] = useState('')
  const filtered = useMemo(() => {
    const value = keyword.trim().toLowerCase()
    if (!value) return tables
    return tables.filter((table) => `${table.name} ${table.comment}`.toLowerCase().includes(value))
  }, [keyword, tables])

  const columns: ColumnsType<CodegenTable> = [
    {
      title: '数据表',
      dataIndex: 'name',
      render: (name: string, row) => (
        <Space size={8} wrap>
          <span style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{name}</span>
          {row.comment && <span style={{ color: 'var(--text-secondary)' }}>{row.comment}</span>}
        </Space>
      ),
    },
    { title: '主键', dataIndex: 'primary_key', width: 130, responsive: ['sm'], render: (value: string) => <Tag>{value}</Tag> },
    { title: '字段', dataIndex: 'column_count', width: 90, align: 'right', responsive: ['md'] },
    { title: '关系', dataIndex: 'relation_count', width: 90, align: 'right', responsive: ['lg'] },
    {
      title: '操作',
      width: 96,
      align: 'right',
      render: (_, row) => (
        <Button icon={<SettingOutlined />} size="small" onClick={() => onConfigure(row)}>
          配置
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索表名或注释"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          style={{ width: 320, maxWidth: '100%' }}
        />
        <Tooltip title="刷新表列表">
          <Button aria-label="刷新表列表" icon={<ReloadOutlined />} onClick={onRefresh} loading={loading} />
        </Tooltip>
      </Space>
      <Table
        rowKey="name"
        size="small"
        columns={columns}
        dataSource={filtered}
        loading={loading}
        pagination={{ pageSize: 15, showSizeChanger: false, hideOnSinglePage: true }}
        locale={{ emptyText: <GlassEmpty text={keyword ? '没有匹配的数据表' : '暂无可生成的数据表'} compact /> }}
      />
    </>
  )
}
