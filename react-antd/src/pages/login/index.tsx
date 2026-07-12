import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Form, Input, Button, Card, Typography, message, Alert } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { useAppDispatch, useAppSelector } from '@/hooks/store'
import { login } from '@/store/slices/authSlice'

const { Title } = Typography

export default function LoginPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { loading } = useAppSelector((s) => s.auth)
  const [error, setError] = useState<string | null>(null)
  const [totpStep, setTotpStep] = useState(false)
  const [challengeId, setChallengeId] = useState<string | null>(null)

  const onFinish = async (values: { username: string; password: string }) => {
    setError(null)
    try {
      const result = await dispatch(login(values)).unwrap()
      if (result.require_totp && result.totp_challenge_id) {
        setChallengeId(result.totp_challenge_id)
        setTotpStep(true)
        return
      }
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '用户名或密码错误'
      setError(msg)
    }
  }

  const onTotpFinish = async (values: { code: string }) => {
    if (!challengeId) return
    setError(null)
    try {
      const { verifyTotpLogin } = await import('@/api/auth')
      await verifyTotpLogin({ challenge_id: challengeId, code: values.code })
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch {
      setError('验证码错误，请重试')
    }
  }

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(135deg, #1677ff 0%, #003eb3 100%)',
    }}>
      <Card style={{ width: 400, borderRadius: 12, boxShadow: '0 8px 32px rgba(0,0,0,0.15)' }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={3} style={{ marginBottom: 4, color: '#1677ff' }}>Go Admin Kit</Title>
          <Typography.Text type="secondary">
            {totpStep ? '请输入两步验证码' : '欢迎回来，请登录'}
          </Typography.Text>
        </div>

        {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} closable onClose={() => setError(null)} />}

        {!totpStep ? (
          <Form name="login" onFinish={onFinish} autoComplete="off" size="large">
            <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
              <Input prefix={<UserOutlined />} placeholder="用户名" />
            </Form.Item>
            <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="密码" />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={loading} block>
                登录
              </Button>
            </Form.Item>
          </Form>
        ) : (
          <Form name="totp" onFinish={onTotpFinish} autoComplete="off" size="large">
            <Form.Item name="code" rules={[{ required: true, message: '请输入验证码' }]}>
              <Input placeholder="6 位验证码" maxLength={6} style={{ letterSpacing: 4, textAlign: 'center' }} />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" block>验证</Button>
            </Form.Item>
            <Button type="link" block onClick={() => { setTotpStep(false); setChallengeId(null) }}>
              返回登录
            </Button>
          </Form>
        )}
      </Card>
    </div>
  )
}
