// 员工权益：开通/停用 Hermes、分配策略与设备上限。
// 由原 /ai/hermes 单页的「员工权益」Tab 拆出，只拉本页需要的三份数据。
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, InputNumber, Modal, Select, Space, Switch, Table, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined, SafetyCertificateOutlined, TeamOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listHermesEntitlements,
  listHermesPolicies,
  updateHermesEntitlement,
  type HermesEntitlement,
  type HermesPolicy,
} from '@/api/ai/hermes'
import { getUserList } from '@/api/system/user'
import type { SystemUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { usePagination } from '@/hooks/usePagination'
import { useTableQuery } from '@/hooks/useTableQuery'
import { fetchAllHermesPages } from '../admin-contract'
import { useIncrementalList } from '../useIncrementalList'

export default function HermesEntitlementsPage() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [enabledCount, setEnabledCount] = useState(0)
  const [open, setOpen] = useState(false)
  const [form] = Form.useForm()

  const pagination = usePagination({ page: 1, page_size: 20 })
  const params = useMemo(
    () => ({ page: pagination.page, page_size: pagination.page_size }),
    [pagination.page, pagination.page_size],
  )
  const fetcher = useCallback(async (p: typeof params) => {
    const result = await listHermesEntitlements(p)
    return { list: result.list ?? [], total: result.total ?? 0 }
  }, [])
  const { list: entitlements, total, loading, reload } = useTableQuery({ params, fetcher })

  // 工具栏聚合：enabled 数来自全量统计（逐页真实拉取，不放大 page_size 伪装全量）
  const loadStats = useCallback(async () => {
    try {
      const all = await fetchAllHermesPages<HermesEntitlement>((page, pageSize) =>
        listHermesEntitlements({ page, page_size: pageSize }))
      setEnabledCount(all.filter((item) => item.enabled).length)
    } catch {
      // 统计失败保留上次值；请求层已提示服务端消息
    }
  }, [])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  // 辅助选择器：策略按页追加；用户远程搜索 + 按页追加（域外契约独立处理）
  const policies = useIncrementalList<HermesPolicy>(
    useCallback((page, pageSize) => listHermesPolicies({ page, page_size: pageSize }), []),
  )
  const users = useIncrementalList<SystemUser>(
    useCallback((page, pageSize, keyword) => getUserList({ page, page_size: pageSize, keyword }), []),
  )

  const openModal = (record?: HermesEntitlement) => {
    form.setFieldsValue({
      user_id: record?.user_id,
      enabled: record?.enabled ?? true,
      policy_id: record?.policy_id || 0,
      device_limit: record?.device_limit ?? 1,
    })
    setOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSaving(true)
    try {
      await updateHermesEntitlement(values.user_id, {
        enabled: values.enabled,
        policy_id: values.policy_id ?? 0,
        device_limit: values.device_limit,
      })
      message.success(t('Hermes 权益已更新'))
      setOpen(false)
      void reload()
      void loadStats()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (record: HermesEntitlement, enabled: boolean) => {
    try {
      await updateHermesEntitlement(record.user_id, {
        enabled,
        policy_id: record.policy_id,
        device_limit: record.device_limit,
      })
      message.success(enabled ? t('已开通 Hermes') : t('已停用 Hermes'))
      void reload()
      void loadStats()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const columns: ColumnsType<HermesEntitlement> = [
    {
      title: t('员工'),
      key: 'user',
      render: (_, record) => (
        <div>
          <div>{record.nickname || record.username || t('用户 {{n}}', { n: record.user_id })}</div>
          <span className="cell-muted">{record.username || `ID ${record.user_id}`}</span>
        </div>
      ),
    },
    {
      title: t('权益'),
      dataIndex: 'enabled',
      width: 100,
      render: (enabled: boolean, record) => (
        <Switch size="small" checked={enabled} onChange={(checked) => void toggle(record, checked)} />
      ),
    },
    { title: t('策略'), dataIndex: 'policy_id', width: 100, render: (value: number) => value || t('最新') },
    {
      title: t('设备'),
      width: 110,
      render: (_, record) => `${record.active_device_count ?? 0} / ${record.device_limit}`,
    },
    {
      title: t('最后在线'),
      dataIndex: 'last_seen_at',
      width: 160,
      render: (value?: string) => (value ? formatDateTime(value) : t('从未')),
    },
    {
      title: t('操作'),
      width: 70,
      render: (_, record) => (
        <Tooltip title={t('编辑权益')}>
          <Button type="text" icon={<SafetyCertificateOutlined />} onClick={() => openModal(record)} />
        </Tooltip>
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-entitlements-page"
      toolbar={
      <TableToolbar
        title="实例管理"
        total={enabledCount}
        icon={<TeamOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={t('共 {{n}} 个员工实例配额', { n: total })}
        extra={
          <Space>
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void loadStats(); void policies.reload(); void users.reload() }} loading={loading} />
            </Tooltip>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>
              {t('开通员工')}
            </Button>
          </Space>
        }
      />}
    >

        <Table
          rowKey="id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={entitlements}
          scroll={{ x: 760 }}
          locale={{ emptyText: <GlassEmpty text="暂无 Hermes 权益" compact /> }}
          pagination={{
            total,
            current: pagination.page,
            pageSize: pagination.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => pagination.setPage(page, page_size),
          }}
        />

      <Modal
        title={t('Hermes 员工权益')}
        open={open}
        onOk={() => void submit()}
        onCancel={() => setOpen(false)}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="user_id" label={t('员工')} rules={[{ required: true, message: t('请选择员工') }]}>
            <Select
              showSearch
              filterOption={false}
              onSearch={(keyword) => void users.reload(keyword)}
              onPopupScroll={users.loadMore}
              loading={users.loading}
              options={users.items.map((user) => ({
                value: user.id,
                label: `${user.nickname || user.username} (${user.username})`,
              }))}
            />
          </Form.Item>
          <Form.Item name="enabled" label={t('开通状态')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="policy_id" label={t('策略')}>
            <Select
              onPopupScroll={policies.loadMore}
              options={[
                { value: 0, label: t('自动使用最新策略') },
                ...policies.items.map((policy) => ({ value: policy.id, label: `R${policy.revision} · ${policy.name}` })),
              ]}
            />
          </Form.Item>
          <Form.Item name="device_limit" label={t('活跃设备上限')} rules={[{ required: true }]}>
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
