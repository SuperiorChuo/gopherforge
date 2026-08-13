import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Input, Select, Form, Descriptions, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getAuditLogList, type AuditLog, type AuditLogListResult } from '@/api/system/audit-log'
import EntityDetailDrawer from '@/components/EntityDetailDrawer'
import ListFilterForm from '@/components/ListFilterForm'
import ListPageShell from '@/components/ListPageShell'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import './styles.css'

interface SearchParams {
  keyword?: string
  action?: string
  target_type?: string
  page: number
  page_size: number
}

function JsonBlock({ title, data }: { title: string; data?: Record<string, unknown> }) {
  if (!data || Object.keys(data).length === 0) return null
  return (
    <div className="log-detail-block">
      <div className="log-detail-block-title">{title}</div>
      <pre>{JSON.stringify(data, null, 2)}</pre>
    </div>
  )
}

export default function AuditLogPage() {
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [facets, setFacets] = useState<AuditLogListResult['facets'] | null>(null)
  const detailModal = useCrudModal<AuditLog>()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()

  const fetchList = useCallback(async (p: SearchParams) => {
    const res = await getAuditLogList(p)
    if (res.facets) setFacets(res.facets)
    return {
      list: res.items ?? [],
      total: res.pagination?.total ?? 0,
    }
  }, [])
  const onLoadError = useCallback(() => message.error(t('获取审计日志失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onLoadError,
  })

  const handleSearch = (values: { keyword?: string; action?: string; target_type?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const columns: ColumnsType<AuditLog> = [
    { title: 'ID', dataIndex: 'id', width: 70, responsive: ['lg'] },
    {
      title: t('操作者'),
      dataIndex: 'actor_id',
      width: 160,
      responsive: ['sm'],
      render: (v: string, record) => (
        <div className="audit-actor-cell">
          <Tag variant="filled" className="audit-actor-type">{record.actor_type}</Tag>
          <Tooltip title={v}>
            <span className="cell-mono audit-actor-id">{v || '—'}</span>
          </Tooltip>
        </div>
      ),
    },
    {
      title: t('动作'),
      dataIndex: 'action',
      width: 200,
      render: (v: string) => (
        <Tooltip title={v}>
          <Tag color="geekblue" variant="filled" className="cell-mono audit-action-tag">{v}</Tag>
        </Tooltip>
      ),
    },
    {
      title: t('目标'),
      dataIndex: 'target_type',
      width: 220,
      responsive: ['md'],
      render: (v: string, record) => {
        const target = `${v}#${record.target_id}`
        return (
          <Tooltip title={target}>
            <span className="cell-mono cell-dim audit-target-cell">{target}</span>
          </Tooltip>
        )
      },
    },
    { title: t('摘要'), dataIndex: 'summary', width: 300, ellipsis: true, responsive: ['lg'] },
    { title: t('时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['md'] },
    {
      title: t('操作'),
      width: 64,
      fixed: 'right',
      render: (_, record) => (
        <Tooltip title={t('查看详情')}>
          <Button
            type="text"
            size="small"
            className="audit-row-action"
            aria-label={t('查看审计日志详情')}
            icon={<EyeOutlined />}
            onClick={() => detailModal.openEdit(record)}
          />
        </Tooltip>
      ),
    },
  ]

  return (
    <ListPageShell
      className="audit-log-page"
      filter={(
        <ListFilterForm
          form={searchForm}
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索关键字')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="action">
            <Select placeholder={t('动作')} style={{ width: 180 }} allowClear showSearch>
              {(facets?.actions ?? []).map((a) => (
                <Select.Option key={a} value={a}>{a}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="target_type">
            <Select placeholder={t('目标类型')} style={{ width: 150 }} allowClear showSearch>
              {(facets?.target_types ?? []).map((t) => (
                <Select.Option key={t} value={t}>{t}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      )}
      toolbar={(
        <TableToolbar
          title="审计日志"
          total={total}
          extra={<Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>}
        />
      )}
    >
      <Table
          rowKey="id"
          className="list-table audit-log-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无审计记录" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
      />

      <EntityDetailDrawer
        title={t('审计详情')}
        open={detailModal.open}
        onClose={detailModal.close}
        width="min(720px, 100vw)"
      >
        {detailModal.record && (
          <div className="audit-detail-content">
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label="ID">{detailModal.record.id}</Descriptions.Item>
              <Descriptions.Item label={t('时间')}>{formatDateTime(detailModal.record.created_at)}</Descriptions.Item>
              <Descriptions.Item label={t('操作者')}>
                <Tag variant="filled">{detailModal.record.actor_type}</Tag>
                <span className="cell-mono audit-detail-long">{detailModal.record.actor_id}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('动作')}>
                <Tag color="geekblue" variant="filled" className="cell-mono audit-detail-tag">{detailModal.record.action}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label={t('目标')} span={2}>
                <span className="cell-mono audit-detail-long">{detailModal.record.target_type}#{detailModal.record.target_id}</span>
              </Descriptions.Item>
              {detailModal.record.summary && (
                <Descriptions.Item label={t('摘要')} span={2}>{detailModal.record.summary}</Descriptions.Item>
              )}
            </Descriptions>
            <JsonBlock title={t('变更前 (before)')} data={detailModal.record.before} />
            <JsonBlock title={t('变更后 (after)')} data={detailModal.record.after} />
          </div>
        )}
      </EntityDetailDrawer>
    </ListPageShell>
  )
}
