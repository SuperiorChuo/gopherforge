import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Form, InputNumber, Popconfirm, Row, Select, Space, Spin, Switch, Table, Tabs, Tag } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, SafetyCertificateOutlined, SaveOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getSettingList, upsertSetting } from '@/api/system/setting'
import { getBlockedIPs, unblockIP, type BlockedIPEntry } from '@/api/system/login-security'
import { getLoginRiskEvents, processLoginRiskEvent, type LoginRiskEvent } from '@/api/system/login-risk-events'
import GlassEmpty from '@/components/GlassEmpty'
import TableToolbar from '@/components/TableToolbar'
import { formatDateTime } from '@/utils/format'
import './styles.css'

const REASON_LABELS: Record<string, string> = {
  new_ip: '新 IP',
  new_device: '新设备',
}

export default function LoginSecurityPage() {
  const { t } = useTranslation()
  const [form] = Form.useForm()

  // ── 风控参数 ──
  const [configLoading, setConfigLoading] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const loadConfig = useCallback(async () => {
    setConfigLoading(true)
    try {
      // 用列表接口避免 GET /system-settings/:key 对未创建行返回 404（会弹全局错误）
      const list = await getSettingList('security')
      const policy = list.find((s) => s.setting_key === 'security.policy')
      if (policy) {
        form.setFieldsValue(policy.value_json ?? {})
      } else {
        // 设置行不存在（首次使用）：用后端默认值占位，保存即创建
        form.setFieldsValue({
          login_limit_max_failures: 5,
          login_limit_window_minutes: 15,
          login_limit_lock_minutes: 30,
          login_ip_shield_max_failures: 30,
          login_ip_shield_window_minutes: 10,
          login_ip_shield_block_minutes: 10,
          login_alert_enabled: true,
        })
      }
    } finally {
      setConfigLoading(false)
    }
  }, [form])
  useEffect(() => { void loadConfig() }, [loadConfig])
  const saveConfig = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setConfigSaving(true)
    try {
      let existing: Record<string, unknown> = {}
      try {
        const list = await getSettingList('security')
        existing = list.find((s) => s.setting_key === 'security.policy')?.value_json ?? {}
      } catch { /* 列表读取失败，按无既有行处理 */ }
      await upsertSetting('security.policy', { ...existing, ...values })
      message.success(t('保存成功'))
    } finally {
      setConfigSaving(false)
    }
  }

  // ── 被屏蔽 IP ──
  const [blocked, setBlocked] = useState<BlockedIPEntry[]>([])
  const [blockedLoading, setBlockedLoading] = useState(false)
  const loadBlocked = useCallback(async () => {
    setBlockedLoading(true)
    try {
      const res = await getBlockedIPs()
      setBlocked(res.items ?? [])
    } catch {
      setBlocked([])
    } finally {
      setBlockedLoading(false)
    }
  }, [])
  useEffect(() => { void loadBlocked() }, [loadBlocked])
  const doUnblock = async (ip: string) => {
    await unblockIP(ip)
    void loadBlocked()
  }

  // ── 异常登录事件 ──
  const [events, setEvents] = useState<LoginRiskEvent[]>([])
  const [eventTotal, setEventTotal] = useState(0)
  const [eventLoading, setEventLoading] = useState(false)
  const [eventParams, setEventParams] = useState<{ page: number; page_size: number; reason?: string; processed?: string }>({
    page: 1,
    page_size: 10,
  })
  const fetchEvents = useCallback(async (p: typeof eventParams) => {
    setEventLoading(true)
    try {
      const res = await getLoginRiskEvents(p)
      setEvents(res.list ?? [])
      setEventTotal(res.total ?? 0)
    } catch {
      setEvents([])
    } finally {
      setEventLoading(false)
    }
  }, [])
  useEffect(() => { void fetchEvents(eventParams) }, [eventParams, fetchEvents])
  const doProcess = async (id: number) => {
    await processLoginRiskEvent(id)
    void fetchEvents(eventParams)
  }

  const blockedColumns: ColumnsType<BlockedIPEntry> = [
    { title: t('IP'), dataIndex: 'ip', width: 220, render: (v: string) => <span className="cell-mono">{v}</span> },
    {
      title: t('剩余屏蔽时间'),
      dataIndex: 'ttl_seconds',
      width: 160,
      render: (v: number) => (v > 0 ? `${Math.ceil(v / 60)} 分钟` : t('已到期')),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 100,
      render: (_, row) => (
        <Popconfirm title={t('确认解封该 IP？')} onConfirm={() => doUnblock(row.ip)}>
          <Button type="link" size="small">{t('解封')}</Button>
        </Popconfirm>
      ),
    },
  ]

  const eventColumns: ColumnsType<LoginRiskEvent> = [
    { title: t('时间'), dataIndex: 'created_at', width: 170, render: (v: string) => formatDateTime(v) },
    { title: t('用户'), dataIndex: 'username', width: 120, ellipsis: true },
    {
      title: t('类型'),
      dataIndex: 'reason',
      width: 90,
      render: (v: string) => REASON_LABELS[v] ?? v,
    },
    {
      title: t('IP / 设备'),
      dataIndex: 'ip',
      width: 200,
      render: (v: string, row) => (
        <Space size={6}>
          <span className="cell-mono">{v}</span>
          {row.device_id ? (
            <span className="cell-mono cell-muted" title={row.device_id}>{row.device_id.slice(0, 8)}…</span>
          ) : null}
        </Space>
      ),
    },
    {
      title: t('提醒'),
      dataIndex: 'alerted',
      width: 90,
      render: (v: boolean) => (v ? <Tag color="green">{t('已提醒')}</Tag> : <Tag>{t('未提醒')}</Tag>),
    },
    {
      title: t('处理'),
      dataIndex: 'processed',
      width: 110,
      render: (v: boolean, row) =>
        v ? (
          <Tag color="blue">{t('已处理')}</Tag>
        ) : (
          <Button type="link" size="small" onClick={() => doProcess(row.id)}>{t('标记处理')}</Button>
        ),
    },
  ]

  return (
    <Card
      className="glass-rise"
      title={
        <Space>
          <SafetyCertificateOutlined className="card-title-icon" /> {t('登录安全')}
        </Space>
      }
    >
      <Tabs
        defaultActiveKey="config"
        items={[
          {
            key: 'config',
            label: t('风控参数'),
            children: (
              <Spin spinning={configLoading}>
                <Form form={form} layout="vertical">
                  <div className="login-security-section-title">{t('账号锁定')}</div>
                  <Row gutter={24}>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_limit_max_failures" label={t('登录失败锁定阈值（次）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_limit_window_minutes" label={t('失败统计窗口（分钟）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_limit_lock_minutes" label={t('锁定时长（分钟）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                  </Row>

                  <div className="login-security-section-title">{t('IP 级失败护盾')}</div>
                  <Row gutter={24}>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_ip_shield_max_failures" label={t('单 IP 失败阈值（次）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_ip_shield_window_minutes" label={t('失败统计窗口（分钟）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_ip_shield_block_minutes" label={t('屏蔽时长（分钟）')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                  </Row>

                  <div className="login-security-section-title">{t('新设备 / 新 IP 提醒')}</div>
                  <Row gutter={24}>
                    <Col xs={24} sm={8}>
                      <Form.Item name="login_alert_enabled" label={t('启用登录提醒')} valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Button type="primary" icon={<SaveOutlined />} loading={configSaving} onClick={saveConfig}>
                    {t('保存配置')}
                  </Button>
                </Form>
              </Spin>
            ),
          },
          {
            key: 'blocked',
            label: t('被屏蔽 IP'),
            children: (
              <>
                <TableToolbar
                  title="被屏蔽 IP"
                  total={blocked.length}
                  icon={<SafetyCertificateOutlined />}
                  extra={
                    <Button icon={<ReloadOutlined />} onClick={loadBlocked} loading={blockedLoading}>
                      {t('刷新')}
                    </Button>
                  }
                />
                <Table
                  rowKey="ip"
                  className="list-table"
                  loading={blockedLoading}
                  dataSource={blocked}
                  columns={blockedColumns}
                  pagination={false}
                  locale={{ emptyText: <GlassEmpty text={t('当前没有被屏蔽的 IP')} compact /> }}
                 scroll={{ x: 960 }} />
              </>
            ),
          },
          {
            key: 'events',
            label: t('异常登录事件'),
            children: (
              <>
                <TableToolbar
                  title="异常登录事件"
                  total={eventTotal}
                  icon={<SafetyCertificateOutlined />}
                  extra={
                    <Space>
                      <Select
                        allowClear
                        placeholder={t('全部类型')}
                        style={{ width: 120 }}
                        onChange={(v?: string) => setEventParams({ ...eventParams, page: 1, reason: v })}
                        options={Object.entries(REASON_LABELS).map(([value, label]) => ({ value, label }))}
                      />
                      <Select
                        allowClear
                        placeholder={t('全部处理状态')}
                        style={{ width: 130 }}
                        onChange={(v?: string) => setEventParams({ ...eventParams, page: 1, processed: v })}
                        options={[
                          { value: 'false', label: t('未处理') },
                          { value: 'true', label: t('已处理') },
                        ]}
                      />
                      <Button icon={<ReloadOutlined />} onClick={() => fetchEvents(eventParams)} loading={eventLoading}>
                        {t('刷新')}
                      </Button>
                    </Space>
                  }
                />
                <Table
                  rowKey="id"
                  className="list-table"
                  loading={eventLoading}
                  dataSource={events}
                  columns={eventColumns}
                  locale={{ emptyText: <GlassEmpty text={t('暂无异常登录事件')} compact /> }}
                  pagination={{
                    total: eventTotal,
                    current: eventParams.page,
                    pageSize: eventParams.page_size,
                    showSizeChanger: true,
                    showTotal: (t2) => `共 ${t2} 条`,
                    onChange: (page, page_size) => setEventParams({ ...eventParams, page, page_size }),
                  }}
                 scroll={{ x: 960 }} />
              </>
            ),
          },
        ]}
      />
    </Card>
  )
}
