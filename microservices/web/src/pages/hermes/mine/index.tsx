import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Input, Popconfirm, Result, Segmented, Space, Steps, Table, Tag, Tooltip } from 'antd'
import {
  ApiOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  DeleteOutlined,
  DownloadOutlined,
  ReloadOutlined,
  RobotOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  approveMyHermesDevice,
  getMyHermes,
  getMyHermesUsage,
  installMyHermesSkill,
  listMyHermesSkillCatalog,
  revokeMyHermesDevice,
  uninstallMyHermesSkill,
  type HermesArtifact,
  type HermesDevice,
  type HermesMe,
  type HermesMyUsage,
  type HermesSkillCatalogItem,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid from '@/components/common/StatsGrid'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { stateLabels, stateTone } from '../_shared'

const CONNECTION_LABELS: Record<HermesDevice['connection_status'], string> = {
  never: '从未连接',
  online: '在线',
  offline: '离线',
}

const HEALTH_LABELS: Record<HermesDevice['health_status'], string> = {
  unknown: '未知',
  healthy: '正常',
  degraded: '降级',
  unhealthy: '异常',
  policy_drift: '策略漂移',
}

const desiredStateLabel = (state: HermesDevice['desired_state']) =>
  state === 'unknown' ? '未解析' : (stateLabels[state] ?? state)

export default function MyHermesPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [dataLoaded, setDataLoaded] = useState(false)
  const [dataError, setDataError] = useState(false)
  const [approving, setApproving] = useState(false)
  const [userCode, setUserCode] = useState('')
  const [data, setData] = useState<HermesMe>({ entitlement: null, devices: [], downloads: [] })
  // M6 技能市场
  const [catalog, setCatalog] = useState<HermesSkillCatalogItem[]>([])
  const [catalogLoading, setCatalogLoading] = useState(false)
  // M6 我的用量
  const [usageDays, setUsageDays] = useState<7 | 30>(7)
  const [usage, setUsage] = useState<HermesMyUsage | null>(null)
  const [usageLoading, setUsageLoading] = useState(true)
  const [usageError, setUsageError] = useState(false)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const result = await getMyHermes()
      // 服务端已保证数组字段不为 null，这里仍兜一层：整页渲染不该被单个字段的
      // 空值打穿（历史上 downloads 为 null 时本页直接白屏，而这是入职必经页）。
      setData({
        entitlement: result.entitlement ?? null,
        devices: result.devices ?? [],
        downloads: result.downloads ?? [],
      })
      setDataLoaded(true)
      setDataError(false)
    } catch {
      // 保留上次成功数据；首次失败由页面明确显示不可用，不伪装成未开通。
      setDataError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadCatalog = useCallback(async () => {
    setCatalogLoading(true)
    try {
      const result = await listMyHermesSkillCatalog({ page: 1, page_size: 100 })
      setCatalog(result.list ?? [])
    } catch {
      // Request layer already reports the server message.
    } finally {
      setCatalogLoading(false)
    }
  }, [])

  const loadUsage = useCallback(async () => {
    setUsageLoading(true)
    try {
      setUsage(await getMyHermesUsage(usageDays))
      setUsageError(false)
    } catch {
      // 保留上次成功用量；首次失败不展示假零值。
      setUsageError(true)
    } finally {
      setUsageLoading(false)
    }
  }, [usageDays])

  useEffect(() => {
    void loadData()
    void loadCatalog()
  }, [loadData, loadCatalog])

  useEffect(() => {
    void loadUsage()
  }, [loadUsage])

  const installSkill = async (item: HermesSkillCatalogItem) => {
    try {
      await installMyHermesSkill(item.id)
      message.success(t('技能已安装，设备下次同步生效'))
      await Promise.all([loadCatalog(), loadData()])
    } catch {
      // Request layer already reports the server message.
    }
  }

  const uninstallSkill = async (item: HermesSkillCatalogItem) => {
    try {
      await uninstallMyHermesSkill(item.id)
      message.success(t('技能已卸载，设备下次同步生效'))
      await Promise.all([loadCatalog(), loadData()])
    } catch {
      // Request layer already reports the server message.
    }
  }

  const managerDownloads = useMemo(
    () => data.downloads.filter((artifact) => artifact.kind === 'manager' && artifact.status === 'approved'),
    [data.downloads],
  )

  const approveDevice = async () => {
    const normalized = userCode.trim().toUpperCase()
    if (!/^[A-Z0-9]{4}-?[A-Z0-9]{4}$/.test(normalized)) {
      message.warning(t('请输入 8 位设备代码'))
      return
    }
    setApproving(true)
    try {
      await approveMyHermesDevice(normalized)
      message.success(t('设备绑定已确认'))
      setUserCode('')
      await loadData()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setApproving(false)
    }
  }

  const revokeDevice = async (deviceId: string) => {
    try {
      await revokeMyHermesDevice(deviceId)
      message.success(t('解绑请求已下发，等待设备确认'))
      await loadData()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const deviceColumns: ColumnsType<HermesDevice> = [
    {
      title: t('设备'),
      dataIndex: 'device_name',
      render: (value: string, record) => (
        <div>
          <div>{value || t('未命名设备')}</div>
          <span className="cell-muted">{record.platform} / {record.arch}</span>
        </div>
      ),
    },
    {
      title: t('期望状态'),
      dataIndex: 'desired_state',
      width: 110,
      render: (value: HermesDevice['desired_state']) => (
        <StatusPill tone={stateTone(value)} label={desiredStateLabel(value)} />
      ),
    },
    {
      title: t('本机状态'),
      dataIndex: 'actual_state',
      width: 110,
      render: (value: string) => <StatusPill tone={stateTone(value)} label={stateLabels[value] ?? value} />,
    },
    {
      title: t('连接'),
      dataIndex: 'connection_status',
      width: 100,
      render: (value: HermesDevice['connection_status']) => (
        <StatusPill
          tone={value === 'online' ? 'success' : value === 'offline' ? 'muted' : 'info'}
          label={CONNECTION_LABELS[value]}
        />
      ),
    },
    {
      title: t('健康'),
      dataIndex: 'health_status',
      width: 120,
      render: (value: HermesDevice['health_status']) => (
        <StatusPill tone={stateTone(value)} label={HEALTH_LABELS[value]} pulse={false} />
      ),
    },
    { title: 'Manager', dataIndex: 'manager_version', width: 110, render: (value: string) => value || '—' },
    { title: 'Hermes', dataIndex: 'hermes_version', width: 120, render: (value: string) => value || t('未安装') },
    { title: t('策略'), dataIndex: 'applied_policy_revision', width: 80, render: (value: number) => value || t('未应用') },
    { title: t('最后在线'), dataIndex: 'last_seen_at', width: 160, render: (value?: string) => value ? formatDateTime(value) : t('从未') },
    {
      title: t('操作'),
      width: 70,
      render: (_, record) => record.revoked_at ? null : (
        <Popconfirm title={t('解绑此设备？')} onConfirm={() => void revokeDevice(record.device_id)}>
          <Tooltip title={t('解绑设备')}><Button type="text" danger icon={<DeleteOutlined />} /></Tooltip>
        </Popconfirm>
      ),
    },
  ]

  const downloadColumns: ColumnsType<HermesArtifact> = [
    { title: t('平台'), width: 150, render: (_, record) => `${record.platform} / ${record.arch}` },
    { title: t('版本'), dataIndex: 'version', width: 130 },
    { title: 'SHA-256', dataIndex: 'sha256', ellipsis: true, render: (value: string) => <span className="cell-mono">{value}</span> },
    {
      title: t('下载'),
      width: 70,
      render: (_, record) => (
        <Tooltip title={t('下载安装包')}>
          <Button type="link" icon={<DownloadOutlined />} href={record.source_url} target="_blank" rel="noreferrer" />
        </Tooltip>
      ),
    },
  ]

  const enabled = data.entitlement?.enabled === true
  const activeDevices = useMemo(() => data.devices.filter((d) => !d.revoked_at), [data.devices])
  const atDeviceLimit = !!data.entitlement && activeDevices.length >= data.entitlement.device_limit

  // 设备状态里值得员工自己看懂的几种，翻译成人话
  const deviceHint = useMemo(() => {
    if (activeDevices.length === 0) return null
    const drift = activeDevices.find((d) => d.health_status === 'policy_drift')
    if (drift) return { type: 'warning' as const, text: t('检测到本机配置与企业策略不一致，Hermes 会在下次同步时自动纠正。') }
    const errored = activeDevices.find((d) => d.actual_state === 'error' || d.health_status === 'unhealthy')
    if (errored) {
      return {
        type: 'error' as const,
        text: t('设备「{{name}}」运行异常{{code}}，请联系管理员。', {
          name: errored.device_name || errored.device_id,
          code: errored.last_error_code ? `（${errored.last_error_code}）` : '',
        }),
      }
    }
    const installing = activeDevices.find((d) => d.actual_state === 'installing' || d.actual_state === 'updating')
    if (installing) return { type: 'info' as const, text: t('Hermes 正在安装或更新，完成后状态会自动变为「运行中」。') }
    return null
  }, [activeDevices, t])

  return (
    <div className="page-list">
      <TableToolbar
        title="工作台"
        icon={<RobotOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={!dataLoaded ? t('正在加载开通状态') : enabled ? t('已开通 · {{n}} 台活跃设备', { n: activeDevices.length }) : t('尚未开通')}
        extra={<Tooltip title={t('刷新')}><Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadData()} /></Tooltip>}
      />

      {dataError && dataLoaded && (
        <Alert
          type="warning"
          showIcon
          message={t('刷新失败，当前显示上次成功数据')}
          action={<Button size="small" onClick={() => void loadData()}>{t('重试')}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      {!dataLoaded ? (
        <Card className="list-main-card" loading={loading}>
          {dataError && (
            <Result
              status="warning"
              title={t('Hermes 状态暂不可用')}
              subTitle={t('无法确认当前开通与设备状态，请稍后重试。')}
              extra={<Button type="primary" onClick={() => void loadData()}>{t('重新加载')}</Button>}
            />
          )}
        </Card>
      ) : !enabled ? (
        <Card className="list-main-card">
          <Result
            status="403"
            title={t('Hermes 尚未开通')}
            subTitle={t('Hermes 是运行在你自己电脑上的专属助手。开通请联系管理员。')}
          />
        </Card>
      ) : (
        <>
          {deviceHint && (
            <Alert type={deviceHint.type} showIcon message={deviceHint.text} style={{ marginBottom: 16 }} />
          )}

          <Card className="list-main-card" title={<span><ApiOutlined className="card-title-icon" />{t('怎么开始')}</span>}>
            <Steps
              direction="vertical"
              size="small"
              current={activeDevices.length > 0 ? 3 : 0}
              items={[
                {
                  title: t('下载 Manager 安装包'),
                  description: t('在下方「Manager 安装包」里选你电脑对应的平台下载。安装包不含任何密钥，可放心保存。'),
                },
                {
                  title: t('安装并启动'),
                  description: t('安装后首次启动会显示一个 8 位设备代码（XXXX-XXXX 形式）。'),
                },
                {
                  title: t('回到本页确认绑定'),
                  description: t('把设备代码填进下面的输入框点「确认绑定」。代码有效期约 10 分钟，过期就重新启动 Manager 取新的。'),
                },
                {
                  title: t('等待自动安装完成'),
                  description: t('Manager 会按企业策略自动安装 Hermes 并保持更新。你的对话、记忆和本机文件都留在自己电脑上，不会上传。'),
                },
              ]}
            />
          </Card>

          <Card className="list-main-card" title={<span><ApiOutlined className="card-title-icon" />{t('确认设备绑定')}</span>}>
            {atDeviceLimit && (
              <Alert
                type="warning"
                showIcon
                style={{ marginBottom: 12 }}
                message={t('已达设备上限（{{n}} 台）。要绑定新设备，请先解绑一台，或联系管理员调整上限。', { n: data.entitlement?.device_limit })}
              />
            )}
            <Space.Compact style={{ width: '100%', maxWidth: 420 }}>
              <Input
                value={userCode}
                onChange={(event) => setUserCode(event.target.value.toUpperCase())}
                onPressEnter={() => void approveDevice()}
                placeholder="XXXX-XXXX"
                maxLength={9}
                className="cell-mono"
                disabled={atDeviceLimit}
              />
              <Button type="primary" loading={approving} onClick={() => void approveDevice()} disabled={atDeviceLimit}>
                {t('确认绑定')}
              </Button>
            </Space.Compact>
          </Card>

          <Card className="list-main-card" title={t('我的设备')} extra={<span className="cell-muted">{t('展开行可查看这台 Hermes 会什么')}</span>}>
            <Table
              rowKey="device_id"
              className="list-table"
              size="middle"
              loading={loading}
              columns={deviceColumns}
              dataSource={data.devices}
              pagination={false}
              scroll={{ x: 1120 }}
              expandable={{
                rowExpandable: (record) => !!record.capabilities,
                expandedRowRender: (record) => (
                  <Space direction="vertical" size={8} style={{ padding: '4px 0' }}>
                    <div>
                      <span className="cell-muted" style={{ marginRight: 8 }}>{t('技能')}</span>
                      {record.capabilities?.skills.length
                        ? record.capabilities.skills.map((s) => (
                            <Tag key={s.name} color="geekblue">{s.name}@{s.version}</Tag>
                          ))
                        : <span className="cell-muted">{t('暂无（去下方技能市场安装）')}</span>}
                    </div>
                    <div>
                      <span className="cell-muted" style={{ marginRight: 8 }}>{t('工具集')}</span>
                      {record.capabilities?.toolsets.map((tool) => <Tag key={tool}>{tool}</Tag>)}
                    </div>
                    <div>
                      <span className="cell-muted" style={{ marginRight: 8 }}>{t('MCP')}</span>
                      {record.capabilities?.mcp_servers.length
                        ? record.capabilities.mcp_servers.map((name) => (
                            <Tag key={name} color="purple">{name}{t('（企业下发）')}</Tag>
                          ))
                        : <span className="cell-muted">{t('无')}</span>}
                    </div>
                  </Space>
                ),
              }}
              locale={{ emptyText: <GlassEmpty text="暂无已绑定设备" compact /> }}
            />
          </Card>

          <Card className="list-main-card" title={<span><AppstoreOutlined className="card-title-icon" />{t('技能市场')}</span>} extra={<span className="cell-muted">{t('仅显示企业已审核技能')}</span>}>
            <Table
              rowKey="id"
              className="list-table"
              size="middle"
              loading={catalogLoading}
              dataSource={catalog}
              pagination={catalog.length > 10 ? { pageSize: 10, showTotal: (total) => t('共 {{n}} 条', { n: total }) } : false}
              columns={[
                {
                  title: t('技能'),
                  dataIndex: 'display_name',
                  render: (value: string, record: HermesSkillCatalogItem) => (
                    <div>
                      <div>{value || record.name}{record.installed && <Tag color="success" style={{ marginLeft: 8 }}>{t('已安装')}</Tag>}</div>
                      <span className="cell-muted">{record.name}@{record.version}</span>
                    </div>
                  ),
                },
                { title: t('描述'), dataIndex: 'description', ellipsis: true, render: (value: string) => <span className="cell-muted">{value || '—'}</span> },
                {
                  title: t('操作'),
                  width: 88,
                  align: 'center' as const,
                  render: (_, record: HermesSkillCatalogItem) =>
                    record.installed ? (
                      <TableRowActions
                        maxInline={1}
                        actions={[
                          {
                            key: 'uninstall',
                            label: t('卸载'),
                            icon: <DeleteOutlined />,
                            danger: true,
                            confirm: t('卸载此技能？设备下次同步移除。'),
                            onClick: () => { void uninstallSkill(record) },
                          },
                        ]}
                      />
                    ) : (
                      <Button size="small" type="primary" onClick={() => void installSkill(record)}>{t('安装')}</Button>
                    ),
                },
              ]}
              locale={{ emptyText: <GlassEmpty text="暂无已审核技能，等待管理员上架" compact /> }}
            />
          </Card>

          <Card
            className="list-main-card"
            title={<span><BarChartOutlined className="card-title-icon" />{t('我的用量')}</span>}
            loading={usageLoading && (!usage || usage.days !== usageDays)}
            extra={
              <Segmented
                size="small"
                value={usageDays}
                onChange={(value) => setUsageDays(value as 7 | 30)}
                options={[{ label: t('7 天'), value: 7 }, { label: t('30 天'), value: 30 }]}
              />
            }
          >
            {usageError && (
              <Alert
                type="warning"
                showIcon
                message={usage ? t('用量刷新失败，当前显示上次成功的 {{n}} 天数据', { n: usage.days }) : t('模型用量暂不可用')}
                action={<Button size="small" onClick={() => void loadUsage()}>{t('重试')}</Button>}
                style={{ marginBottom: usage ? 16 : 0 }}
              />
            )}
            {usage && (
              <div style={{ marginBottom: usage.daily.length ? 16 : 0 }}>
                <StatsGrid>
                  <MetricCard label={t('模型调用')} value={usage.summary.total_requests.toLocaleString()} />
                  <MetricCard
                    label={t('失败')}
                    value={usage.summary.failed_requests.toLocaleString()}
                    valueClassName={usage.summary.failed_requests ? 'metric-card-value-danger' : undefined}
                  />
                  <MetricCard label="Token" value={usage.summary.total_tokens.toLocaleString()} />
                  <MetricCard label={t('费用')} value={`¥${usage.summary.total_cost.toFixed(2)}`} />
                </StatsGrid>
              </div>
            )}
            {!!usage?.daily.length && (
              <Table
                rowKey="day"
                className="list-table"
                size="small"
                loading={usageLoading}
                dataSource={usage.daily}
                pagination={false}
                columns={[
                  { title: t('日期'), dataIndex: 'day', width: 130 },
                  { title: t('调用'), dataIndex: 'total_requests', width: 90, align: 'right' },
                  { title: t('失败'), dataIndex: 'failed_requests', width: 90, align: 'right' },
                  { title: 'Token', dataIndex: 'total_tokens', width: 120, align: 'right' },
                  { title: t('费用'), dataIndex: 'total_cost', width: 100, align: 'right', render: (value: number) => `¥${(value ?? 0).toFixed(2)}` },
                ]}
              />
            )}
          </Card>

          <Card className="list-main-card" title={t('Manager 安装包')}>
            <Table
              rowKey="id"
              className="list-table"
              size="middle"
              loading={loading}
              columns={downloadColumns}
              dataSource={managerDownloads}
              pagination={false}
              locale={{ emptyText: <GlassEmpty text="暂无已批准安装包" compact /> }}
            />
            {data.entitlement && (
              <Space wrap style={{ marginTop: 14 }}>
                <Tag>{t('设备上限 {{n}}', { n: data.entitlement.device_limit })}</Tag>
                <Tag>{t('策略 {{name}}', { name: data.entitlement.policy_id || t('最新') })}</Tag>
              </Space>
            )}
          </Card>
        </>
      )}
    </div>
  )
}
