import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Input, Row, Segmented, Space, Tag, Tooltip } from 'antd'
import { AppstoreOutlined, ClusterOutlined, ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { listHermesClusters, listHermesPolicies, type HermesCluster, type HermesPolicy } from '@/api/ai/hermes'
import TableToolbar from '@/components/common/TableToolbar'
import { fetchAllHermesPages, policyToolsetCatalog } from '../admin-contract'
import { p1Toolsets, p2ClusterToolsets } from '../_shared'

interface ApprovedTool {
  key: string
  name: string
  category: '协作' | '生产力' | '知识' | '安全'
  description: string
  source: 'Hermes 内置' | '控制面内置'
}

const TOOL_DETAILS: Record<string, Omit<ApprovedTool, 'key'>> = {
  clarify: { name: '需求澄清', category: '协作', description: '在目标或约束不完整时发起结构化澄清。', source: 'Hermes 内置' },
  memory: { name: '本地记忆', category: '知识', description: '在员工本机维护受当前用户隔离的长期记忆。', source: 'Hermes 内置' },
  todo: { name: '任务清单', category: '生产力', description: '维护可追踪的个人任务与完成状态。', source: 'Hermes 内置' },
  no_mcp: { name: 'MCP 隔离', category: '安全', description: '阻止受管端点加载未经企业批准的 MCP 工具。', source: 'Hermes 内置' },
  delegation: { name: '任务委派', category: '协作', description: '允许 dispatcher 在受控 Agent 集群内拆解和委派任务。', source: '控制面内置' },
  skills: { name: '企业技能', category: '生产力', description: '允许 dispatcher 使用企业已审批并绑定到集群的技能。', source: '控制面内置' },
  mcp: { name: '企业 MCP', category: '生产力', description: '允许 dispatcher 使用企业已审批并绑定到集群的 MCP Server。', source: '控制面内置' },
}

const APPROVED_TOOLS: ApprovedTool[] = policyToolsetCatalog(p1Toolsets, p2ClusterToolsets).map((key) => ({
  key,
  ...TOOL_DETAILS[key],
}))
const CHANNEL_CAPABILITIES = [
  { key: 'kanban', name: 'Kanban 协作通道', description: '由集群 slug 启用，负责把协作任务派生给 worker；它不是策略工具。' },
]

type CategoryFilter = '全部' | ApprovedTool['category']

export default function HermesSkillsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [policies, setPolicies] = useState<HermesPolicy[]>([])
  const [clusters, setClusters] = useState<HermesCluster[]>([])
  const [keyword, setKeyword] = useState('')
  const [category, setCategory] = useState<CategoryFilter>('全部')

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      // 统计引用覆盖全部策略/集群：逐页真实拉取，不放大 page_size 伪装全量
      const [policyResult, clusterResult] = await Promise.allSettled([
        fetchAllHermesPages<HermesPolicy>((page, pageSize) => listHermesPolicies({ page, page_size: pageSize })),
        fetchAllHermesPages<HermesCluster>((page, pageSize) => listHermesClusters({ page, page_size: pageSize })),
      ])
      if (policyResult.status === 'fulfilled') setPolicies(policyResult.value)
      if (clusterResult.status === 'fulfilled') setClusters(clusterResult.value)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const filtered = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    return APPROVED_TOOLS.filter((tool) => {
      if (category !== '全部' && tool.category !== category) return false
      if (!needle) return true
      return `${tool.key} ${tool.name} ${tool.description}`.toLowerCase().includes(needle)
    })
  }, [category, keyword])

  const policyCount = (tool: ApprovedTool) =>
    policies.filter((policy) => (policy.toolsets ?? []).includes(tool.key)).length

  return (
    <div className="page-list">
      <TableToolbar
        title="工具白名单"
        total={APPROVED_TOOLS.length}
        icon={<AppstoreOutlined />}
        gradient="linear-gradient(135deg, #ec4899, #7c3aed)"
        glow="rgba(236, 72, 153, 0.24)"
        description={t('{{a}} 项策略工具 · 覆盖设备与 dispatcher 两类策略', { a: APPROVED_TOOLS.length })}
        extra={
          <Tooltip title={t('刷新策略引用')}>
            <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading} />
          </Tooltip>
        }
      />

      <Space wrap style={{ marginBottom: 16 }}>
        <Input.Search
          allowClear
          placeholder={t('搜索工具')}
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          style={{ width: 260, maxWidth: '100%' }}
        />
        <Segmented
          value={category}
          onChange={(value) => setCategory(value as CategoryFilter)}
          options={['全部', '协作', '生产力', '知识', '安全'].map((c) => ({ label: t(c), value: c }))}
        />
      </Space>

      <Row gutter={[16, 16]}>
        {filtered.map((tool) => {
          const references = policyCount(tool)
          return (
            <Col key={tool.key} xs={24} md={12} xl={8}>
              <Card
                title={t(tool.name)}
                extra={<Tag color="success">{t('已批准')}</Tag>}
                actions={[
                  <Button
                    key="policy"
                    type="text"
                    icon={<SafetyCertificateOutlined />}
                    onClick={() => navigate('/hermes/models')}
                  >
                    {t('设备策略')}
                  </Button>,
                ]}
              >
                <div className="cell-mono" style={{ marginBottom: 10 }}>{tool.key}</div>
                <div style={{ minHeight: 44, color: 'var(--text-secondary)' }}>{t(tool.description)}</div>
                <Space wrap style={{ marginTop: 16 }}>
                  <Tag>{t(tool.category)}</Tag>
                  <Tag>{t(tool.source)}</Tag>
                  <Tag>{p1Toolsets.includes(tool.key) ? t('设备 + dispatcher') : 'dispatcher'}</Tag>
                  <Tag color={references > 0 ? 'processing' : 'default'}>{t('{{n}} 个策略引用', { n: references })}</Tag>
                </Space>
              </Card>
            </Col>
          )
        })}
      </Row>

      <div style={{ marginTop: 24, marginBottom: 12, fontWeight: 600 }}>
        <ClusterOutlined /> {t('集群通道能力')}
      </div>
      <Row gutter={[16, 16]}>
        {CHANNEL_CAPABILITIES.map((capability) => {
          const enabledClusters = clusters.filter((cluster) => cluster.enabled && cluster.slug)
          return (
            <Col key={capability.key} xs={24} md={12} xl={8}>
              <Card title={t(capability.name)} extra={<Tag color="cyan">{t('通道能力')}</Tag>}>
                <div className="cell-mono" style={{ marginBottom: 10 }}>{capability.key}</div>
                <div style={{ minHeight: 44, color: 'var(--text-secondary)' }}>{t(capability.description)}</div>
                <Space wrap style={{ marginTop: 16 }}>
                  <Tag>{t('集群 slug 启用')}</Tag>
                  <Tag color={enabledClusters.length > 0 ? 'processing' : 'default'}>
                    {t('{{n}} 个集群启用', { n: enabledClusters.length })}
                  </Tag>
                </Space>
              </Card>
            </Col>
          )
        })}
      </Row>
    </div>
  )
}
