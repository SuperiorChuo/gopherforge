// 收银台：匿名无登录路由（/cashier/:token），token 即凭据。
// 可用渠道与「模拟支付」按钮全部由后端能力决定（channels / mock_enabled）：
// 这是面向真实付款人的匿名页面，不能用前端开关判断是否演示环境——生产下后端
// 不注册 mock 渠道，模拟支付端点连路由都没有，这里自然渲染不出按钮。
// 真渠道（M2）出二维码/跳转。视觉照登录页深空玻璃拟态范本，浅色兜底。
import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Result, Space, Spin, Tag, Typography } from 'antd'
import { CheckCircleFilled, PayCircleOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { formatCents, payApi, type CashierInfo } from '@/api/pay'

const { Title, Text } = Typography

const CHANNEL_NAME: Record<string, string> = {
  mock: '模拟支付（演示）',
  wechat_native: '微信扫码支付',
  wechat_jsapi: '微信支付',
  alipay_pc: '支付宝',
  alipay_wap: '支付宝',
}

export default function CashierPage() {
  const { token = '' } = useParams()
  const { t } = useTranslation()
  const [info, setInfo] = useState<CashierInfo | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [channel, setChannel] = useState<string>()
  const [paying, setPaying] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await payApi.cashierGet(token)
      setInfo(data)
      if (!channel && data.channels.length > 0) setChannel(data.channel_code || data.channels[0].code)
      return data
    } catch {
      setNotFound(true)
      return null
    }
  }, [token, channel])

  useEffect(() => {
    void load()
  }, [load])

  // 支付中轮询结果（3s；终态即停）
  useEffect(() => {
    if (info?.status !== 'paying') return
    pollRef.current = setInterval(() => {
      void load().then((d) => {
        if (d && d.status !== 'paying' && d.status !== 'waiting' && pollRef.current) {
          clearInterval(pollRef.current)
        }
      })
    }, 3000)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [info?.status, load])

  const startPay = async () => {
    if (!channel) return
    setPaying(true)
    try {
      await payApi.cashierPay(token, channel)
      await load()
    } finally {
      setPaying(false)
    }
  }

  const mockPay = async (succeed: boolean) => {
    setPaying(true)
    try {
      await payApi.cashierMock(token, succeed)
      if (!succeed) message.info(t('已模拟支付失败，订单保持支付中'))
      await load()
    } finally {
      setPaying(false)
    }
  }

  if (notFound) {
    return (
      <CashierShell>
        <Result status="404" title={t('订单不存在或链接已失效')} />
      </CashierShell>
    )
  }
  if (!info) {
    return (
      <CashierShell>
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      </CashierShell>
    )
  }

  if (info.status === 'paid') {
    return (
      <CashierShell>
        <Result
          icon={<CheckCircleFilled style={{ color: 'var(--c-success)' }} />}
          title={t('支付成功')}
          subTitle={`${info.subject} · ${formatCents(info.amount_cents)}`}
        />
      </CashierShell>
    )
  }
  if (info.status === 'closed') {
    return (
      <CashierShell>
        <Result status="warning" title={t('订单已关闭')} subTitle={t('请返回业务页面重新发起支付')} />
      </CashierShell>
    )
  }

  const expired = new Date(info.expire_at).getTime() < Date.now()

  return (
    <CashierShell>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <Text type="secondary">{info.subject}</Text>
        <Title level={1} style={{ margin: '8px 0 0' }}>{formatCents(info.amount_cents)}</Title>
        <Tag style={{ marginTop: 8 }}>{info.order_no}</Tag>
      </div>
      {expired ? (
        <Alert type="warning" showIcon message={t('订单已过期，即将自动关闭')} />
      ) : (
        <>
          <div style={{ marginBottom: 16 }}>
            {info.channels.length === 0 ? (
              <Alert type="error" showIcon message={t('暂无可用支付渠道，请联系管理员配置')} />
            ) : (
              <Space direction="vertical" style={{ width: '100%' }}>
                {info.channels.map((ch) => (
                  <Card
                    key={ch.code}
                    size="small"
                    hoverable
                    onClick={() => setChannel(ch.code)}
                    style={{
                      borderColor: channel === ch.code ? 'var(--c-primary)' : undefined,
                      borderWidth: channel === ch.code ? 2 : 1,
                    }}
                  >
                    <Space>
                      <PayCircleOutlined style={{ fontSize: 20, color: 'var(--c-primary)' }} />
                      <span>{ch.name || t(CHANNEL_NAME[ch.code] ?? '') || ch.code}</span>
                    </Space>
                  </Card>
                ))}
              </Space>
            )}
          </div>
          {info.status === 'paying' && channel === 'mock' && info.mock_enabled ? (
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert type="warning" showIcon message={t('演示渠道：这里的按钮直接改订单状态，不产生任何真实收款')} />
              <Button type="primary" size="large" block loading={paying} onClick={() => void mockPay(true)}>
                {t('模拟支付成功')}
              </Button>
              <Button size="large" block disabled={paying} onClick={() => void mockPay(false)}>
                {t('模拟支付失败')}
              </Button>
            </Space>
          ) : (
            <Button
              type="primary"
              size="large"
              block
              disabled={!channel || info.channels.length === 0}
              loading={paying}
              onClick={() => void startPay()}
            >
              {info.status === 'paying' ? t('重新拉起支付') : t('立即支付')}
            </Button>
          )}
        </>
      )}
    </CashierShell>
  )
}

function CashierShell({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'radial-gradient(120% 120% at 20% 0%, #1b2a5e 0%, #0d1330 45%, #070a1c 100%)',
        padding: 16,
      }}
    >
      <Card
        style={{
          width: 420,
          maxWidth: '100%',
          background: 'rgba(255,255,255,0.06)',
          backdropFilter: 'blur(18px)',
          border: '1px solid rgba(255,255,255,0.12)',
          borderRadius: 16,
        }}
        styles={{ body: { padding: 32 } }}
      >
        {children}
      </Card>
    </div>
  )
}
