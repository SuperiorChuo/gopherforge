import { useTranslation } from 'react-i18next'
import { Button, Col, Row, Space, Statistic, Tag, Typography } from 'antd'
import { SwapOutlined } from '@ant-design/icons'
import type { VisGeoCompare, VisGeoRun } from '@/api/visibility'

const { Text, Paragraph } = Typography

export interface RunCompareCardProps {
  runId: number
  compare: VisGeoCompare | null
  comparePrev: VisGeoRun | null
  compareLoading: boolean
  onRefresh: (runId: number) => Promise<void> | void
}

export default function RunCompareCard({
  runId,
  compare,
  comparePrev,
  compareLoading,
  onRefresh,
}: RunCompareCardProps) {
  const { t } = useTranslation()

  return (
    <div
      style={{
        border: '1px solid rgba(59, 130, 246, .35)',
        borderRadius: 10,
        padding: 12,
        marginBottom: 16,
        background: 'rgba(59, 130, 246, .06)',
      }}
    >
      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 8 }}>
        <Text strong><SwapOutlined /> {t('与上次检测对比')}</Text>
        <Button size="small" loading={compareLoading} onClick={() => void onRefresh(runId)}>
          {t('刷新对比')}
        </Button>
      </Space>
      {!compare ? (
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {compareLoading ? t('加载中…') : t('暂无更早的完成记录，再跑一次检测后可对比提及率变化。')}
        </Paragraph>
      ) : (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Row gutter={12}>
            <Col span={8}>
              <Statistic
                title={t('本次提及率 (#{{n}})', { n: compare.current_id })}
                value={Math.round(compare.current_rate * 10) / 10}
                suffix="%"
              />
            </Col>
            <Col span={8}>
              <Statistic
                title={t('上次 (#{{a}}{{b}})', {
                  a: compare.previous_id,
                  b: comparePrev?.mode ? ` · ${comparePrev.mode}` : '',
                })}
                value={Math.round(compare.previous_rate * 10) / 10}
                suffix="%"
              />
            </Col>
            <Col span={8}>
              <Statistic
                title={t('变化')}
                value={Math.round(compare.rate_delta * 10) / 10}
                suffix="pt"
                valueStyle={{ color: compare.rate_delta >= 0 ? 'var(--c-success)' : 'var(--c-error)' }}
                prefix={compare.rate_delta >= 0 ? '+' : ''}
              />
            </Col>
          </Row>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {t('提及数 {{a}} → {{b}}（Δ {{c}}）· 新提到 {{d}} 题 · 新丢失 {{e}} 题 · 仍未提到 {{f}} 题', {
              a: compare.previous_mentions,
              b: compare.current_mentions,
              c: (compare.mention_delta >= 0 ? '+' : '') + compare.mention_delta,
              d: compare.newly_mentioned?.length || 0,
              e: compare.newly_missed?.length || 0,
              f: compare.still_missing?.length || 0,
            })}
          </Text>
          {(compare.newly_mentioned?.length || 0) > 0 ? (
            <div>
              <Text type="success" style={{ fontSize: 12 }}>{t('新提到：')}</Text>
              {compare.newly_mentioned!.slice(0, 5).map((q) => (
                <Tag key={q} color="green" style={{ marginTop: 4 }}>{q.slice(0, 40)}{q.length > 40 ? '…' : ''}</Tag>
              ))}
            </div>
          ) : null}
          {(compare.newly_missed?.length || 0) > 0 ? (
            <div>
              <Text type="danger" style={{ fontSize: 12 }}>{t('新丢失：')}</Text>
              {compare.newly_missed!.slice(0, 5).map((q) => (
                <Tag key={q} color="red" style={{ marginTop: 4 }}>{q.slice(0, 40)}{q.length > 40 ? '…' : ''}</Tag>
              ))}
            </div>
          ) : null}
        </Space>
      )}
    </div>
  )
}
