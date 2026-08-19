import { useTranslation } from 'react-i18next'
import { Tabs } from 'antd'
import ListPageShell from '@/components/common/ListPageShell'
import ChannelTab from './ChannelTab'
import TemplateTab from './TemplateTab'
import LogTab from './LogTab'

export default function SmsPage() {
  const { t } = useTranslation()
  return (
    <ListPageShell className="sms-page" toolbar={null}>
      <Tabs
        defaultActiveKey="channel"
        items={[
          { key: 'channel', label: t('短信渠道'), children: <ChannelTab /> },
          { key: 'template', label: t('短信模板'), children: <TemplateTab /> },
          { key: 'log', label: t('发送日志'), children: <LogTab /> },
        ]}
      />
    </ListPageShell>
  )
}
