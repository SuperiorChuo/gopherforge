import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Button, Space, Tag, Typography } from 'antd'
import { PhoneOutlined } from '@ant-design/icons'
import { notification, message } from '@/utils/feedback'
import { useAppSelector } from '@/hooks/store'
import type { MenuItem } from '@/types'
import { subscribeCcEvents, type CcCallEvent } from '@/utils/ccSocket'
import { createLead, lookupPhone, LEAD_STATUS_META, type PhoneLookup } from '@/api/crm'

const { Text } = Typography
const LEVEL_COLORS: Record<string, string> = { A: 'gold', B: 'blue', C: 'default' }

// 全局来电弹屏（CC×CRM 联动）：坐席在任意页面都能毫秒级收到 inbound ringing 弹屏，
// 按主叫号码识别 CRM 归属（客户/线索/陌生）给出对应快捷动作。挂在 MainLayout 内、
// 鉴权就绪后渲染；仅对有呼叫中心菜单权限的用户建 WS（避免全员空连）。

function hasCcMenu(menus: MenuItem[]): boolean {
  for (const m of menus) {
    const p = m.path || ''
    if (p === 'cc' || p.startsWith('cc/') || p.startsWith('/cc')) return true
    if (m.children?.length && hasCcMenu(m.children)) return true
  }
  return false
}

export default function CcCallNotifier() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const menus = useAppSelector((s) => s.auth.menus)
  const roles = useAppSelector((s) => s.auth.userInfo?.roles)
  const isSuperAdmin = roles?.some((r) => r.code === 'super_admin') ?? false
  const enabled = isSuperAdmin || hasCcMenu(menus)

  useEffect(() => {
    if (!enabled) return
    const unsub = subscribeCcEvents((ev: CcCallEvent) => {
      // 只对入方向来电弹屏；接通/挂断由页面自身事件驱动刷新，不打扰坐席。
      if (ev.type !== 'call.ringing' || ev.direction !== 'inbound') return
      void popFor(ev)
    })

    async function popFor(ev: CcCallEvent) {
      const caller = ev.caller || ''
      // CRM 识客：任一服务不可用也弹基础信息，不阻塞弹屏。
      let lookup: PhoneLookup = { matched: 'none' }
      try {
        if (caller) lookup = await lookupPhone(caller)
      } catch {
        /* crm 不可用，退化为陌生号码弹屏 */
      }

      const key = `cc-call-${ev.call_id}`
      const goWith = (path: string) => {
        notification?.destroy(key)
        navigate(path)
      }
      const openWorkbench = () => goWith(`/cc/workbench?phone=${encodeURIComponent(caller)}`)

      let title: React.ReactNode
      let actions: React.ReactNode
      if (lookup.matched === 'customer' && lookup.customer) {
        const cu = lookup.customer
        title = (
          <Space size={6} wrap>
            <Text strong>{cu.name}</Text>
            <Tag color={LEVEL_COLORS[cu.level] || 'default'} variant="filled">{t('{{level}} 级客户', { level: cu.level })}</Tag>
            {lookup.contact ? <Text type="secondary">{t('联系人：{{name}}', { name: lookup.contact.name })}</Text> : null}
          </Space>
        )
        actions = (
          <Space wrap>
            <Button size="small" type="primary" onClick={openWorkbench}>{t('坐席工作台')}</Button>
            <Button size="small" onClick={() => goWith(`/crm/customers?keyword=${encodeURIComponent(caller)}`)}>
              {t('查看客户')}
            </Button>
          </Space>
        )
      } else if (lookup.matched === 'lead' && lookup.lead) {
        const meta = LEAD_STATUS_META[lookup.lead.status] ?? { label: lookup.lead.status, color: 'default' }
        title = (
          <Space size={6} wrap>
            <Text strong>{t('线索：{{name}}', { name: lookup.lead.name })}</Text>
            <Tag color={meta.color} variant="filled">{t(meta.label)}</Tag>
          </Space>
        )
        actions = (
          <Space wrap>
            <Button size="small" type="primary" onClick={openWorkbench}>{t('坐席工作台')}</Button>
            <Button size="small" onClick={() => goWith(`/crm/leads?keyword=${encodeURIComponent(caller)}`)}>
              {t('查看线索')}
            </Button>
          </Space>
        )
      } else {
        const who = ev.caller_name ? `${caller} ${ev.caller_name}` : caller || t('未知号码')
        title = (
          <Space direction="vertical" size={2}>
            <Text strong>{who}</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>{t('CRM 中未找到该号码')}</Text>
          </Space>
        )
        actions = (
          <Space wrap>
            <Button size="small" type="primary" onClick={openWorkbench}>{t('坐席工作台')}</Button>
            <Button
              size="small"
              onClick={async () => {
                try {
                  await createLead({ name: t('来电 {{number}}', { number: caller }), phone: caller, source: 'call' })
                  message.success(t('线索已创建'))
                  goWith(`/crm/leads?keyword=${encodeURIComponent(caller)}`)
                } catch (e: unknown) {
                  message.error(e instanceof Error ? e.message : t('创建失败'))
                }
              }}
            >
              {t('一键建线索')}
            </Button>
          </Space>
        )
      }

      notification?.open({
        key,
        message: (
          <Space size={8}>
            <PhoneOutlined style={{ color: 'var(--c-cyan)' }} />
            <span>{t('来电')} <span className="cell-mono">{caller || t('未知号码')}</span></span>
          </Space>
        ),
        description: (
          <Space direction="vertical" size={6} style={{ width: '100%' }}>
            {title}
            {ev.queue ? <span style={{ opacity: 0.7 }}>{t('队列：{{queue}}', { queue: ev.queue })}</span> : null}
            {actions}
          </Space>
        ),
        duration: 15,
        placement: 'topRight',
      })
    }

    return unsub
  }, [enabled, navigate, t])

  return null
}
