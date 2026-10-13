import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd'
import { PhoneOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { fsOriginate, listFsGateways, type FsGateway } from '@/api/cc'

const { Text } = Typography
const EXT_KEY = 'crm_dial_ext'
const GW_KEY = 'crm_dial_gateway'

// CRM 点击外呼（CC×CRM）：选坐席分机与 SIP 线路 → 分机先振铃，接起后
// 桥接客户号码。分机与线路选择记忆在本地。
export default function CrmDialButton({ phone, label = '外呼', size = 'small', block = false }: {
  phone?: string
  label?: string
  size?: 'small' | 'middle'
  block?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [gateways, setGateways] = useState<FsGateway[]>([])
  const [calling, setCalling] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    if (!open) return
    listFsGateways()
      .then((r) => setGateways((r.list ?? []).filter((g) => g.state !== 'NOT_LOADED')))
      .catch(() => setGateways([]))
    form.setFieldsValue({
      from_ext: localStorage.getItem(EXT_KEY) || '',
      gateway: localStorage.getItem(GW_KEY) || '',
    })
  }, [open, form])

  async function onDial() {
    const values = await form.validateFields()
    setCalling(true)
    try {
      await fsOriginate(values.from_ext, (phone || '').replace(/[^0-9*#+]/g, ''), values.gateway || undefined)
      localStorage.setItem(EXT_KEY, values.from_ext)
      localStorage.setItem(GW_KEY, values.gateway || '')
      message.success(t('呼叫已发起：分机 {{ext}} 即将振铃，接起后接通 {{phone}}', { ext: values.from_ext, phone }))
      setOpen(false)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('外呼失败'))
    } finally {
      setCalling(false)
    }
  }

  return (
    <>
      <Button type="link" size={size} icon={<PhoneOutlined />} disabled={!phone} block={block}
        onClick={(e) => { e.stopPropagation(); setOpen(true) }}>
        {label ? label : t('外呼')}
      </Button>
      <Modal title={t('外呼 {{phone}}', { phone: phone ?? '' })} open={open} onCancel={() => setOpen(false)}
        onOk={() => void onDial()} okText={t('呼叫')} confirmLoading={calling} destroyOnHidden width={420}>
        <Text type="secondary">{t('你的分机先振铃，接起后自动拨通客户。')}</Text>
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="from_ext" label={t('我的分机')} rules={[
            { required: true, message: t('请输入分机号') },
            { pattern: /^[0-9]{2,32}$/, message: t('分机号为数字') },
          ]}>
            <Input placeholder={t('如 1001')} />
          </Form.Item>
          <Form.Item name="gateway" label={t('外呼线路')}
            extra={t('不选则走系统默认拨号计划（内线/默认路由）')}>
            <Select allowClear placeholder={t('选择 SIP 线路')}
              options={gateways.map((g) => ({
                value: g.name,
                label: (
                  <Space size={6}>
                    {g.name}
                    <Tag variant="filled" color={g.state === 'REGED' ? 'success' : g.state === 'NOREG' ? 'orange' : 'red'}>
                      {g.state || t('未知')}
                    </Tag>
                  </Space>
                ),
              }))} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
