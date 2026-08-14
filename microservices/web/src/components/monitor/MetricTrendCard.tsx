import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Segmented, Space } from 'antd'
import { LineChartOutlined } from '@ant-design/icons'
import { getMetricTrends, TREND_RANGES, type MetricTrend, type TrendRange } from '@/api/monitor'
import TrendChart from '@/components/common/TrendChart'

interface MetricTrendCardProps {
  title: string
  metric: string
  style?: React.CSSProperties
}

/** 指标历史趋势卡片：自绘 SVG 折线 + 1h/24h/7d 切换。 */
export default function MetricTrendCard({ title, metric, style }: MetricTrendCardProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState<TrendRange>('1h')
  const [trend, setTrend] = useState<MetricTrend | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(
    async (r: TrendRange) => {
      setLoading(true)
      try {
        const res = await getMetricTrends(metric, r)
        setTrend(res)
      } catch {
        setTrend(null)
      } finally {
        setLoading(false)
      }
    },
    [metric],
  )

  useEffect(() => {
    void load(range)
  }, [range, load])

  return (
    <Card
      className="glass-rise"
      style={style}
      title={
        <Space>
          <LineChartOutlined className="card-title-icon" /> {title}
        </Space>
      }
      extra={
        <Segmented
          size="small"
          value={range}
          options={TREND_RANGES.map((r) => ({ value: r.value, label: t(r.label) }))}
          onChange={(v) => setRange(v as TrendRange)}
        />
      }
    >
      <TrendChart points={trend?.points ?? []} unit={trend?.unit} loading={loading} />
    </Card>
  )
}
