import { useTranslation } from 'react-i18next'
import { Button, Tooltip } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import type { MonitorAlertSummary } from '@/api/monitor'

interface OverviewStripProps {
  summary: MonitorAlertSummary | null
  loading: boolean
  failed: boolean
  onRefresh: () => void
}

export default function OverviewStrip({ summary, loading, failed, onRefresh }: OverviewStripProps) {
  const { t } = useTranslation()

  const summaryStatusClass = failed
    ? summary
      ? 'is-stale'
      : 'is-error'
    : summary
      ? 'is-live'
      : 'is-loading'

  const summaryStatusText = failed
    ? summary
      ? t('概览刷新中断 · 保留 {{n}} 数据', { n: dayjs(summary.checked_at).format('HH:mm:ss') })
      : t('告警概览暂不可用')
    : summary
      ? t('概览更新于 {{n}}', { n: dayjs(summary.checked_at).format('HH:mm:ss') })
      : t('正在获取告警概览')

  return (
    <div className="alert-overview-block">
      <div className="alert-overview-meta" aria-live="polite" aria-atomic="true">
        <span className={`alert-overview-status ${summaryStatusClass}`}>
          <span className="alert-overview-status-dot" />
          {summaryStatusText}
        </span>
        <Tooltip title={t('刷新告警概览')}>
          <Button
            type="text"
            size="small"
            icon={<ReloadOutlined />}
            aria-label={t('刷新告警概览')}
            loading={loading}
            onClick={onRefresh}
          />
        </Tooltip>
      </div>
      <div
        className={`alert-overview-strip ${summary ? '' : 'is-unavailable'}`}
        aria-label={t('告警状态摘要')}
        aria-busy={loading}
      >
        <div><span>{t('启用规则')}</span><strong>{summary?.enabled ?? '--'}</strong></div>
        <div data-tone="danger"><span>{t('告警中')}</span><strong>{summary?.firing ?? '--'}</strong></div>
        <div data-tone="warning"><span>{t('等待确认')}</span><strong>{summary?.pending ?? '--'}</strong></div>
        <div data-tone="error"><span>{t('采集异常')}</span><strong>{summary?.error ?? '--'}</strong></div>
      </div>
    </div>
  )
}
