import { useTranslation } from 'react-i18next'
import { Button, Checkbox, Space, Tag, Typography } from 'antd'
import { BulbOutlined } from '@ant-design/icons'
import AiMarkdown from '@/components/common/AiMarkdown'
import type { VisAdviceItem } from '@/api/visibility'

const { Text, Paragraph } = Typography

export interface RunAdviceCardProps {
  advice?: string
  adviceItems: VisAdviceItem[]
  advising: boolean
  onGenerateAdvice: () => Promise<void> | void
  onToggleItem: (id: string, done: boolean) => void
}

export default function RunAdviceCard({
  advice,
  adviceItems,
  advising,
  onGenerateAdvice,
  onToggleItem,
}: RunAdviceCardProps) {
  const { t } = useTranslation()

  return (
    <div
      style={{
        border: '1px solid rgba(139, 92, 246, .35)',
        borderRadius: 10,
        padding: 12,
        marginBottom: 16,
        background: 'rgba(139, 92, 246, .06)',
      }}
    >
      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: advice ? 8 : 0 }}>
        <Text strong><BulbOutlined /> {t('AI 优化建议')}</Text>
        <Button size="small" loading={advising} onClick={() => void onGenerateAdvice()}>
          {advice ? t('重新生成') : t('生成建议')}
        </Button>
      </Space>
      {advice ? (
        <>
          {adviceItems.length > 0 ? (
            <div style={{ marginBottom: 12 }}>
              <Text strong style={{ fontSize: 13 }}>{t('可执行清单')}</Text>
              <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
                {t('已完成 {{a}}/{{b}}', { a: adviceItems.filter((i) => i.done).length, b: adviceItems.length })}
              </Text>
              <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
                {adviceItems.map((it) => (
                  <div
                    key={it.id}
                    style={{
                      display: 'flex',
                      gap: 8,
                      alignItems: 'flex-start',
                      opacity: it.done ? 0.65 : 1,
                    }}
                  >
                    <Checkbox
                      checked={!!it.done}
                      onChange={(e) => void onToggleItem(it.id, e.target.checked)}
                    />
                    <div style={{ flex: 1 }}>
                      <Space size={6} wrap>
                        <Text delete={!!it.done} strong={!it.done}>{it.title}</Text>
                        {it.priority === 'high' ? <Tag color="red">{t('高优')}</Tag> : null}
                        {it.priority === 'medium' ? <Tag color="orange">{t('中')}</Tag> : null}
                        {it.priority === 'low' ? <Tag>{t('低')}</Tag> : null}
                      </Space>
                      {it.detail ? (
                        <Paragraph type="secondary" style={{ marginBottom: 0, fontSize: 12 }}>
                          {it.detail}
                        </Paragraph>
                      ) : null}
                      {it.related ? (
                        <Text type="secondary" style={{ fontSize: 11 }}>{t('关联：{{name}}', { name: it.related })}</Text>
                      ) : null}
                    </div>
                  </div>
                ))}
              </Space>
            </div>
          ) : null}
          <AiMarkdown content={advice} />
        </>
      ) : (
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t('根据这次检测的逐题结果，生成可勾选的行动清单 + 完整建议正文，并与上次检测对比提及率变化。')}
        </Paragraph>
      )}
    </div>
  )
}
