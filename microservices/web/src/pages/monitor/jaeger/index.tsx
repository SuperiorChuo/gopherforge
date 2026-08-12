import { useMemo, useState } from 'react'
import { Card, Space, Button, Tooltip } from 'antd'
import { ReloadOutlined, ExportOutlined, NodeIndexOutlined } from '@ant-design/icons'

const JAEGER_PATH = '/jaeger'

export default function MonitorJaegerPage() {
  const [nonce, setNonce] = useState(0)
  const embedUrl = useMemo(() => `${JAEGER_PATH}/search?_r=${nonce}`, [nonce])

  return (
    <div className="page-list monitor-jaeger-page">
      <Card className="list-main-card" bordered={false} styles={{ body: { padding: 16 } }}>
        <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 12 }} wrap>
          <Space size={10}>
            <NodeIndexOutlined />
            <b>链路追踪</b>
            <span className="cell-dim" style={{ fontSize: 12 }}>
              OpenTelemetry · 请求级耗时分布 · 跨服务调用链
            </span>
          </Space>
          <Space size={12}>
            <Button size="small" icon={<ReloadOutlined />} onClick={() => setNonce((n) => n + 1)}>
              刷新
            </Button>
            <Tooltip title="在 Jaeger 中打开（可搜索、对比 trace）">
              <Button size="small" icon={<ExportOutlined />} href={JAEGER_PATH} target="_blank">
                Jaeger
              </Button>
            </Tooltip>
          </Space>
        </Space>
        <iframe
          key={embedUrl}
          src={embedUrl}
          title="Jaeger 链路追踪"
          style={{
            width: '100%',
            height: 'calc(100vh - 220px)',
            minHeight: 560,
            border: 'none',
            borderRadius: 12,
            background: '#fff',
          }}
        />
      </Card>
    </div>
  )
}
