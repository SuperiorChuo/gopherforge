import { useState } from 'react'
import {
  Card, Row, Col, Form, Input, Button, Descriptions, Tag,
  Modal, message, Steps, Space,
} from 'antd'
import { useAppSelector } from '@/hooks/store'
import {
  updateProfile, changePassword,
  generateTotpSetup, enableTotp, disableTotp,
} from '@/api/auth'
import { useAppDispatch } from '@/hooks/store'
import { fetchCurrentUser } from '@/store/slices/authSlice'

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

  const handleSaveProfile = async () => {
    try {
      const values = await profileForm.validateFields()
      setProfileLoading(true)
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
    try {
      const values = await pwdForm.validateFields()
      if (values.new_password !== values.confirm_password) {
        message.error('两次密码不一致')
        return
      }
      setPwdLoading(true)
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
    try {
      const { current_password } = await enableForm.validateFields()
      setEnableLoading(true)
      const res = await generateTotpSetup({ current_password }) as unknown as Record<string, unknown>
      setQrCode(String(res.qr_code ?? ''))
      setSetupPassword(current_password)
      setEnableStep(1)
    } catch {
      message.error('获取二维码失败')
    } finally {
      setEnableLoading(false)
    }
  }

  const handleEnableConfirm = async () => {
    try {
      const { code } = await enableCodeForm.validateFields()
      setEnableLoading(true)
      await enableTotp({ code, current_password: setupPassword })
      message.success('TOTP 已启用')
      setEnableTotpOpen(false)
      dispatch(fetchCurrentUser())
    } catch {
      message.error('验证失败')
    } finally {
      setEnableLoading(false)
    }
  }

  const handleDisableTotp = async () => {
    try {
      const values = await disableForm.validateFields()
      setDisableLoading(true)
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
      <Row gutter={16}>
        <Col xs={24} lg={12}>
          <Card title="个人信息" style={{ marginBottom: 16 }}>
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
          <Card title="修改密码" style={{ marginBottom: 16 }}>
            <Form form={pwdForm} layout="vertical">
              <Form.Item name="old_password" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="new_password" label="新密码" rules={[{ required: true, message: '请输入新密码' }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="confirm_password" label="确认密码" rules={[{ required: true, message: '请确认新密码' }]}>
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

      <Card title="两步验证 (TOTP)">
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="TOTP 状态">
            {userInfo?.totp_enabled ? (
              <Tag color="success">已启用</Tag>
            ) : (
              <Tag color="default">未启用</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="操作">
            {userInfo?.totp_enabled ? (
              <Button danger onClick={() => { disableForm.resetFields(); setDisableTotpOpen(true) }}>
                禁用 TOTP
              </Button>
            ) : (
              <Button type="primary" onClick={openEnableTotp}>
                启用 TOTP
              </Button>
            )}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Modal
        title="启用 TOTP"
        open={enableTotpOpen}
        onCancel={() => setEnableTotpOpen(false)}
        footer={null}
        destroyOnClose
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
                <img src={qrCode} alt="TOTP QR Code" style={{ width: 200, height: 200 }} />
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
        destroyOnClose
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
    </div>
  )
}
