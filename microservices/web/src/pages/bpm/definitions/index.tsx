import { useEffect, useState } from 'react'
import { Button, Card, Drawer, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd'
import {
  ApartmentOutlined,
  CopyOutlined,
  EditOutlined,
  EyeOutlined,
  PauseCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  SendOutlined,
  BarChartOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  BPM_BIZ_TYPE_PRESETS,
  BPM_DEFINITION_STATUS_META,
  createDefaultFlowSchema,
  createDefinition,
  listDefinitions,
  newDefinitionVersion,
  publishDefinition,
  suspendDefinition,
  type BpmDefinition,
} from '@/api/bpm'
import FlowDesigner from '@/pages/bpm/designer'
import BpmStatsPanel from '@/components/BpmStatsPanel'
import { useAppSelector } from '@/hooks/store'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  keyword?: string
  biz_type?: string
  page: number
  page_size: number
}

const BIZ_TYPE_OPTIONS = Object.entries(BPM_BIZ_TYPE_PRESETS).map(([value, meta]) => ({
  value,
  label: meta.label,
}))

export default function BpmDefinitionsPage() {
  const isPlatform = !!useAppSelector((s) => s.auth.userInfo)?.is_platform_admin
  const [statsOpen, setStatsOpen] = useState(false)
  const [list, setList] = useState<BpmDefinition[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  // 设计器视图：与列表同路由互斥切换（设计器不占独立路由，路由接线由主会话统一处理）
  const [design, setDesign] = useState<{ id: number; readOnly: boolean } | null>(null)
  const [createForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listDefinitions(p)
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!design) void fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params, design])

  const onCreate = async () => {
    const values = await createForm.validateFields().catch(() => null)
    if (!values) return
    setCreating(true)
    try {
      const res = await createDefinition({
        key: values.key,
        name: values.name,
        biz_type: values.biz_type || undefined,
        remark: values.remark || undefined,
        node_tree: createDefaultFlowSchema(values.biz_type),
      })
      message.success('已创建草稿，进入设计器配置节点')
      setCreateOpen(false)
      createForm.resetFields()
      if (res?.id) {
        setDesign({ id: res.id, readOnly: false })
      } else {
        void fetchList(params)
      }
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setCreating(false)
    }
  }

  const onPublish = async (row: BpmDefinition) => {
    try {
      await publishDefinition(row.id)
      message.success(`「${row.name}」v${row.version} 已发布`)
      void fetchList(params)
    } catch {
      // 后端 Schema 校验失败等，拦截器已提示；建议进设计器修正后再发布
    }
  }

  const onNewVersion = async (row: BpmDefinition) => {
    try {
      const res = await newDefinitionVersion(row.id)
      message.success('已复制出新草稿版本')
      if (res?.id) {
        setDesign({ id: res.id, readOnly: false })
      } else {
        void fetchList(params)
      }
    } catch {
      // 错误提示由 request 拦截器统一弹出
    }
  }

  const onSuspend = async (row: BpmDefinition) => {
    try {
      await suspendDefinition(row.id)
      message.success('已停用，不再允许新发起（在途实例不受影响）')
      void fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    }
  }

  const columns: ColumnsType<BpmDefinition> = [
    {
      title: '流程名称',
      dataIndex: 'name',
      render: (v: string, row) => (
        <div>
          <span style={{ fontWeight: 500 }}>{v}</span>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }} className="cell-mono">
              {row.key}
            </Typography.Text>
          </div>
        </div>
      ),
    },
    {
      title: '业务类型',
      dataIndex: 'biz_type',
      width: 150,
      render: (v?: string) =>
        v ? <Tag variant="filled">{BPM_BIZ_TYPE_PRESETS[v]?.label ?? v}</Tag> : <span className="cell-muted">—</span>,
    },
    {
      title: '版本',
      width: 150,
      render: (_, row) => (
        <Space size={6}>
          <Tag>v{row.version}</Tag>
          {row.active_version && row.active_version !== row.version ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              生效 v{row.active_version}
            </Typography.Text>
          ) : null}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (v: string) => {
        const meta = BPM_DEFINITION_STATUS_META[v]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} /> : <Tag>{v}</Tag>
      },
    },
    {
      title: '更新时间',
      width: 170,
      className: 'cell-time',
      render: (_, row) => formatDateTime(row.updated_at || row.created_at),
    },
    {
      title: '操作',
      width: 240,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          {row.status === 'draft' ? (
            <>
              {hasPerm('bpm:definition:update') && (
                <Button
                  type="link"
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => setDesign({ id: row.id, readOnly: false })}
                >
                  设计
                </Button>
              )}
              {hasPerm('bpm:definition:publish') && (
                <Popconfirm
                  title="发布该草稿版本？"
                  description="发布后立即生效，同一 key 的旧生效版本将自动归档"
                  onConfirm={() => void onPublish(row)}
                >
                  <Button type="link" size="small" icon={<SendOutlined />}>
                    发布
                  </Button>
                </Popconfirm>
              )}
            </>
          ) : (
            <>
              <Button
                type="link"
                size="small"
                icon={<EyeOutlined />}
                onClick={() => setDesign({ id: row.id, readOnly: true })}
              >
                查看
              </Button>
              {hasPerm('bpm:definition:update') && (
                <Button type="link" size="small" icon={<CopyOutlined />} onClick={() => void onNewVersion(row)}>
                  新版本
                </Button>
              )}
              {row.status === 'active' && hasPerm('bpm:definition:update') && (
                <Popconfirm
                  title="停用该流程？"
                  description="停用后不允许新发起，在途实例不受影响"
                  onConfirm={() => void onSuspend(row)}
                >
                  <Button type="link" size="small" danger icon={<PauseCircleOutlined />}>
                    停用
                  </Button>
                </Popconfirm>
              )}
            </>
          )}
        </Space>
      ),
    },
  ]

  if (design) {
    return (
      <div className="page-list bpm-definitions-page">
        <FlowDesigner
          definitionId={design.id}
          readOnly={design.readOnly}
          onBack={() => setDesign(null)}
        />
      </div>
    )
  }

  return (
    <div className="page-list bpm-definitions-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          initialValues={params}
          onFinish={(v) => setParams({ ...params, page: 1, keyword: v.keyword, biz_type: v.biz_type })}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索名称 / key" prefix={<SearchOutlined />} allowClear style={{ width: 240 }} />
          </Form.Item>
          <Form.Item name="biz_type">
            <Select placeholder="业务类型" style={{ width: 160 }} allowClear options={BIZ_TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                查询
              </Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => {
                  searchForm.resetFields()
                  setParams({ page: 1, page_size: 10 })
                }}
              >
                重置
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="流程定义"
          total={total}
          icon={<ApartmentOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.4)"
          description="审批流程模板的版本化管理，发布后业务方即可按 key 发起审批"
          extra={
            <Space wrap>
              {isPlatform && (
                <Button icon={<BarChartOutlined />} onClick={() => setStatsOpen(true)}>
                  审批统计
                </Button>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>
                刷新
              </Button>
              {hasPerm('bpm:definition:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                  新建流程
                </Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无流程定义，点击右上角新建" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Modal
        title="新建审批流程"
        open={createOpen}
        onOk={() => void onCreate()}
        onCancel={() => setCreateOpen(false)}
        confirmLoading={creating}
        okText="创建并进入设计器"
        destroyOnHidden
        width={520}
      >
        <Form form={createForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="key"
            label="流程标识 key"
            tooltip="业务方按此 key 发起审批；同一 key 可多版本，同时仅一个生效版本"
            rules={[
              { required: true, message: '请输入流程标识' },
              {
                pattern: /^[a-z][a-z0-9_]{1,63}$/,
                message: '小写字母开头，仅含小写字母/数字/下划线，2-64 位',
              },
            ]}
          >
            <Input placeholder="如：expense_approval" />
          </Form.Item>
          <Form.Item name="name" label="流程名称" rules={[{ required: true, message: '请输入流程名称' }]}>
            <Input placeholder="如：报销审批" maxLength={128} />
          </Form.Item>
          <Form.Item
            name="biz_type"
            label="业务类型"
            tooltip="决定发起表单快照字段与终态回写目标；留空为通用流程"
          >
            <Select placeholder="选择业务类型（可选）" allowClear options={BIZ_TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} maxLength={256} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>
      <Drawer
        title="审批统计"
        open={statsOpen}
        onClose={() => setStatsOpen(false)}
        width={720}
        destroyOnHidden
      >
        <BpmStatsPanel />
      </Drawer>
    </div>
  )
}
