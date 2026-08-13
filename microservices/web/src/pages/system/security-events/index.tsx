import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Select, Space, Table, Tag } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getSecurityEventList, type SecurityEvent } from '@/api/system/security-events'
import GlassEmpty from '@/components/GlassEmpty'
import ListPageShell from '@/components/ListPageShell'
import TableToolbar from '@/components/TableToolbar'
import { formatDateTime } from '@/utils/format'
import { useTableQuery } from '@/hooks/useTableQuery'

const RULE_LABELS: Record<string, string> = {
  high_volume_write: '写入操作激增',
  permission_storm: '权限变更风暴',
  failure_burst: '失败操作激增',
}

const SEVERITY_META: Record<string, { label: string; color: string }> = {
  info: { label: '提示', color: 'blue' },
  warning: { label: '警告', color: 'gold' },
  critical: { label: '严重', color: 'red' },
}

export default function SecurityEventsPage() {
  const { t } = useTranslation()
  const [params, setParams] = useState<{ page: number; page_size: number; severity?: string }>({
    page: 1,
    page_size: 10,
  })

  const fetchList = useCallback(async (p: typeof params) => {
    const res = await getSecurityEventList(p)
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
  })

  const columns: ColumnsType<SecurityEvent> = [
    {
      title: t('级别'),
      dataIndex: 'severity',
      width: 90,
      render: (v: string) => {
        const meta = SEVERITY_META[v] ?? SEVERITY_META.info
        return <Tag color={meta.color}>{t(meta.label)}</Tag>
      },
    },
    {
      title: t('规则'),
      dataIndex: 'rule',
      width: 140,
      render: (v: string) => (RULE_LABELS[v] ? t(RULE_LABELS[v]) : v),
    },
    { title: t('操作者'), dataIndex: 'actor_id', width: 160, ellipsis: true },
    { title: t('摘要'), dataIndex: 'summary', ellipsis: true },
    {
      title: t('通知'),
      dataIndex: 'notified_at',
      width: 100,
      render: (v: string | null) =>
        v ? <Tag color="green">{t('已通知')}</Tag> : <Tag>{t('未通知')}</Tag>,
    },
    {
      title: t('发生时间'),
      dataIndex: 'occurred_at',
      width: 180,
      render: (v: string) => formatDateTime(v),
    },
  ]

  return (
    <ListPageShell
      className="security-events-page"
      toolbar={(
        <TableToolbar
        title="安全事件"
        total={total}
        extra={
          <Space>
            <Select
              allowClear
              placeholder={t('全部级别')}
              style={{ width: 140 }}
              onChange={(v?: string) => setParams({ ...params, page: 1, severity: v })}
              options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: t(meta.label) }))}
            />
            <Button icon={<ReloadOutlined />} onClick={() => void reload()} loading={loading}>
              {t('刷新')}
            </Button>
          </Space>
        }
        />
      )}
      >
      <Table
        rowKey="id"
        className="list-table"
        loading={loading}
        dataSource={list}
        columns={columns}
        scroll={{ x: 960 }}
        locale={{ emptyText: <GlassEmpty text="暂无安全事件" compact /> }}
        pagination={{
          total,
          current: params.page,
          pageSize: params.page_size,
          showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setParams({ ...params, page, page_size }),
        }}
      />
    </ListPageShell>
  )
}
