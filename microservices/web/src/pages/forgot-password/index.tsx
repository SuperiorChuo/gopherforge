import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Form, Input } from 'antd'
import { MailOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { forgotPassword } from '@/api/auth'
import AuthShell from '@/components/AuthShell'

export default function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)

  const onFinish = async (values: { email: string }) => {
    setLoading(true)
    try {
      await forgotPassword({ email: values.email })
      setSent(true)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('发送失败，请稍后再试'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell title={t('忘记密码')} subtitle={t('输入注册邮箱，我们将发送密码重置链接（30 分钟内有效）。')}>
      {sent ? (
        <Alert
          className="login-inline-alert"
          type="success"
          showIcon
          message={t('重置邮件已发送')}
          description={t('如果该邮箱存在，你会收到一封含重置链接的邮件。请检查收件箱（含垃圾邮件）。')}
        />
      ) : (
        <Form onFinish={onFinish} size="large" className="login-form" requiredMark={false} disabled={loading}>
          <Form.Item
            name="email"
            rules={[
              { required: true, message: t('请输入邮箱') },
              { type: 'email', message: t('邮箱格式不正确') },
            ]}
          >
            <Input prefix={<MailOutlined />} placeholder={t('注册邮箱')} autoComplete="email" aria-label={t('注册邮箱')} />
          </Form.Item>
          <Form.Item className="login-submit-item">
            <Button type="primary" htmlType="submit" block loading={loading}>
              {t('发送重置链接')}
            </Button>
          </Form.Item>
        </Form>
      )}
      <div className="login-form-links">
        <Link to="/login" className="login-back-link">
          {t('返回登录')}
        </Link>
      </div>
    </AuthShell>
  )
}
