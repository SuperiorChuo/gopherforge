import { useEffect, useState } from 'react'
import {
  Card, Row, Col, Form, Input, Button, Tag,
  Modal, Steps, Space, Avatar, Table,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  UserOutlined, MailOutlined, PhoneOutlined, HistoryOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useAppSelector } from '@/hooks/store'
import {
  updateProfile, changePassword,
  generateTotpSetup, enableTotp, disableTotp, regenerateTotpRecoveryCodes,
} from '@/api/auth'
import { useAppDispatch } from '@/hooks/store'
import { fetchCurrentUser } from '@/store/slices/authSlice'
import { getMyLoginLogs } from '@/api/system/log'
import type { LoginLog } from '@/types'
import { formatDateTime } from '@/utils/format'

const loginLogColumns: ColumnsType<LoginLog> = [
  { title: '时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
  {
    title: 'IP',
    dataIndex: 'ip',
    width: 140,
    render: (v: string) => <span className="cell-mono">{v || '-'}</span>,
  },
  {
    title: '状态',
    dataIndex: 'status',
    width: 80,
    render: (v: number) => <Tag color={v === 1 ? 'success' : 'error'}>{v === 1 ? '成功' : '失败'}</Tag>,
  },
  { title: '浏览器', dataIndex: 'browser', ellipsis: true },
  { title: '系统', dataIndex: 'os', width: 140 },
]

export default function ProfilePage() {
  const dispatch = useAppDispatch()
  const { userInfo } = useAppSelector((s) => s.auth)

  const [profileForm] = Form.useForm()
  const [pwdForm] = Form.useForm()
  const [profileLoading, setProfileLoading] = useState(false)
  const [pwdLoading, setPwdLoading] = useState(false)

  const [enableTotpOpen, setEnableTotpOpen] = useState(false)
  const [enableStep, setEnableStep] = useState(0)
  const [setupPassword, setSetupPassword] = useState('')
  const [qrCode, setQrCode] = useState('')
  const [enableLoading, setEnableLoading] = useState(false)
  const [enableForm] = Form.useForm()
  const [enableCodeForm] = Form.useForm()

  const [disableTotpOpen, setDisableTotpOpen] = useState(false)
  const [disableForm] = Form.useForm()
  const [disableLoading, setDisableLoading] = useState(false)

  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [regenOpen, setRegenOpen] = useState(false)
  const [regenForm] = Form.useForm()
  const [regenLoading, setRegenLoading] = useState(false)

  const [myLogs, setMyLogs] = useState<LoginLog[]>([])
  const [logsLoading, setLogsLoading] = useState(true)

  useEffect(() => {
    getMyLoginLogs({ page: 1, page_size: 5 })
      .then((res) => setMyLogs(res.list ?? []))
      .catch(() => setMyLogs([]))
      .finally(() => setLogsLoading(false))
  }, [])

  const handleSaveProfile = async () => {
    const values = await profileForm.validateFields().catch(() => null)
    if (!values) return
    setProfileLoading(true)
    try {
      await updateProfile(values)
      message.success('保存成功')
      dispatch(fetchCurrentUser())
    } catch {
      message.error('保存失败')
    } finally {
      setProfileLoading(false)
    }
  }

  const handleChangePassword = async () => {
    const values = await pwdForm.validateFields().catch(() => null)
    if (!values) return
    setPwdLoading(true)
    try {
      await changePassword({ old_password: values.old_password, new_password: values.new_password })
      message.success('密码修改成功')
      pwdForm.resetFields()
    } catch {
      message.error('密码修改失败')
    } finally {
      setPwdLoading(false)
    }
  }

  const openEnableTotp = () => {
    setEnableStep(0)
    setQrCode('')
    setSetupPassword('')
    enableForm.resetFields()
    enableCodeForm.resetFields()
    setEnableTotpOpen(true)
  }

  const handleEnableNext = async () => {
    const values = await enableForm.validateFields().catch(() => null)
    if (!values) return
    setEnableLoading(true)
    try {
      const res = await generateTotpSetup({ current_password: values.current_password }) as unknown as Record<string, unknown>
      setQrCode(String(res.qr_code ?? ''))
      setSetupPassword(values.current_password)
      setEnableStep(1)
    } catch {
      message.error('获取二维码失败')
    } finally {
      setEnableLoading(false)
    }
  }

  const handleEnableConfirm = async () => {
    const values = await enableCodeForm.validateFields().catch(() => null)
    if (!values) return
    setEnableLoading(true)
    try {
      const res = await enableTotp({ code: values.code, current_password: setupPassword }) as unknown as { recovery_codes?: string[] } | null
      message.success('TOTP 已启用')
      setEnableTotpOpen(false)
      // 恢复码只在此刻返回一次，必须立即展示给用户保存
      if (res?.recovery_codes?.length) {
        setRecoveryCodes(res.recovery_codes)
      }
      dispatch(fetchCurrentUser())
    } catch {
      message.error('验证失败')
    } finally {
      setEnableLoading(false)
    }
  }

  const handleRegenCodes = async () => {
    const values = await regenForm.validateFields().catch(() => null)
    if (!values) return
    setRegenLoading(true)
    try {
      const res = await regenerateTotpRecoveryCodes(values) as unknown as { recovery_codes?: string[] } | null
      setRegenOpen(false)
      if (res?.recovery_codes?.length) {
        setRecoveryCodes(res.recovery_codes)
      } else {
        message.success('恢复码已重新生成')
      }
    } catch {
      message.error('操作失败，请检查验证码和密码')
    } finally {
      setRegenLoading(false)
    }
  }

  const copyRecoveryCodes = async () => {
    if (!recoveryCodes) return
    try {
      await navigator.clipboard.writeText(recoveryCodes.join('\n'))
      message.success('已复制到剪贴板')
    } catch {
      message.error('复制失败，请手动选择复制')
    }
  }

  const handleDisableTotp = async () => {
    const values = await disableForm.validateFields().catch(() => null)
    if (!values) return
    setDisableLoading(true)
    try {
      await disableTotp(values)
      message.success('TOTP 已禁用')
      setDisableTotpOpen(false)
      dispatch(fetchCurrentUser())
    } catch {
      message.error('操作失败')
    } finally {
      setDisableLoading(false)
    }
  }

  return (
    <div>
      <div className="profile-hero">
        <div className="profile-hero-avatar-ring">
          <Avatar size={72} icon={<UserOutlined />} className="profile-hero-avatar" />
        </div>
        <div className="profile-hero-info">
          <div className="profile-hero-name">
            {userInfo?.nickname || userInfo?.username}
            {userInfo?.totp_enabled && <span className="profile-hero-2fa">2FA 已开启</span>}
          </div>
          <Space size={20} wrap className="profile-hero-meta">
            <span><UserOutlined /> {userInfo?.username}</span>
            {userInfo?.email && <span><MailOutlined /> {userInfo.email}</span>}
            {userInfo?.phone && <span><PhoneOutlined /> {userInfo.phone}</span>}
          </Space>
        </div>
      </div>

      <Row gutter={16}>
        <Col xs={24} lg={12}>
          <Card
            title="个人信息"
            className="glass-rise"
            style={{ marginBottom: 16, '--i': 0 } as React.CSSProperties}
          >
            <Form
              form={profileForm}
              layout="vertical"
              initialValues={{
                nickname: userInfo?.nickname,
                email: userInfo?.email,
                phone: userInfo?.phone,
              }}
            >
              <Form.Item name="nickname" label="昵称">
                <Input />
              </Form.Item>
              <Form.Item name="email" label="邮箱">
                <Input />
              </Form.Item>
              <Form.Item name="phone" label="手机号">
                <Input />
              </Form.Item>
              <Form.Item>
                <Button type="primary" onClick={handleSaveProfile} loading={profileLoading}>
                  保存
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card
            title="修改密码"
            className="glass-rise"
            style={{ marginBottom: 16, '--i': 1 } as React.CSSProperties}
          >
            <Form form={pwdForm} layout="vertical">
              <Form.Item name="old_password" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="new_password" label="新密码" rules={[{ required: true, message: '请输入新密码' }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item
                name="confirm_password"
                label="确认密码"
                dependencies={['new_password']}
                rules={[
                  { required: true, message: '请确认新密码' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('new_password') === value) {
                        return Promise.resolve()
                      }
                      return Promise.reject(new Error('两次输入的密码不一致'))
                    },
                  }),
                ]}
              >
                <Input.Password />
              </Form.Item>
              <Form.Item>
                <Button type="primary" onClick={handleChangePassword} loading={pwdLoading}>
                  保存
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>
      </Row>

      <Card
        title={<span><HistoryOutlined className="card-title-icon" />最近登录记录</span>}
        className="glass-rise"
        style={{ marginBottom: 16, '--i': 2 } as React.CSSProperties}
      >
        <Table
          rowKey="id"
          size="small"
          columns={loginLogColumns}
          dataSource={myLogs}
          loading={logsLoading}
          pagination={false}
        />
      </Card>

      <Card
        title={<span><SafetyCertificateOutlined className="card-title-icon" />两步验证 (TOTP)</span>}
        className="glass-rise"
        style={{ '--i': 3 } as React.CSSProperties}
      >
        <div className={`totp-panel ${userInfo?.totp_enabled ? 'totp-panel-on' : ''}`}>
          <div className={`totp-shield ${userInfo?.totp_enabled ? 'totp-shield-on' : ''}`}>
            <SafetyCertificateOutlined />
          </div>
          <div className="totp-panel-info">
            <div className="totp-panel-state">
              {userInfo?.totp_enabled ? '已启用' : '未启用'}
            </div>
            <div className="totp-panel-desc">
              {userInfo?.totp_enabled
                ? '登录时需要输入 Authenticator 动态验证码，账号受两步验证保护。'
                : '启用后，登录除密码外还需验证器动态验证码，可有效防止密码泄露带来的风险。'}
            </div>
          </div>
          <div className="totp-panel-actions">
            {userInfo?.totp_enabled ? (
              <Space>
                <Button danger onClick={() => { disableForm.resetFields(); setDisableTotpOpen(true) }}>
                  禁用 TOTP
                </Button>
                <Button onClick={() => { regenForm.resetFields(); setRegenOpen(true) }}>
                  重新生成恢复码
                </Button>
              </Space>
            ) : (
              <Button type="primary" onClick={openEnableTotp}>
                启用 TOTP
              </Button>
            )}
          </div>
        </div>
      </Card>

      <Modal
        title="启用 TOTP"
        open={enableTotpOpen}
        onCancel={() => setEnableTotpOpen(false)}
        footer={null}
        destroyOnHidden
        width={480}
      >
        <Steps
          current={enableStep}
          items={[{ title: '验证身份' }, { title: '扫描二维码' }]}
          style={{ marginBottom: 24 }}
        />
        {enableStep === 0 && (
          <Form form={enableForm} layout="vertical">
            <Form.Item
              name="current_password"
              label="当前密码"
              rules={[{ required: true, message: '请输入当前密码' }]}
            >
              <Input.Password />
            </Form.Item>
            <Form.Item>
              <Button type="primary" onClick={handleEnableNext} loading={enableLoading}>
                下一步
              </Button>
            </Form.Item>
          </Form>
        )}
        {enableStep === 1 && (
          <div>
            {qrCode && (
              <div style={{ textAlign: 'center', marginBottom: 16 }}>
                {/* 二维码需保持白底，暗色弹窗里包一层白色圆角容器 */}
                <div className="qr-white-box">
                  <img src={qrCode} alt="TOTP QR Code" style={{ width: 200, height: 200, display: 'block' }} />
                </div>
              </div>
            )}
            <Form form={enableCodeForm} layout="vertical">
              <Form.Item
                name="code"
                label="验证码"
                rules={[{ required: true, message: '请输入 6 位验证码' }]}
              >
                <Input maxLength={6} placeholder="请输入 Authenticator 中的 6 位验证码" />
              </Form.Item>
              <Form.Item>
                <Space>
                  <Button onClick={() => setEnableStep(0)}>上一步</Button>
                  <Button type="primary" onClick={handleEnableConfirm} loading={enableLoading}>
                    确认启用
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </div>
        )}
      </Modal>

      <Modal
        title="禁用 TOTP"
        open={disableTotpOpen}
        onOk={handleDisableTotp}
        onCancel={() => setDisableTotpOpen(false)}
        confirmLoading={disableLoading}
        destroyOnHidden
      >
        <Form form={disableForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="code"
            label="TOTP 验证码"
            rules={[{ required: true, message: '请输入验证码' }]}
          >
            <Input maxLength={6} />
          </Form.Item>
          <Form.Item
            name="current_password"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="重新生成恢复码"
        open={regenOpen}
        onOk={handleRegenCodes}
        onCancel={() => setRegenOpen(false)}
        confirmLoading={regenLoading}
        okText="确认生成"
        destroyOnHidden
      >
        <div className="modal-note modal-note-warn">重新生成后，旧的恢复码将全部失效。</div>
        <Form form={regenForm} layout="vertical">
          <Form.Item
            name="code"
            label="TOTP 验证码"
            rules={[{ required: true, message: '请输入验证码' }]}
          >
            <Input maxLength={6} />
          </Form.Item>
          <Form.Item
            name="current_password"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="请保存您的恢复码"
        open={!!recoveryCodes}
        onOk={() => setRecoveryCodes(null)}
        onCancel={() => setRecoveryCodes(null)}
        okText="我已保存"
        cancelButtonProps={{ style: { display: 'none' } }}
        maskClosable={false}
        width={440}
      >
        <div className="modal-note modal-note-danger">
          恢复码仅显示这一次。丢失验证器设备时，它是找回账号的唯一途径，请妥善离线保存。
        </div>
        <div
          className="cell-mono glass-well"
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: 8,
            padding: 16,
            fontSize: 14,
            textAlign: 'center',
          }}
        >
          {(recoveryCodes ?? []).map((c) => (
            <span key={c}>{c}</span>
          ))}
        </div>
        <Button block style={{ marginTop: 12 }} onClick={copyRecoveryCodes}>
          复制全部
        </Button>
      </Modal>
    </div>
  )
}
