import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Alert, Button, Card, Form, Input } from 'antd'
import { MailOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { forgotPassword } from '@/api/auth'

export default function ForgotPasswordPage() {
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)

  const onFinish = async (values: { email: string }) => {
    setLoading(true)
    try {
      await forgotPassword({ email: values.email })
      setSent(true)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '发送失败，请稍后再试')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-shell">
      <Card className="login-card" style={{ width: 380 }}>
        <h2 className="login-title">忘记密码</h2>
        <p className="login-subtitle">输入注册邮箱，我们将发送密码重置链接（30 分钟内有效）。</p>
        {sent ? (
          <Alert
            type="success"
            showIcon
            message="重置邮件已发送"
            description="如果该邮箱存在，你会收到一封含重置链接的邮件。请检查收件箱（含垃圾邮件）。"
          />
        ) : (
          <Form onFinish={onFinish} layout="vertical" size="large" disabled={loading}>
            <Form.Item
              name="email"
              rules={[{ required: true, message: '请输入邮箱' }, { type: 'email', message: '邮箱格式不正确' }]}
            >
              <Input prefix={<MailOutlined />} placeholder="注册邮箱" autoComplete="email" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>
              发送重置链接
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
