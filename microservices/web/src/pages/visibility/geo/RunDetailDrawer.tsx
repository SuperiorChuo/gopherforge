import { useTranslation } from 'react-i18next'
import { Alert, Col, Descriptions, Drawer, Progress, Row, Statistic, Tag, Typography } from 'antd'
import { formatDateTime } from '@/utils/format'
import type { VisAdviceItem, VisGeoCompare, VisGeoResult, VisGeoRun } from '@/api/visibility'
import RunCompareCard from './RunCompareCard'
import RunAdviceCard from './RunAdviceCard'
import RunResultList from './RunResultList'

const { Text } = Typography

export interface RunDetailDrawerProps {
  open: boolean
  run: VisGeoRun | null
  results: VisGeoResult[]
  advising: boolean
  adviceItems: VisAdviceItem[]
  compare: VisGeoCompare | null
  comparePrev: VisGeoRun | null
  compareLoading: boolean
  onClose: () => void
  onAdvice: () => Promise<void> | void
  onRefreshCompare: (runId: number) => Promise<void> | void
  onToggleAdviceItem: (id: string, done: boolean) => void
}

export default function RunDetailDrawer({
  open,
  run,
  results,
  advising,
  adviceItems,
  compare,
  comparePrev,
  compareLoading,
  onClose,
  onAdvice,
  onRefreshCompare,
  onToggleAdviceItem,
}: RunDetailDrawerProps) {
  const { t } = useTranslation()

  const isRunning = run?.status === 'running'
  const percent = run && run.prompt_count > 0
    ? Math.round(((run.done_count ?? 0) / run.prompt_count) * 100)
    : 0
  const recommendedCount = results.filter((r) => r.recommended).length
  const competitorCount = results.filter((r) => r.mention_competitor).length
  const matchedBrands = Array.from(
    new Set(results.flatMap((r) => (r.matched_brands || '').split(',').map((s) => s.trim()).filter(Boolean))),
  )

  return (
    <Drawer
      title={run ? t('检测 #{{n}}', { n: run.id }) : t('检测结果')}
      open={open}
      onClose={onClose}
      width={560}
    >
      {isRunning ? (
        <div style={{ marginBottom: 16 }}>
          <Progress percent={percent} status="active" />
          <Text type="secondary">
            {t('已完成 {{a}}/{{b}} 个问题，正在向 AI 提问……可以关闭此窗口，稍后在「检测记录」里查看', { a: run?.done_count ?? 0, b: run?.prompt_count ?? 0 })}
          </Text>
        </div>
      ) : null}
      {run?.status === 'failed' ? (
        <Alert type="error" showIcon style={{ marginBottom: 16 }} message={t('检测失败')} description={run.error || t('未知原因')} />
      ) : null}
      {run && run.status === 'done' ? (
        <>
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col span={8}>
              <Statistic
                title={t('提及率')}
                value={run.prompt_count > 0 ? Math.round((run.mention_count / run.prompt_count) * 100) : 0}
                suffix="%"
              />
            </Col>
            <Col span={8}>
              <Statistic title={t('正面推荐')} value={recommendedCount} suffix={t('条')} />
            </Col>
            <Col span={8}>
              <Statistic title={t('提到竞品')} value={competitorCount} suffix={t('条')} />
            </Col>
          </Row>
          {matchedBrands.length > 0 ? (
            <div style={{ marginBottom: 16 }}>
              <Text type="secondary">{t('回答中出现过的品牌：')}</Text>{' '}
              {matchedBrands.map((b) => <Tag key={b} variant="filled">{b}</Tag>)}
            </div>
          ) : null}
          <Descriptions size="small" column={1} bordered style={{ marginBottom: 16 }}>
            <Descriptions.Item label={t('方式')}>
              {run.mode === 'stub' ? t('模拟演示') : run.engine === 'chatgpt' ? t('真实 AI · ChatGPT 联网搜索') : t('真实 AI · 自建检索')}
            </Descriptions.Item>
            <Descriptions.Item label={t('时间')}>{formatDateTime(run.created_at)}</Descriptions.Item>
          </Descriptions>
          <RunCompareCard
            runId={run.id}
            compare={compare}
            comparePrev={comparePrev}
            compareLoading={compareLoading}
            onRefresh={onRefreshCompare}
          />
          <RunAdviceCard
            advice={run.advice}
            adviceItems={adviceItems}
            advising={advising}
            onGenerateAdvice={onAdvice}
            onToggleItem={onToggleAdviceItem}
          />
        </>
      ) : null}
      <RunResultList results={results} runMode={run?.mode} runEngine={run?.engine} />
    </Drawer>
  )
}
