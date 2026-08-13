import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Form, Input } from 'antd'
import { LockOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { resetPassword } from '@/api/auth'
import AuthShell from '@/components/AuthShell'

export default function ResetPasswordPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const token = new URLSearchParams(window.location.search).get('token') || ''

  const onFinish = async (values: { password: string }) => {
    setLoading(true)
    try {
      await resetPassword({ token, new_password: values.password })
      message.success(t('密码已重置，请用新密码登录'))
      navigate('/login')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('重置失败，链接可能已失效'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell title={t('设置新密码')} subtitle={t('新密码至少 8 位，需含大小写字母和数字。')}>
      {!token ? (
        <Alert
          className="login-inline-alert"
          type="warning"
          showIcon
          message={t('链接无效或已失效，请重新申请。')}
        />
      ) : (
        <Form onFinish={onFinish} size="large" className="login-form" requiredMark={false} disabled={loading}>
          <Form.Item
            name="password"
            rules={[
              { required: true, message: t('请输入新密码') },
              { min: 8, message: t('至少 8 位') },
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={t('新密码')}
              autoComplete="new-password"
              aria-label={t('新密码')}
            />
          </Form.Item>
          <Form.Item
            name="confirm"
            dependencies={['password']}
            rules={[
              { required: true, message: t('请再次输入新密码') },
              ({ getFieldValue }) => ({
                validator: (_, value) =>
                  !value || getFieldValue('password') === value
                    ? Promise.resolve()
                    : Promise.reject(new Error(t('两次输入的密码不一致'))),
              }),
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={t('确认新密码')}
              autoComplete="new-password"
              aria-label={t('确认新密码')}
            />
          </Form.Item>
          <Form.Item className="login-submit-item">
            <Button type="primary" htmlType="submit" block loading={loading}>
              {t('重置密码')}
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
