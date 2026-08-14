import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, Card, Col, Descriptions, Form, Row, Select, Space, Spin, Tag, Typography,
} from 'antd'
import { ApiOutlined, SafetyCertificateOutlined, SearchOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import {
  diagnosePermission,
  getPermissionDiagnosticMenus,
  getPermissionDiagnosticOptions,
  type PermissionDiagnosticOption,
  type PermissionDiagnosticResult,
  type PermissionMenuBinding,
} from '@/api/system/permission-diagnostic'
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
  const [permissionOptions, setPermissionOptions] = useState<PermissionDiagnosticOption[]>([])
  const [usersLoading, setUsersLoading] = useState(false)
  const [permissionsLoading, setPermissionsLoading] = useState(false)
  const [diagnosing, setDiagnosing] = useState(false)
  const [result, setResult] = useState<PermissionDiagnosticResult | null>(null)
  const [menus, setMenus] = useState<PermissionMenuBinding[]>([])
  const [menusUnavailable, setMenusUnavailable] = useState(false)

  useEffect(() => {
    setUsersLoading(true)
    getUserList({ page: 1, page_size: 100 })
      .then((response) => setUsers(response.list ?? []))
      .catch(() => message.error(t('加载用户失败')))
      .finally(() => setUsersLoading(false))
  }, [t])

  const loadPermissionOptions = async (keyword = '') => {
    setPermissionsLoading(true)
    try {
      setPermissionOptions(await getPermissionDiagnosticOptions({ keyword, limit: 100 }))
    } catch {
      message.error(t('加载权限失败'))
    } finally {
      setPermissionsLoading(false)
    }
  }

  useEffect(() => {
    setPermissionsLoading(true)
    getPermissionDiagnosticOptions({ limit: 100 })
      .then(setPermissionOptions)
      .catch(() => message.error(t('加载权限失败')))
      .finally(() => setPermissionsLoading(false))
  }, [t])

  const permissionSelectOptions = useMemo(() => permissionOptions.map((permission) => ({
    value: permission.code,
    label: permission.path
      ? `${permission.code} · ${permission.method || '--'} ${permission.path}`
      : `${permission.code} · ${permission.name}`,
  })), [permissionOptions])

  const handleDiagnose = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    const permission = values.permission.trim()
    setDiagnosing(true)
    setMenusUnavailable(false)
    try {
      const [diagnosticResult, menuResult] = await Promise.all([
        diagnosePermission({ user_id: values.user_id, permission }),
        getPermissionDiagnosticMenus(permission).catch(() => null),
      ])
      setResult(diagnosticResult)
      setMenus(menuResult?.menus ?? [])
      setMenusUnavailable(menuResult === null)
    } catch {
      setResult(null)
      setMenus([])
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
                <Select
                  showSearch
                  allowClear
                  filterOption={false}
                  loading={permissionsLoading}
                  placeholder="system:user:list"
                  options={permissionSelectOptions}
                  onSearch={loadPermissionOptions}
                  notFoundContent={permissionsLoading ? <Spin size="small" /> : t('无匹配权限')}
                />
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

              <Card size="small" title={<Space><ApiOutlined />{t('资源与菜单链')}</Space>}>
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  {result.resource.registered ? (
                    <div className="diagnostic-resource-row">
                      <Space wrap>
                        <Tag color="success">{t('已登记资源')}</Tag>
                        <strong>{result.resource.name}</strong>
                        {result.resource.method && <Tag color="blue">{result.resource.method}</Tag>}
                        <code>{result.resource.path || result.requested_permission}</code>
                      </Space>
                      {result.resource.description && <Typography.Text type="secondary">{result.resource.description}</Typography.Text>}
                    </div>
                  ) : (
                    <Alert type="warning" showIcon message={t('权限码未登记为资源')} description={t('该权限码不在权限资源表中，可能是历史数据或配置漂移')} />
                  )}
                  {menusUnavailable ? (
                    <Alert type="warning" showIcon message={t('菜单链暂不可用')} />
                  ) : menus.length === 0 ? (
                    <GlassEmpty compact text={t('没有菜单绑定此权限')} />
                  ) : (
                    <div className="diagnostic-menu-grid">
                      {menus.map((menu) => (
                        <div className="diagnostic-menu-row" key={menu.id}>
                          <Space wrap>
                            <strong>{menu.title}</strong>
                            <code>{menu.path || '--'}</code>
                            {menu.status !== 1 && <Tag color="error">{t('停用')}</Tag>}
                            {menu.hidden === 1 && <Tag>{t('隐藏')}</Tag>}
                          </Space>
                          <Typography.Text type="secondary">{menu.component || '--'}</Typography.Text>
                        </div>
                      ))}
                    </div>
                  )}
                </Space>
              </Card>

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
