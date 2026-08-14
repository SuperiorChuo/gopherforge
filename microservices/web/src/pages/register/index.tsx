import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Form, Input } from 'antd'
import { LockOutlined, MailOutlined, UserOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { register } from '@/api/auth'
import AuthShell from '@/components/auth/AuthShell'

// 邀请注册：仅受邀用户可通过邀请链接进入（?invite=<token>），无公开自注册。
export default function RegisterPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const inviteToken = new URLSearchParams(window.location.search).get('invite') || ''
  const [form] = Form.useForm()

  const onFinish = async (values: { username: string; password: string; email: string }) => {
    setLoading(true)
    try {
      await register({
        username: values.username,
        password: values.password,
        email: values.email,
        invite_token: inviteToken,
      })
      message.success(t('注册成功，请登录'))
      navigate('/login')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('注册失败'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell title={t('注册账号')} subtitle={t('仅限受邀用户')}>
      {!inviteToken ? (
        <Alert
          className="login-inline-alert"
          type="warning"
          showIcon
          message={t('注册需要有效的邀请链接')}
          description={t('请通过管理员发送的邀请链接访问本页；首次注册成功后需先登录修改初始密码。')}
        />
      ) : (
        <Form
          form={form}
          size="large"
          className="login-form"
          requiredMark={false}
          onFinish={onFinish}
          disabled={loading}
        >
          <Form.Item name="username" rules={[{ required: true, message: t('请输入用户名') }]}>
            <Input prefix={<UserOutlined />} placeholder={t('用户名')} autoComplete="username" aria-label={t('用户名')} />
          </Form.Item>
          <Form.Item
            name="email"
            rules={[
              { required: true, message: t('请输入有效邮箱') },
              { type: 'email', message: t('请输入有效邮箱') },
            ]}
          >
            <Input prefix={<MailOutlined />} placeholder={t('邮箱')} autoComplete="email" aria-label={t('邮箱')} />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, min: 8, message: t('至少 8 位，需含大小写与数字') }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={t('密码')}
              autoComplete="new-password"
              aria-label={t('密码')}
            />
          </Form.Item>
          <Form.Item
            name="confirm"
            dependencies={['password']}
            rules={[
              { required: true, message: t('请再次输入密码') },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) return Promise.resolve()
                  return Promise.reject(new Error(t('两次输入的密码不一致')))
                },
              }),
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={t('确认密码')}
              autoComplete="new-password"
              aria-label={t('确认密码')}
            />
          </Form.Item>
          <Form.Item className="login-submit-item">
            <Button type="primary" htmlType="submit" block loading={loading}>
              {t('注册')}
            </Button>
          </Form.Item>
        </Form>
      )}
      <div className="login-form-links">
        <Link to="/login" className="login-back-link">
          {t('已有账号？去登录')}
        </Link>
      </div>
    </AuthShell>
  )
}
