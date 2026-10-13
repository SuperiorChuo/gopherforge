import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Space, Tabs } from 'antd'
import { StopOutlined, SafetyCertificateOutlined, ClockCircleOutlined } from '@ant-design/icons'
import request from '@/utils/request'
import InboundBlacklistTab from './inbound-tab'
import OutboundListsTab from './lists-tab'
import OutboundPolicyTab from './policy-tab'

export default function CcBlacklistPage() {
  const { t } = useTranslation()
  const [isPlatform, setIsPlatform] = useState(false)

  useEffect(() => {
    request.get('/api/v1/cc/fs/blacklist', { silent: true })
      .then(() => setIsPlatform(true))
      .catch(() => setIsPlatform(false))
  }, [])

  return (
    <div className="page-list cc-blacklist-page">
      <Tabs
        defaultActiveKey="lists"
        items={[
          ...(isPlatform
            ? [{
                key: 'inbound',
                label: <Space size={6}><StopOutlined />{t('来电黑名单')}</Space>,
                children: <InboundBlacklistTab />,
              }]
            : []),
          {
            key: 'lists',
            label: <Space size={6}><SafetyCertificateOutlined />{t('外呼名单')}</Space>,
            children: <OutboundListsTab />,
          },
          {
            key: 'policy',
            label: <Space size={6}><ClockCircleOutlined />{t('外呼策略')}</Space>,
            children: <OutboundPolicyTab />,
          },
        ]}
      />
    </div>
  )
}
