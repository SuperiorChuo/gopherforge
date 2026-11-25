import { useEffect, useMemo, useState } from 'react'
import { Alert, Form, Input, Modal, Select, Switch } from 'antd'
import { message } from '@/utils/feedback'
import * as API from '@/api/system/edgeCert'
import {
  DOMAIN_PATTERN,
  getCertFormInitialValues,
  hasCertificate,
  type CertFormValues,
  type CertRow,
} from './helpers'

export interface CertFormModalProps {
  open: boolean
  editingRow: CertRow | null
  onClose: () => void
  onSaved: () => Promise<void>
}

export function CertFormModal({ open, editingRow, onClose, onSaved }: CertFormModalProps) {
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm<CertFormValues>()
  const deploymentMode = Form.useWatch('deployment_mode', form)
  const initialValues = useMemo(() => getCertFormInitialValues(editingRow), [editingRow])

  useEffect(() => {
    if (!open) return
    form.setFieldsValue(initialValues)
  }, [form, initialValues, open])

  const handleCancel = () => {
    form.resetFields()
    onClose()
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setCreating(true)
    try {
      await API.createEdgeCert({
        domain: String(values.domain).trim().toLowerCase(),
        email: String(values.email).trim(),
        is_staging: !!values.is_staging,
        deployment_mode: values.deployment_mode,
        deployment_provider: values.deployment_provider,
        auto_renew_enabled: !!values.auto_renew_enabled,
      })
      message.success(editingRow ? '证书管理配置已更新' : '域名已保存，尚未签发或部署')
      form.resetFields()
      onClose()
      await onSaved()
    } catch {
      message.error('保存域名失败')
    } finally {
      setCreating(false)
    }
  }

  return (
    <Modal
      title={editingRow ? '编辑证书管理配置' : '添加域名'}
      open={open}
      onCancel={handleCancel}
      onOk={() => void handleSubmit()}
      confirmLoading={creating}
      okText="保存域名"
      destroyOnHidden
    >
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="保存后不会自动签发或修改网关"
      />
      <Form form={form} layout="vertical" initialValues={initialValues}>
        <Form.Item
          name="domain"
          label="域名"
          normalize={(value) => String(value || '').trim().toLowerCase()}
          rules={[
            { required: true, message: '请输入域名' },
            { pattern: DOMAIN_PATTERN, message: '请输入单个完整域名，不含协议、端口或路径' },
          ]}
        >
          <Input placeholder="如 admin.example.com" autoComplete="off" disabled={!!editingRow} />
        </Form.Item>
        <Form.Item name="email" label="ACME 通知邮箱" rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}>
          <Input placeholder="用于 CA 账号与到期通知" autoComplete="email" />
        </Form.Item>
        <Form.Item name="is_staging" label="使用 Staging（建议先验证）" valuePropName="checked">
          <Switch
            checkedChildren="测试"
            unCheckedChildren="正式"
            disabled={deploymentMode === 'traefik_file' || (!!editingRow && hasCertificate(editingRow))}
          />
        </Form.Item>
        <Form.Item
          name="deployment_mode"
          label="证书部署方式"
          rules={[{ required: true, message: '请选择证书部署方式' }]}
        >
          <Select
            options={[
              { value: 'external', label: '外部托管（Caddy / CDN）' },
              { value: 'traefik_file', label: '由本系统部署到 Traefik' },
            ]}
            onChange={(value) => {
              if (value === 'traefik_file') {
                form.setFieldsValue({
                  is_staging: false,
                  deployment_provider: 'traefik',
                  auto_renew_enabled: true,
                })
              } else {
                form.setFieldsValue({
                  deployment_provider: 'caddy',
                  auto_renew_enabled: false,
                })
              }
            }}
          />
        </Form.Item>
        {deploymentMode === 'traefik_file' ? (
          <>
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message="本系统将写入 Traefik 动态证书配置"
              description="仅用于 DNS 与流量确实指向本机 Traefik 的生产域名；当前由 Caddy 托管的域名不要选择此项。"
            />
            <Form.Item name="auto_renew_enabled" label="自动续期" valuePropName="checked">
              <Switch checkedChildren="开启" unCheckedChildren="关闭" />
            </Form.Item>
          </>
        ) : (
          <Form.Item name="deployment_provider" label="外部 TLS 提供方" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'caddy', label: 'Caddy' },
                { value: 'cdn', label: 'CDN / 云网关' },
                { value: 'other', label: '其他外部提供方' },
                { value: 'external', label: '未指定' },
              ]}
            />
          </Form.Item>
        )}
      </Form>
    </Modal>
  )
}
