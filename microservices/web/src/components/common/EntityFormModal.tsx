import type { ReactNode } from 'react'
import { Form, Modal, type FormInstance, type FormProps, type ModalProps } from 'antd'

export type EntityFormModalProps<Values extends object = Record<string, unknown>> =
  Omit<ModalProps, 'children' | 'open' | 'title' | 'onOk' | 'onCancel' | 'confirmLoading'> & {
    title: ReactNode
    open: boolean
    form: FormInstance<Values>
    onClose: () => void
    onSubmit: () => void | Promise<void>
    submitting?: boolean
    children: ReactNode
    formProps?: Omit<FormProps<Values>, 'form' | 'children' | 'layout'>
  }

/** 统一实体编辑弹窗：负责弹窗确认态、关闭动作与垂直表单容器。 */
export default function EntityFormModal<Values extends object = Record<string, unknown>>({
  title,
  open,
  form,
  onClose,
  onSubmit,
  submitting = false,
  children,
  formProps,
  ...modalProps
}: EntityFormModalProps<Values>) {
  return (
    <Modal
      {...modalProps}
      title={title}
      open={open}
      onOk={() => void onSubmit()}
      onCancel={onClose}
      confirmLoading={submitting}
      destroyOnHidden
    >
      <Form {...formProps} form={form} layout="vertical">
        {children}
      </Form>
    </Modal>
  )
}
