import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Space, Button, Segmented, Alert, Tooltip } from 'antd'
import { ReloadOutlined, ExportOutlined, DashboardOutlined } from '@ant-design/icons'

// Grafana 看板嵌入页：monitoring profile 启动后，Grafana(:3003) 开启了匿名
// Viewer + allow_embedding，这里直接 iframe 嵌「呼叫中心」看板（kiosk 模式去
// 掉 Grafana 自身导航）。Prometheus 指标源：control-api /metrics（fscc_*）与
// monitor-service /api/v1/metrics。
// 看板不可达时给出启动指引（monitoring profile 是可选启动项）。

const GRAFANA_PORT = 3003
const DASH_UID = 'fscc-call-center'

const RANGES = [
  { label: '近 1 小时', value: 'from=now-1h&to=now' },
  { label: '近 6 小时', value: 'from=now-6h&to=now' },
  { label: '近 24 小时', value: 'from=now-24h&to=now' },
]

export default function MonitorGrafanaPage() {
  const { t } = useTranslation()
  const [range, setRange] = useState(RANGES[0].value)
  const [nonce, setNonce] = useState(0) // 刷新 iframe 用

  const grafanaBase = useMemo(
    () => `${window.location.protocol}//${window.location.hostname}:${GRAFANA_PORT}`,
    [],
  )
  const embedUrl = `${grafanaBase}/d/${DASH_UID}?kiosk&theme=dark&refresh=10s&${range}&_r=${nonce}`
  const openUrl = `${grafanaBase}/d/${DASH_UID}?${range}`

  return (
    <div className="page-list monitor-grafana-page">
      <Card className="list-main-card" bordered={false} styles={{ body: { padding: 16 } }}>
        <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 12 }} wrap>
          <Space size={10}>
            <DashboardOutlined />
            <b>{t('呼叫中心实时看板')}</b>
            <span className="cell-dim" style={{ fontSize: 12 }}>
              {t('通道并发 · 队列深度 · 坐席状态 · 线路注册 · 接通/放弃（10 秒自动刷新）')}
            </span>
          </Space>
          <Space size={12}>
            <Segmented
              size="small"
              options={RANGES.map((r) => ({ label: t(r.label), value: r.value }))}
              value={range}
              onChange={(v) => setRange(v as string)}
            />
            <Button size="small" icon={<ReloadOutlined />} onClick={() => setNonce((n) => n + 1)}>
              {t('刷新')}
            </Button>
            <Tooltip title={t('在 Grafana 中打开（可编辑面板、查更多指标）')}>
              <Button size="small" icon={<ExportOutlined />} href={openUrl} target="_blank">
                Grafana
              </Button>
            </Tooltip>
          </Space>
        </Space>

        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message={t('看板空白？监控栈是可选组件')}
          description={
            <span className="cell-dim" style={{ fontSize: 12 }}>
              {t('服务器执行')} <code>docker compose --profile compose-fallback --profile monitoring up -d</code>{' '}
              {t('启动 Prometheus(:9090) 与 Grafana(:{{n}}) 后即可展示。', { n: GRAFANA_PORT })}
            </span>
          }
          closable
        />

        <iframe
          key={embedUrl}
          src={embedUrl}
          title={t('呼叫中心 Grafana 看板')}
          style={{
            width: '100%',
            height: 'calc(100vh - 260px)',
            minHeight: 560,
            border: 'none',
            borderRadius: 12,
            background: 'transparent',
          }}
        />
      </Card>
    </div>
  )
}
