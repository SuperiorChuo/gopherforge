import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Input, Form, Badge, Tooltip, Modal, Switch, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, UserOutlined, PlusOutlined, DeleteOutlined, MailOutlined, EditOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listFsRegistrations, listFsExtensions, createFsExtension, updateFsExtension, deleteFsExtension,
  type FsRegistration, type FsExtension,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

type ExtFormValues = {
  ext: string
  password: string
  label?: string
  vm_enabled?: boolean
  vm_password?: string
  dir_first_name?: string
  dir_last_name?: string
}

export default function CcExtensionsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [regs, setRegs] = useState<FsRegistration[]>([])
  const [modalOpen, setModalOpen] = useState(false)
  const [editingExt, setEditingExt] = useState<FsExtension | null>(null)
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm<ExtFormValues>()
  // 随机密码：去易混淆字符（0/O/1/l/I）的字母数字，crypto 强随机。
  // 创建分机时密码必须自定义或随机——不提供任何默认密码（弱口令注册风险）。
  const genRandomPassword = useCallback(() => {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789'
    const arr = new Uint32Array(12)
    crypto.getRandomValues(arr)
    return Array.from(arr, (n) => chars[n % chars.length]).join('')
  }, [])
  // 语音信箱开关的实时值，控制密码输入框显隐
  const vmEnabled = Form.useWatch('vm_enabled', form)

  const fetcher = useCallback(async () => {
    const extRes = await listFsExtensions()
    const list = extRes.list ?? []
    return { list, total: list.length }
  }, [])
  const { list: exts, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取分机列表失败')), [t]),
  })

  const loadRegs = useCallback(async () => {
    try {
      const regRes = await listFsRegistrations()
      setRegs(regRes.list ?? [])
    } catch {
      // 注册态是辅数据，失败不挡分机列表
    }
  }, [])

  useEffect(() => { void loadRegs() }, [loadRegs])

  const handleCreate = async (values: ExtFormValues) => {
    setCreating(true)
    try {
      const payload = {
        ext: values.ext.trim(),
        password: values.password.trim(),
        label: values.label?.trim(),
        vm_enabled: !!values.vm_enabled,
        vm_password: values.vm_enabled ? values.vm_password?.trim() : undefined,
        dir_last_name: values.dir_last_name?.trim(),
        dir_first_name: values.dir_first_name?.trim(),
      }
      if (editingExt) {
        await updateFsExtension(editingExt.ext, payload)
        message.success(t('分机 {{ext}} 已更新并热加载（改密码后需用新密码重新注册）', { ext: editingExt.ext }))
      } else {
        await createFsExtension(payload)
        message.success(t('分机 {{ext}} 已创建并热加载，软电话即可注册', { ext: values.ext }))
      }
      setModalOpen(false)
      setEditingExt(null)
      form.resetFields()
      void reload(); void loadRegs()
    } catch (e) {
      // 后端 409（ext already exists）转友好提示；其余显示原始错误
      const msg = (e as Error)?.message || ''
      if (msg.includes('already exists')) {
        message.error(t('该分机号已存在'))
      } else {
        message.error(msg || t(editingExt ? '更新分机失败' : '创建分机失败'))
      }
    } finally {
      setCreating(false)
    }
  }

  // 打开新建/编辑弹窗
  const openModal = (row?: FsExtension) => {
    if (row) {
      setEditingExt(row)
      form.setFieldsValue({
        ext: row.ext,
        password: row.password,
        label: row.label,
        vm_enabled: !!row.vm_enabled,
        vm_password: row.vm_password,
        dir_first_name: row.dir_first_name,
        dir_last_name: row.dir_last_name,
      })
    } else {
      setEditingExt(null)
      form.resetFields()
    }
    setModalOpen(true)
  }

  const handleDelete = async (ext: string) => {
    try {
      await deleteFsExtension(ext)
      message.success(t('分机 {{ext}} 已删除', { ext }))
      void reload(); void loadRegs()
    } catch (e) {
      message.error((e as Error)?.message || t('删除分机失败'))
    }
  }

  const regOf = (ext: string) => regs.find((r) => r.reg_user === ext)

  const columns: ColumnsType<FsExtension> = [
    {
      title: t('分机'),
      dataIndex: 'ext',
      width: 110,
      render: (v: string) => (
        <Space size={6}><UserOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('名称'),
      dataIndex: 'label',
      width: 160,
      responsive: ['sm'],
      render: (v: string) => v || <span className="cell-dim">—</span>,
    },
    {
      title: t('411 目录'),
      key: 'directory',
      width: 130,
      responsive: ['lg'],
      render: (_, row) => {
        const name = [row.dir_last_name, row.dir_first_name].filter(Boolean).join(' ')
        return name ? (
          <Tooltip title={t('拨 411 可按此姓名拼音查找到该分机')}>
            <Tag color="purple" variant="filled">{name}</Tag>
          </Tooltip>
        ) : (
          <Tooltip title={t('未设姓名拼音：不进 411 按名字拨号目录')}>
            <span className="cell-dim">{t('未入目录')}</span>
          </Tooltip>
        )
      },
    },
    {
      title: t('SIP 密码'),
      dataIndex: 'password',
      width: 130,
      responsive: ['lg'],
      render: (v: string) => (v ? <span className="cell-mono cell-dim">{v}</span> : '—'),
    },
    {
      title: t('语音信箱'),
      key: 'vm',
      width: 120,
      responsive: ['md'],
      render: (_, row) =>
        row.vm_enabled ? (
          <Tooltip title={t('无人接 25 秒转留言，坐席拨 *98 听取')}>
            <Tag color="gold" variant="filled"><MailOutlined /> {t('已开启')}</Tag>
          </Tooltip>
        ) : (
          <span className="cell-dim">—</span>
        ),
    },
    {
      title: t('来源'),
      dataIndex: 'source',
      width: 90,
      responsive: ['md'],
      render: (v: string) =>
        v === 'dynamic' ? <Tag color="cyan" variant="filled">{t('动态')}</Tag> : <Tag variant="filled">{t('内置')}</Tag>,
    },
    {
      title: t('注册状态'),
      key: 'online',
      responsive: ['sm'],
      render: (_, row) => {
        const reg = regOf(row.ext)
        if (!reg) return <Badge status="default" text={t('离线')} />
        return (
          <Space size={8}>
            <Badge status="success" text={t('在线')} />
            <span className="cell-mono cell-dim" style={{ fontSize: 12 }}>
              {reg.network_ip}:{reg.network_port}/{(reg.network_proto || '').toUpperCase()}
            </span>
          </Space>
        )
      },
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) =>
        row.source === 'dynamic' ? (
          <TableRowActions
            menuOnly={compactActions}
            maxInline={2}
            ariaLabel={t('更多操作：{{name}}', { name: row.ext })}
            actions={[
              { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openModal(row) },
              {
                key: 'delete',
                label: t('删除'),
                icon: <DeleteOutlined />,
                danger: true,
                confirm: {
                  title: t('确认删除分机 {{ext}}？', { ext: row.ext }),
                  description: t('已注册的话机将无法再注册'),
                },
                onClick: () => { void handleDelete(row.ext) },
              },
            ]}
          />
        ) : (
          <Tooltip title={t('内置分机来自镜像模板，不可删除')}>
            <span className="cell-dim">—</span>
          </Tooltip>
        ),
    },
  ]

  return (
    <ListPageShell
      className="cc-extensions-page"
      toolbar={
        <TableToolbar
          title="分机管理"
          total={exts.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>{t('新建分机')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload(); void loadRegs() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Table
          rowKey="ext"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={exts}
          loading={loading}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无分机" compact /> }}
        />

      <Modal
        title={editingExt ? t('编辑分机 {{ext}}', { ext: editingExt.ext }) : t('新建分机')}
        open={modalOpen}
        onCancel={() => { setModalOpen(false); setEditingExt(null) }}
        onOk={() => form.submit()}
        confirmLoading={creating}
        okText={editingExt ? t('保存') : t('创建')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleCreate} style={{ marginTop: 8 }}>
          <Form.Item
            name="ext"
            label={t('分机号')}
            rules={[
              { required: true, message: t('请输入分机号') },
              { pattern: /^[0-9]{4,6}$/, message: t('4-6 位数字') },
              {
                validator: (_, v: string) => {
                  const n = Number(v)
                  if (v && ((n >= 1000 && n <= 1019) || v === '5000' || v === '9196')) {
                    return Promise.reject(new Error(t('该号段为内置保留（1000-1019/5000/9196）')))
                  }
                  // 新建时查重：分机号已在库 → 直接提示，不提交到后端吃 409
                  if (v && !editingExt && exts.some((e) => e.ext === v)) {
                    return Promise.reject(new Error(t('该分机号已存在')))
                  }
                  return Promise.resolve()
                },
              },
            ]}
          >
            <Input placeholder={t('如 2000')} prefix={<UserOutlined />} disabled={!!editingExt} />
          </Form.Item>
          <Form.Item
            name="password"
            label={t('SIP 密码')}
            rules={[{ required: true, message: t('请输入密码') }, { pattern: /^[A-Za-z0-9]{4,32}$/, message: t('4-32 位字母数字') }]}
          >
            <Input
              placeholder={t('软电话注册密码')}
              addonAfter={
                <Button
                  size="small"
                  type="text"
                  icon={<ReloadOutlined />}
                  onClick={() => form.setFieldValue('password', genRandomPassword())}
                  title={t('随机生成强密码（可再次点击换一个）')}
                >
                  {t('随机')}
                </Button>
              }
            />
          </Form.Item>
          <Form.Item name="label" label={t('名称（可选）')}>
            <Input placeholder={t('如：客服 5 号')} maxLength={32} />
          </Form.Item>
          <Form.Item
            name="vm_enabled"
            label={
              <Space size={6}>
                <span>{t('语音信箱')}</span>
                <Tooltip title={t('开启后：无人接听 / 占线约 25 秒转语音留言；坐席在话机上拨 *98 可听取留言')}>
                  <MailOutlined style={{ opacity: 0.6 }} />
                </Tooltip>
              </Space>
            }
            valuePropName="checked"
            initialValue={false}
          >
            <Switch checkedChildren={t('开')} unCheckedChildren={t('关')} />
          </Form.Item>
          {vmEnabled && (
            <Form.Item
              name="vm_password"
              label={t('信箱密码（可选）')}
              tooltip={t('4-8 位数字，*98 听取留言时输入；留空则与分机号相同')}
              rules={[{ pattern: /^[0-9]{4,8}$/, message: t('4-8 位数字') }]}
            >
              <Input placeholder={t('留空 = 同分机号')} maxLength={8} inputMode="numeric" />
            </Form.Item>
          )}
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="dir_last_name"
              label={t('姓（拼音）')}
              style={{ flex: 1 }}
              tooltip={t('拨 411 按姓名查找同事（mod_directory 目录）；如 zhang。留空则不进目录')}
              rules={[{ pattern: /^[a-z]{0,32}$/, message: t('小写字母拼音，如 zhang') }]}
            >
              <Input placeholder={t('如 zhang')} maxLength={32} />
            </Form.Item>
            <Form.Item
              name="dir_first_name"
              label={t('名（拼音）')}
              style={{ flex: 1 }}
              tooltip={t('拨 411 按姓名查找同事；如 san。姓、名留空则该分机不进 411 目录')}
              rules={[{ pattern: /^[a-z]{0,32}$/, message: t('小写字母拼音，如 san') }]}
            >
              <Input placeholder={t('如 san')} maxLength={32} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
