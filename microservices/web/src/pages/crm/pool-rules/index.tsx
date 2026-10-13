import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Card, Col, InputNumber, List, Popconfirm, Row, Space, Statistic, Switch, Table, Tag, Typography,
} from 'antd'
import {
  SyncOutlined, SafetyOutlined, FieldTimeOutlined, InboxOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import i18n from '@/i18n/init'
import {
  getPoolRules, getPoolStats, recyclePoolNow, savePoolRule,
  type CrmPoolRule, type PoolPreviewRow, type PoolStats,
} from '@/api/crm'
import GlassEmpty from '@/components/common/GlassEmpty'
import TableToolbar from '@/components/common/TableToolbar'

const { Text, Paragraph } = Typography

const LEVEL_META: Record<string, { label: string; color: string; desc: string }> = {
  '*': { label: '默认档', color: 'default', desc: '未被 A/B/C 覆盖的客户走此档' },
  A: { label: 'A 级', color: 'gold', desc: '重点客户，通常给更长保护期' },
  B: { label: 'B 级', color: 'blue', desc: '普通客户' },
  C: { label: 'C 级', color: 'default', desc: '低优客户，可更快回收' },
}

// 公海回收规则引擎（独立菜单页）：分级规则配置 + 公海实时态 +
// 预估回收名单 + 手动触发。规则每小时由 crm-service 引擎执行。
export default function CrmPoolRulesPage() {
  const { t } = useTranslation()
  const [rules, setRules] = useState<CrmPoolRule[]>([])
  const [stats, setStats] = useState<PoolStats | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState<string | null>(null)
  const [recycling, setRecycling] = useState(false)

  // load 由挂载与手动刷新触发；catch 文案用全局 i18n.t（取失败当下语言，
  // 不产生订阅依赖），语言切换不应重拉。
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [r, st] = await Promise.all([getPoolRules(), getPoolStats()])
      const byLevel = new Map(r.rules.map((x) => [x.level, x]))
      setRules(['*', 'A', 'B', 'C'].map((lv) =>
        byLevel.get(lv) ?? { level: lv, idle_days: 0, protect_active: true, enabled: false }))
      setStats(st)
    } catch {
      message.error(i18n.t('读取规则失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  async function saveRule(rule: CrmPoolRule) {
    setSaving(rule.level)
    try {
      await savePoolRule(rule)
      message.success(t('{{label}} 规则已保存', { label: t(LEVEL_META[rule.level]?.label ?? rule.level) }))
      // 规则变化影响预估名单，重拉实时态
      getPoolStats().then(setStats).catch(() => {})
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      setSaving(null)
    }
  }

  const patch = (level: string, p: Partial<CrmPoolRule>) =>
    setRules((prev) => prev.map((r) => (r.level === level ? { ...r, ...p } : r)))

  async function onRecycleNow() {
    setRecycling(true)
    try {
      const res = await recyclePoolNow()
      message.success(res.recycled > 0 ? t('本轮回收 {{n}} 个客户进公海', { n: res.recycled }) : t('本轮无可回收客户'))
      void load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('执行失败'))
    } finally {
      setRecycling(false)
    }
  }

  const previewColumns: ColumnsType<PoolPreviewRow> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: t('客户'), dataIndex: 'name', ellipsis: true },
    {
      title: t('级别'), dataIndex: 'level', width: 80,
      render: (v: string) => <Tag color={LEVEL_META[v]?.color || 'default'} variant="filled">{v || 'C'}</Tag>,
    },
    { title: t('归属'), dataIndex: 'owner_user_id', width: 90, render: (v: number) => <Tag color="blue" variant="filled">#{v}</Tag> },
    {
      title: t('本轮结果'), width: 130,
      render: (_, row) => row.protected
        ? <Tag color="green" variant="filled" icon={<SafetyOutlined />}>{t('保护不回收')}</Tag>
        : <Tag color="orange" variant="filled">{t('将回收')}</Tag>,
    },
  ]

  return (
    <div className="page-list crm-pool-rules-page">
      <TableToolbar title="公海规则" total={stats?.pool_count} />
      <Row gutter={16}>
        <Col xs={24} sm={8}>
          <Card bordered={false} loading={loading}>
            <Statistic title={t('当前公海客户')} value={stats?.pool_count ?? 0} prefix={<InboxOutlined style={{ color: 'var(--c-cyan)' }} />} />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card bordered={false} loading={loading}>
            <Statistic title={t('今日已回收')} value={stats?.today_recycled ?? 0} prefix={<SyncOutlined style={{ color: 'var(--c-warning)' }} />} />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card bordered={false} loading={loading}>
            <Statistic title={t('按现行规则待回收')} value={stats?.preview_count ?? 0} prefix={<FieldTimeOutlined style={{ color: 'var(--c-error)' }} />} />
          </Card>
        </Col>
      </Row>

      <Card bordered={false} style={{ marginTop: 16 }} title={t('分级回收规则')}
        extra={
          <Popconfirm title={t('立即按现行规则执行一轮回收？')} onConfirm={() => void onRecycleNow()}>
            <Button type="primary" icon={<ThunderboltOutlined />} loading={recycling}>{t('立即执行回收')}</Button>
          </Popconfirm>
        }>
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          {t('超过设定天数未跟进的客户由引擎自动回收进公海（每小时扫描一次，跟进/领取会重置计时）。A/B/C 级覆盖默认档；停用某档或天数为 0 表示该档永不回收。「保护」开启时，有进行中商机或生效合同的客户不会被回收。')}
        </Paragraph>
        <List
          dataSource={rules}
          loading={loading}
          renderItem={(r) => {
            const meta = LEVEL_META[r.level] ?? { label: r.level, color: 'default', desc: '' }
            return (
              <List.Item
                actions={[
                  <Button key="save" size="small" type="primary" ghost loading={saving === r.level}
                    onClick={() => void saveRule(r)}>
                    {t('保存')}
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  avatar={<Tag color={meta.color} variant="filled" style={{ width: 64, textAlign: 'center', marginTop: 4 }}>{t(meta.label)}</Tag>}
                  title={
                    <Space size={16} wrap>
                      <Switch checked={r.enabled} checkedChildren={t('启用')} unCheckedChildren={t('停用')}
                        onChange={(v) => patch(r.level, { enabled: v })} />
                      <InputNumber min={0} max={3650} value={r.idle_days} disabled={!r.enabled}
                        addonAfter={t('天未跟进回收')} style={{ width: 210 }}
                        onChange={(v) => patch(r.level, { idle_days: Number(v ?? 0) })} />
                      <Space size={6}>
                        <Switch checked={r.protect_active} disabled={!r.enabled} checkedChildren={t('保护')} unCheckedChildren={t('不保护')}
                          onChange={(v) => patch(r.level, { protect_active: v })} />
                        <Text type="secondary" style={{ fontSize: 12 }}>{t('商机/合同保护')}</Text>
                      </Space>
                    </Space>
                  }
                  description={t(meta.desc)}
                />
              </List.Item>
            )
          }}
        />
      </Card>

      <Card bordered={false} style={{ marginTop: 16 }}
        title={t('预估回收名单（下轮扫描，最多显示 20 条）')}>
        <Table<PoolPreviewRow> rowKey="id" size="small" columns={previewColumns}
          dataSource={stats?.preview ?? []} loading={loading} pagination={false}
          locale={{ emptyText: <GlassEmpty text="按现行规则暂无待回收客户" compact /> }} />
      </Card>
    </div>
  )
}
