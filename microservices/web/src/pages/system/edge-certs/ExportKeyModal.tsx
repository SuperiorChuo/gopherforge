import { useEffect, useState } from 'react'
import { Alert, Form, Input, Modal, Typography } from 'antd'
import { message } from '@/utils/feedback'
import * as API from '@/api/system/edgeCert'
import { readError, saveBlob, type CertRow, type ExportKeyFormValues } from './helpers'

export interface ExportKeyModalProps {
  row: CertRow | null
  totpEnabled: boolean
  onClose: () => void
}

export function ExportKeyModal({ row, totpEnabled, onClose }: ExportKeyModalProps) {
  const [exporting, setExporting] = useState(false)
  const [form] = Form.useForm<ExportKeyFormValues>()

  useEffect(() => {
    if (row) form.resetFields()
  }, [form, row])

  const handleCancel = () => {
    form.resetFields()
    onClose()
  }

  const handleSubmit = async () => {
    if (!row) return
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setExporting(true)
    try {
      const proofResult = await API.requestEdgeCertExportStepUp({
        current_password: values.current_password,
        totp_code: values.totp_code || undefined,
        certificate_id: row.id,
      })
      const proof = proofResult.proof || proofResult.step_up_proof
      if (!proof) throw new Error('服务端未返回二次认证凭证')
      const blob = await API.exportEdgeCertPrivateKey(row.id, {
        step_up_proof: proof,
        confirm_domain: String(values.confirm_domain).trim().toLowerCase(),
      })
      saveBlob(blob, `${row.domain}.private-key.pem`)
      message.success('私钥已导出；请离线保管，使用后及时清理下载文件')
      form.resetFields()
      onClose()
    } catch (error) {
      message.error(readError(error, '私钥导出失败'))
    } finally {
      setExporting(false)
    }
  }

  return (
    <Modal
      title="导出证书私钥"
      open={!!row}
      onCancel={handleCancel}
      onOk={() => void handleSubmit()}
      okText="认证并导出私钥"
      okButtonProps={{ danger: true }}
      confirmLoading={exporting}
      maskClosable={false}
      keyboard={!exporting}
      destroyOnHidden
    >
      <Alert
        type="error"
        showIcon
        style={{ marginBottom: 16 }}
        message="私钥一旦泄露，任何人都可能冒充此域名"
        description="此操作会被审计。请仅在离线迁移或故障恢复时导出；不要通过聊天、工单或邮件传输。"
      />
      <Form form={form} layout="vertical" autoComplete="off">
        <Form.Item
          name="current_password"
          label="当前登录密码"
          rules={[{ required: true, message: '请输入当前登录密码' }]}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>
        {totpEnabled && (
          <Form.Item
            name="totp_code"
            label="TOTP 动态验证码"
            rules={[
              { required: true, message: '请输入动态验证码' },
              { pattern: /^\d{6}$/, message: '请输入 6 位数字验证码' },
            ]}
          >
            <Input inputMode="numeric" maxLength={6} autoComplete="one-time-code" placeholder="6 位数字" />
          </Form.Item>
        )}
        <Form.Item
          name="confirm_domain"
          label={<>输入完整域名确认：<Typography.Text code>{row?.domain}</Typography.Text></>}
          dependencies={[]}
          rules={[
            { required: true, message: '请输入完整域名' },
            {
              validator: (_, value) => String(value || '').trim().toLowerCase() === row?.domain
                ? Promise.resolve()
                : Promise.reject(new Error('域名不一致')),
            },
          ]}
        >
          <Input autoComplete="off" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
