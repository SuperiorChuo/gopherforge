import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Form, Input, Typography } from 'antd'
import { LockOutlined, MailOutlined, UserOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { register } from '@/api/auth'

// 邀请注册：仅受邀用户可通过邀请链接进入（?invite=<token>），无公开自注册。
export default function RegisterPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const inviteToken = new URLSearchParams(window.location.search).get('invite') || ''
  const [form] = Form.useForm()

  const onFinish = async (values: { username: string; password: string; email: string }) => {
    setLoading(true)
    try {
      await register({ username: values.username, password: values.password, email: values.email, invite_token: inviteToken })
      message.success('注册成功，请登录')
      navigate('/login')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '注册失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-shell">
      <Card className="login-card" style={{ width: 380 }}>
        <Typography.Title level={3} style={{ textAlign: 'center', marginBottom: 4 }}>
          注册账号
        </Typography.Title>
        <Typography.Text type="secondary" style={{ display: 'block', textAlign: 'center', marginBottom: 16 }}>
          仅限受邀用户
        </Typography.Text>
        {!inviteToken ? (
          <Alert
            type="warning"
            showIcon
            message="注册需要有效的邀请链接"
            description="请通过管理员发送的邀请链接访问本页；首次注册成功后需先登录修改初始密码。"
          />
        ) : (
          <Form form={form} layout="vertical" onFinish={onFinish}>
            <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
              <Input prefix={<UserOutlined />} placeholder="用户名" />
            </Form.Item>
            <Form.Item name="email" label="邮箱" rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}>
              <Input prefix={<MailOutlined />} placeholder="邮箱" />
            </Form.Item>
            <Form.Item name="password" label="密码" rules={[{ required: true, min: 8, message: '至少 8 位，需含大小写与数字' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="密码" />
            </Form.Item>
            <Form.Item
              name="confirm"
              label="确认密码"
              dependencies={['password']}
              rules={[
                { required: true, message: '请再次输入密码' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('password') === value) return Promise.resolve()
                    return Promise.reject(new Error('两次输入的密码不一致'))
                  },
                }),
              ]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="确认密码" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>
              注册
            </Button>
            <div style={{ textAlign: 'center', marginTop: 12 }}>
              <Link to="/login">已有账号？去登录</Link>
            </div>
          </Form>
        )}
      </Card>
    </div>
  )
}
