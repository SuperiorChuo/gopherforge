import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Card, Col, Descriptions, Form, Input, Row, Select, Space, Spin, Tag, Typography,
} from 'antd'
import { SafetyCertificateOutlined, SearchOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/GlassEmpty'
import { diagnosePermission, type PermissionDiagnosticResult } from '@/api/system/permission-diagnostic'
import { getUserList } from '@/api/system/user'
import type { SystemUser } from '@/types'
import './styles.css'

const translateScope = (t: (key: string) => string, scope: string) => {
  switch (scope) {
    case 'all': return t('全部数据')
    case 'department_tree': return t('本部门及下级')
    case 'department': return t('本部门')
    case 'custom': return t('指定部门')
    case 'self': return t('仅本人')
    case 'none': return t('无数据')
    default: return scope || '--'
  }
}

export default function PermissionDiagnosticsPage() {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const [users, setUsers] = useState<SystemUser[]>([])
  const [usersLoading, setUsersLoading] = useState(false)
  const [diagnosing, setDiagnosing] = useState(false)
  const [result, setResult] = useState<PermissionDiagnosticResult | null>(null)

  useEffect(() => {
    setUsersLoading(true)
    getUserList({ page: 1, page_size: 100 })
      .then((response) => setUsers(response.list ?? []))
      .catch(() => message.error(t('加载用户失败')))
      .finally(() => setUsersLoading(false))
  }, [t])

  const handleDiagnose = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setDiagnosing(true)
    try {
      setResult(await diagnosePermission({
        user_id: values.user_id,
        permission: values.permission.trim(),
      }))
    } catch {
      setResult(null)
      message.error(t('权限诊断失败'))
    } finally {
      setDiagnosing(false)
    }
  }

  return (
    <div className="page-list permission-diagnostics-page">
      <Card className="list-filter-card" bordered={false}>
        <div className="list-page-heading">
          <div>
            <Typography.Title level={4}>{t('权限诊断')}</Typography.Title>
            <Typography.Text type="secondary">{t('解释用户是否拥有指定权限，并展示角色、套餐和数据范围链路')}</Typography.Text>
          </div>
          <SafetyCertificateOutlined className="page-heading-icon" />
        </div>
        <Form form={form} layout="vertical" onFinish={handleDiagnose}>
          <Row gutter={16} align="bottom">
            <Col xs={24} md={10}>
              <Form.Item name="user_id" label={t('目标用户')} rules={[{ required: true, message: t('请选择用户') }]}>
                <Select
                  showSearch
                  loading={usersLoading}
                  optionFilterProp="label"
                  placeholder={t('选择要诊断的用户')}
                  options={users.map((user) => ({
                    value: user.id,
                    label: `${user.nickname || user.username} (${user.username})`,
                  }))}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={10}>
              <Form.Item name="permission" label={t('权限码')} rules={[{ required: true, whitespace: true, message: t('请输入权限码') }]}>
                <Input allowClear placeholder="system:user:list" prefix={<SafetyCertificateOutlined />} />
              </Form.Item>
            </Col>
            <Col xs={24} md={4}>
              <Form.Item>
                <Button type="primary" htmlType="submit" block icon={<SearchOutlined />} loading={diagnosing}>
                  {t('开始诊断')}
                </Button>
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <Spin spinning={diagnosing}>
          {!result ? (
            <GlassEmpty text="选择用户并输入权限码后开始诊断" />
          ) : (
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
              <Alert
                showIcon
                type={result.allowed ? 'success' : 'error'}
                message={result.allowed ? t('允许访问') : t('拒绝访问')}
                description={result.reason}
              />

              <Descriptions bordered size="small" column={{ xs: 1, sm: 2, lg: 3 }}>
                <Descriptions.Item label={t('用户')}>{result.user.nickname || result.user.username} ({result.user.username})</Descriptions.Item>
                <Descriptions.Item label={t('租户 ID')}>{result.user.tenant_id}</Descriptions.Item>
                <Descriptions.Item label={t('账号状态')}>
                  <Tag color={result.user.status === 1 ? 'success' : 'error'}>{result.user.status === 1 ? t('启用') : t('停用')}</Tag>
                </Descriptions.Item>
                <Descriptions.Item label={t('权限码')}><code>{result.requested_permission}</code></Descriptions.Item>
                <Descriptions.Item label={t('命中来源')}>{result.matched_by || '--'}</Descriptions.Item>
                <Descriptions.Item label={t('数据范围')}>{translateScope(t, result.data_scope.scope)}</Descriptions.Item>
                <Descriptions.Item label={t('套餐约束')} span={3}>
                  {!result.package.bound ? t('未绑定套餐（不限）') : (
                    <Space wrap>
                      <span>{result.package.name || `#${result.package.id}`}</span>
                      <Tag color={result.package.allows_permission ? 'success' : 'warning'}>
                        {result.package.allows_permission ? t('套餐允许') : t('套餐不允许新分配')}
                      </Tag>
                      {result.package.has_existing_overrun && <Tag color="error">{t('存在存量越界权限')}</Tag>}
                    </Space>
                  )}
                </Descriptions.Item>
              </Descriptions>

              <Card size="small" title={t('角色授权链')}>
                {result.roles.length === 0 ? <GlassEmpty compact text="该用户没有角色" /> : (
                  <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    {result.roles.map((role) => (
                      <div key={role.id} className="diagnostic-role-row">
                        <div className="diagnostic-role-title">
                          <Space wrap>
                            <strong>{role.name}</strong>
                            <code>{role.code}</code>
                            <Tag>{translateScope(t, role.data_scope)}</Tag>
                            {role.matches && <Tag color="success">{t('命中')}</Tag>}
                          </Space>
                          <Typography.Text type="secondary">{role.match_reason || t('未命中')}</Typography.Text>
                        </div>
                        <div className="diagnostic-permission-tags">
                          {role.permissions.length > 0
                            ? role.permissions.map((permission) => <Tag key={permission} className="cell-mono">{permission}</Tag>)
                            : <Typography.Text type="secondary">{t('无权限')}</Typography.Text>}
                        </div>
                      </div>
                    ))}
                  </Space>
                )}
              </Card>
            </Space>
          )}
        </Spin>
      </Card>
    </div>
  )
}
