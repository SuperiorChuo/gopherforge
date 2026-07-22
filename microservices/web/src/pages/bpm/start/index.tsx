import { useEffect, useState } from 'react'
import { Button, Card, Col, Form, Modal, Row, Space, Tag, Typography } from 'antd'
import { FormOutlined, ReloadOutlined, SendOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { message } from '@/utils/feedback'
import {
  listStartableDefinitions,
  startFormInstance,
  type BpmDefinition,
} from '@/api/bpm'
import BpmDynamicForm, { formValuesToSnapshot } from '@/components/BpmDynamicForm'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'

const { Text } = Typography

/**
 * 通用发起页（表单构建器 M1）：流程表单模式的统一发起入口——浏览可发起
 * 流程 → 动态渲染表单 → 提交。业务表单仍从业务页入口发起。
 */
export default function BpmStartPage() {
  const navigate = useNavigate()
  const [list, setList] = useState<BpmDefinition[]>([])
  const [loading, setLoading] = useState(false)
  const [current, setCurrent] = useState<BpmDefinition | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const load = async () => {
    setLoading(true)
    try {
      setList(await listStartableDefinitions())
    } catch {
      // 拦截器统一提示
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const submit = async () => {
    if (!current) return
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      await startFormInstance(current.key, formValuesToSnapshot(current.form_schema, values))
      message.success('已发起，可在「我发起的」查看进度')
      setCurrent(null)
      navigate('/bpm/instances')
    } catch {
      // 拦截器统一提示（服务端按 Schema 权威校验）
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="page-list bpm-start-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="发起申请"
          total={list.length}
          icon={<FormOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.35)"
          description="选择流程填写表单即可发起；进度在「我发起的」跟踪"
          extra={
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>
              刷新
            </Button>
          }
        />
        {list.length === 0 && !loading ? (
          <GlassEmpty text="暂无可发起的流程（需管理员在流程定义中配置表单并发布）" />
        ) : (
          <Row gutter={[16, 16]}>
            {list.map((d) => (
              <Col key={d.id} xs={24} sm={12} lg={8} xl={6}>
                <Card
                  hoverable
                  size="small"
                  onClick={() => {
                    form.resetFields()
                    setCurrent(d)
                  }}
                >
                  <Space direction="vertical" size={6} style={{ width: '100%' }}>
                    <Space size={8}>
                      <FormOutlined style={{ color: '#7c3aed' }} />
                      <Text strong>{d.name}</Text>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 12 }} className="cell-mono">
                      {d.key} · v{d.version}
                    </Text>
                    <Space size={4} wrap>
                      <Tag color="purple">{d.form_schema?.fields?.length ?? 0} 个字段</Tag>
                      {d.remark ? (
                        <Text type="secondary" style={{ fontSize: 12 }} ellipsis>
                          {d.remark}
                        </Text>
                      ) : null}
                    </Space>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Card>

      <Modal
        title={current ? `发起：${current.name}` : '发起'}
        open={!!current}
        onCancel={() => setCurrent(null)}
        okText="提交"
        okButtonProps={{ icon: <SendOutlined />, loading: submitting }}
        onOk={() => void submit()}
        destroyOnHidden
        width={520}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 8 }}>
          <BpmDynamicForm schema={current?.form_schema} />
        </Form>
      </Modal>
    </div>
  )
}
