import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button, Card, Form, Input } from 'antd'
import { LockOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { resetPassword } from '@/api/auth'

export default function ResetPasswordPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const token = new URLSearchParams(window.location.search).get('token') || ''

  const onFinish = async (values: { password: string }) => {
    setLoading(true)
    try {
      await resetPassword({ token, new_password: values.password })
      message.success('密码已重置，请用新密码登录')
      navigate('/login')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '重置失败，链接可能已失效')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-shell">
      <Card className="login-card" style={{ width: 380 }}>
        <h2 className="login-title">设置新密码</h2>
        <p className="login-subtitle">新密码至少 8 位，需含大小写字母和数字。</p>
        {!token ? (
          <div style={{ color: 'var(--text-secondary)' }}>链接无效或已失效，请重新申请。</div>
        ) : (
          <Form onFinish={onFinish} layout="vertical" size="large" disabled={loading}>
            <Form.Item
              name="password"
              rules={[{ required: true, message: '请输入新密码' }, { min: 8, message: '至少 8 位' }]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="新密码" autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              name="confirm"
              dependencies={['password']}
              rules={[
                { required: true, message: '请再次输入新密码' },
                ({ getFieldValue }) => ({
                  validator: (_, value) =>
                    !value || getFieldValue('password') === value
                      ? Promise.resolve()
                      : Promise.reject(new Error('两次输入的密码不一致')),
                }),
              ]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="确认新密码" autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>
              重置密码
            </Button>
          </Form>
        )}
        <div style={{ marginTop: 16, textAlign: 'center' }}>
          <Link to="/login">返回登录</Link>
        </div>
      </Card>
    </div>
  )
}
