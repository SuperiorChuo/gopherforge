import { useTranslation } from 'react-i18next'
import { Space, Tag, Typography } from 'antd'
import type { VisGeoResult } from '@/api/visibility'
import { hostnameOf } from './helpers'

const { Text, Paragraph } = Typography

function SentimentTag({ sentiment }: { sentiment?: string }) {
  const { t } = useTranslation()
  if (sentiment === 'positive') return <Tag color="green" variant="filled">{t('评价正面')}</Tag>
  if (sentiment === 'negative') return <Tag color="red" variant="filled">{t('评价负面')}</Tag>
  if (sentiment === 'neutral') return <Tag variant="filled">{t('评价中性')}</Tag>
  return null
}

export interface RunResultListProps {
  results: VisGeoResult[]
  runMode?: string
  runEngine?: string
}

export default function RunResultList({ results, runMode, runEngine }: RunResultListProps) {
  const { t } = useTranslation()

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={12}>
      {results.map((r) => (
        <div key={r.id} style={{ border: '1px solid rgba(148,163,184,.2)', borderRadius: 10, padding: 12 }}>
          <Text strong>{r.question}</Text>
          <div style={{ margin: '8px 0' }}>
            {r.mention_brand ? <Tag color="green" variant="filled">{t('提到品牌')}</Tag> : <Tag variant="filled">{t('没提到品牌')}</Tag>}
            {r.mention_competitor ? <Tag color="orange" variant="filled">{t('提到竞品')}</Tag> : null}
            {r.recommended ? <Tag color="blue" variant="filled">{t('正面推荐')}</Tag> : null}
            <SentimentTag sentiment={r.sentiment} />
            {r.list_position > 0 ? <Tag color="purple" variant="filled">{t('推荐榜第 {{n}} 位', { n: r.list_position })}</Tag> : null}
            {r.has_link ? <Tag variant="filled">{t('引用来源')}</Tag> : null}
            {r.analyze_by === 'heuristic' && runMode === 'llm' ? (
              <Text type="secondary" style={{ fontSize: 12 }}>{t('（关键词分析）')}</Text>
            ) : null}
          </div>
          <Paragraph type="secondary" style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
            {r.answer}
          </Paragraph>
          {r.sources && r.sources.length > 0 ? (
            <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                {t('来源引用（{{a}}/{{b}}）', { a: r.sources.filter((s) => s.cited).length, b: r.sources.length })}
              </Text>
              {r.sources.map((s, i) => {
                const host = hostnameOf(s.url)
                return (
                  <div key={i} style={{ fontSize: 12, display: 'flex', gap: 6, alignItems: 'baseline', opacity: s.cited ? 1 : 0.55 }}>
                    <Text style={{ fontSize: 12 }} type={s.cited ? undefined : 'secondary'}>
                      {s.cited ? t('被引用') : t('未引用')} ·{' '}
                      <a href={s.url} target="_blank" rel="noopener noreferrer">{s.title || host}</a>
                      <Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{host}</Text>
                    </Text>
                    {s.own_domain ? <Tag color="green" style={{ marginInlineEnd: 0 }}>{t('自有')}</Tag> : null}
                  </div>
                )
              })}
            </div>
          ) : runMode === 'llm' && runEngine !== 'chatgpt' && r.engine !== 'llm_fallback_stub' ? (
            <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
              {t('未检索到来源（已降级为无来源回答）')}
            </Text>
          ) : null}
        </div>
      ))}
    </Space>
  )
}
