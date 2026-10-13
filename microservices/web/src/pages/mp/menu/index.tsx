import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Form, Input, Modal, Select, Space, Spin, Tag, Tooltip, Typography,
} from 'antd'
import {
  AppstoreOutlined, ArrowDownOutlined, ArrowUpOutlined, CloudUploadOutlined,
  DeleteOutlined, PlusOutlined, ReloadOutlined, SaveOutlined,
} from '@ant-design/icons'
import { UNSAFE_NavigationContext, useLocation, useNavigate, type To } from 'react-router-dom'
import { message } from '@/utils/feedback'
import {
  deleteMpMenu, getMpMenu, listMpAccounts, publishMpMenu, saveMpMenu,
  type MpAccount, type MpMenuButton,
} from '@/api/mp'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import './index.css'

const { Text } = Typography

type MenuParams = { account_id?: number }

const actionOptions = [
  { label: '触发事件', value: 'click' },
  { label: '打开网页', value: 'view' },
  { label: '打开小程序', value: 'miniprogram' },
  { label: '扫码推送', value: 'scancode_push' },
  { label: '扫码提示', value: 'scancode_waitmsg' },
  { label: '系统拍照', value: 'pic_sysphoto' },
  { label: '拍照或相册', value: 'pic_photo_or_album' },
  { label: '微信相册', value: 'pic_weixin' },
  { label: '发送位置', value: 'location_select' },
  { label: '永久素材', value: 'media_id' },
  { label: '图文素材', value: 'view_limited' },
  { label: '发布文章', value: 'article_id' },
  { label: '文章视图', value: 'article_view_limited' },
]

const keyActionTypes = new Set([
  'click', 'scancode_push', 'scancode_waitmsg', 'pic_sysphoto', 'pic_photo_or_album', 'pic_weixin',
  'location_select',
])
const mediaActionTypes = new Set(['media_id', 'view_limited'])
const articleActionTypes = new Set(['article_id', 'article_view_limited'])
const unsavedMenuMessage = '当前菜单有未保存修改，离开后将丢失。'

function targetPathname(to: To) {
  if (typeof to === 'string') return new URL(to, window.location.href).pathname
  return to.pathname
}

function newLeaf(): MpMenuButton {
  return { name: '', type: 'click', key: '' }
}

function resetAction(button: MpMenuButton, type: string): MpMenuButton {
  const base: MpMenuButton = { name: button.name, type }
  if (keyActionTypes.has(type)) base.key = ''
  if (type === 'view') base.url = ''
  if (type === 'miniprogram') {
    base.appid = ''
    base.pagepath = ''
    base.url = ''
  }
  if (mediaActionTypes.has(type)) base.media_id = ''
  if (articleActionTypes.has(type)) base.article_id = ''
  return base
}

function nameWeight(raw: string) {
  return Array.from(raw).reduce((total, char) => total + (char.charCodeAt(0) <= 127 ? 1 : 2), 0)
}

function validHTTPURL(raw?: string) {
  if (!raw) return false
  try {
    const parsed = new URL(raw)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function validateButtons(buttons: MpMenuButton[]): string | null {
  if (buttons.length < 1 || buttons.length > 3) return '一级菜单数量必须为 1 到 3 个'
  for (let i = 0; i < buttons.length; i += 1) {
    const root = buttons[i]
    if (!root.name.trim() || nameWeight(root.name.trim()) > 8) return `一级菜单 ${i + 1} 名称不符合长度要求`
    const children = root.sub_button ?? []
    const leaves = children.length ? children : [root]
    if (children.length > 5) return `一级菜单 ${i + 1} 最多包含 5 个二级菜单`
    for (let j = 0; j < leaves.length; j += 1) {
      const leaf = leaves[j]
      const label = children.length ? `二级菜单 ${i + 1}.${j + 1}` : `一级菜单 ${i + 1}`
      if (!leaf.name.trim() || (children.length && nameWeight(leaf.name.trim()) > 16)) return `${label} 名称不符合长度要求`
      if (keyActionTypes.has(leaf.type ?? '') && !leaf.key?.trim()) return `${label} 的事件 Key 不能为空`
      if (leaf.type === 'view' && !validHTTPURL(leaf.url)) return `${label} 需要完整的 http/https 地址`
      if (leaf.type === 'miniprogram' && (!leaf.appid?.trim() || !leaf.pagepath?.trim() || !validHTTPURL(leaf.url))) {
        return `${label} 需要 AppID、页面路径和备用网页地址`
      }
      if (mediaActionTypes.has(leaf.type ?? '') && !leaf.media_id?.trim()) return `${label} 的素材 Media ID 不能为空`
      if (articleActionTypes.has(leaf.type ?? '') && !leaf.article_id?.trim()) return `${label} 的文章 Article ID 不能为空`
      if (!actionOptions.some((option) => option.value === leaf.type)) return `${label} 使用了当前编辑器不支持的动作类型`
    }
  }
  return null
}

function ActionFields({
  button, disabled, pathLabel, onChange,
}: {
  button: MpMenuButton
  disabled: boolean
  pathLabel: string
  onChange: (next: MpMenuButton) => void
}) {
  const { t } = useTranslation()
  const type = button.type || 'click'
  return (
    <div className="mp-menu-action-fields">
      <Select
        aria-label={`${pathLabel} ${t('动作类型')}`}
        disabled={disabled}
        options={actionOptions.map((o) => ({ ...o, label: t(o.label) }))}
        value={type}
        onChange={(value) => onChange(resetAction(button, String(value)))}
      />
      {keyActionTypes.has(type) && (
        <Form.Item label={t('事件 Key')} required>
          <Input
            disabled={disabled}
            aria-label={`${pathLabel} ${t('事件 Key')}`}
            maxLength={128}
            placeholder={t('例如 CUSTOMER_SERVICE')}
            value={button.key}
            onChange={(event) => onChange({ ...button, key: event.target.value })}
          />
        </Form.Item>
      )}
      {type === 'view' && (
        <Form.Item label={t('网页地址')} required>
          <Input
            disabled={disabled}
            aria-label={`${pathLabel} ${t('网页地址')}`}
            maxLength={1024}
            placeholder="https://example.com"
            value={button.url}
            onChange={(event) => onChange({ ...button, url: event.target.value })}
          />
        </Form.Item>
      )}
      {type === 'miniprogram' && (
        <div className="mp-menu-miniprogram-fields">
          <Form.Item label={t('小程序 AppID')} required>
            <Input
              disabled={disabled}
              aria-label={`${pathLabel} ${t('小程序 AppID')}`}
              maxLength={64}
              value={button.appid}
              onChange={(event) => onChange({ ...button, appid: event.target.value })}
            />
          </Form.Item>
          <Form.Item label={t('页面路径')} required>
            <Input
              disabled={disabled}
              aria-label={`${pathLabel} ${t('页面路径')}`}
              maxLength={1024}
              placeholder="pages/index"
              value={button.pagepath}
              onChange={(event) => onChange({ ...button, pagepath: event.target.value })}
            />
          </Form.Item>
          <Form.Item className="mp-menu-field-wide" label={t('备用网页地址')} required>
            <Input
              disabled={disabled}
              aria-label={`${pathLabel} ${t('备用网页地址')}`}
              maxLength={1024}
              placeholder="https://example.com/fallback"
              value={button.url}
              onChange={(event) => onChange({ ...button, url: event.target.value })}
            />
          </Form.Item>
        </div>
      )}
      {mediaActionTypes.has(type) && (
        <Form.Item label={t('素材 Media ID')} required>
          <Input
            disabled={disabled}
            aria-label={`${pathLabel} ${t('素材 Media ID')}`}
            maxLength={128}
            value={button.media_id}
            onChange={(event) => onChange({ ...button, media_id: event.target.value })}
          />
        </Form.Item>
      )}
      {articleActionTypes.has(type) && (
        <Form.Item label={t('文章 Article ID')} required>
          <Input
            disabled={disabled}
            aria-label={`${pathLabel} ${t('文章 Article ID')}`}
            maxLength={128}
            value={button.article_id}
            onChange={(event) => onChange({ ...button, article_id: event.target.value })}
          />
        </Form.Item>
      )}
    </div>
  )
}

export default function MpMenuPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const { navigator } = useContext(UNSAFE_NavigationContext)
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('social:write')
  const [params, setParams] = useUrlParams<MenuParams>({}, ['account_id'])
  const [accounts, setAccounts] = useState<MpAccount[]>([])
  const [buttons, setButtons] = useState<MpMenuButton[]>([])
  const [published, setPublished] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<string>()
  const [accountsLoading, setAccountsLoading] = useState(true)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [dirty, setDirty] = useState(false)
  const initialAccountID = useRef(params.account_id)
  const currentAccountID = useRef(params.account_id)
  const menuRequestSequence = useRef(0)
  const restoringHistory = useRef(false)
  const operationBusy = saving || publishing || deleting
  const canEdit = canWrite && !operationBusy

  const selectedAccount = useMemo(
    () => accounts.find((account) => account.id === params.account_id),
    [accounts, params.account_id],
  )
  const selectedAccountActive = selectedAccount?.status === 'active'

  useEffect(() => {
    if (!dirty) return
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty])

  useEffect(() => {
    if (!dirty) return
    const originalPush = navigator.push
    const originalReplace = navigator.replace
    const allow = (to: To) => {
      const pathname = targetPathname(to)
      return !pathname || pathname === location.pathname || window.confirm(t(unsavedMenuMessage))
    }
    const guardedPush: typeof navigator.push = (to, state, options) => {
      if (allow(to)) originalPush.call(navigator, to, state, options)
    }
    const guardedReplace: typeof navigator.replace = (to, state, options) => {
      if (allow(to)) originalReplace.call(navigator, to, state, options)
    }
    const handlePopState = (event: PopStateEvent) => {
      if (restoringHistory.current) {
        restoringHistory.current = false
        return
      }
      if (window.confirm(t(unsavedMenuMessage))) return
      event.stopImmediatePropagation()
      restoringHistory.current = true
      window.history.go(1)
    }
    // 刻意的脏状态路由劫持护栏：effect 内包 push/replace 拦截、
    // cleanup 恢复原引用；属运行时外部系统接线，非渲染副作用。
    // eslint-disable-next-line react-hooks/exhaustive-deps
    navigator.push = guardedPush
    navigator.replace = guardedReplace
    window.addEventListener('popstate', handlePopState, true)
    return () => {
      if (navigator.push === guardedPush) navigator.push = originalPush
      if (navigator.replace === guardedReplace) navigator.replace = originalReplace
      window.removeEventListener('popstate', handlePopState, true)
    }
  }, [dirty, location.pathname, navigator, t])

  useEffect(() => {
    let active = true
    setAccountsLoading(true)
    listMpAccounts({ page: 1, page_size: 100 })
      .then((result) => {
        if (!active) return
        const next = result.list ?? []
        setAccounts(next)
        const requestedAccountID = initialAccountID.current
        if (next.length > 0 && (!requestedAccountID || !next.some((account) => account.id === requestedAccountID))) {
          setParams({ account_id: next[0].id })
        }
      })
      .catch(() => {
        if (active) message.error(t('获取公众号账号失败'))
      })
      .finally(() => {
        if (active) setAccountsLoading(false)
      })
    return () => { active = false }
  }, [setParams, t])

  const loadMenu = useCallback(async (accountId: number) => {
    const sequence = ++menuRequestSequence.current
    setLoading(true)
    try {
      const result = await getMpMenu(accountId)
      if (sequence !== menuRequestSequence.current || currentAccountID.current !== accountId) return
      setButtons(result.buttons ?? [])
      setPublished(result.published)
      setUpdatedAt(result.updated_at)
      setDirty(false)
    } catch (error: unknown) {
      if (sequence !== menuRequestSequence.current || currentAccountID.current !== accountId) return
      setButtons([])
      setPublished(false)
      setUpdatedAt(undefined)
      setDirty(false)
      message.error(error instanceof Error ? error.message : t('获取菜单草稿失败'))
    } finally {
      if (sequence === menuRequestSequence.current) setLoading(false)
    }
  }, [t])

  useEffect(() => {
    currentAccountID.current = params.account_id
    menuRequestSequence.current += 1
    setButtons([])
    setPublished(false)
    setUpdatedAt(undefined)
    setDirty(false)
    if (params.account_id) void loadMenu(params.account_id)
    else {
      setLoading(false)
    }
  }, [loadMenu, params.account_id])

  const replaceButtons = (next: MpMenuButton[]) => {
    setButtons(next)
    setDirty(true)
  }

  const updateRoot = (index: number, next: MpMenuButton) => {
    replaceButtons(buttons.map((button, current) => current === index ? next : button))
  }

  const moveRoot = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= buttons.length) return
    const next = [...buttons]
    ;[next[index], next[target]] = [next[target], next[index]]
    replaceButtons(next)
  }

  const addChild = (rootIndex: number) => {
    const root = buttons[rootIndex]
    const children = root.sub_button ?? []
    if (children.length >= 5) return
    updateRoot(rootIndex, { name: root.name, sub_button: [...children, newLeaf()] })
  }

  const updateChild = (rootIndex: number, childIndex: number, nextChild: MpMenuButton) => {
    const root = buttons[rootIndex]
    const children = [...(root.sub_button ?? [])]
    children[childIndex] = nextChild
    updateRoot(rootIndex, { ...root, sub_button: children })
  }

  const removeChild = (rootIndex: number, childIndex: number) => {
    const root = buttons[rootIndex]
    const children = (root.sub_button ?? []).filter((_, index) => index !== childIndex)
    updateRoot(rootIndex, children.length ? { ...root, sub_button: children } : { name: root.name, type: 'click', key: '' })
  }

  const moveChild = (rootIndex: number, childIndex: number, direction: -1 | 1) => {
    const root = buttons[rootIndex]
    const children = [...(root.sub_button ?? [])]
    const target = childIndex + direction
    if (target < 0 || target >= children.length) return
    ;[children[childIndex], children[target]] = [children[target], children[childIndex]]
    updateRoot(rootIndex, { ...root, sub_button: children })
  }

  const chooseAccount = (accountId: number) => {
    if (!dirty) {
      setParams({ account_id: accountId })
      return
    }
    Modal.confirm({
      title: t('切换公众号'),
      content: t('当前菜单有未保存修改，切换后将丢失。'),
      okText: t('放弃并切换'),
      cancelText: t('继续编辑'),
      onOk: () => setParams({ account_id: accountId }),
    })
  }

  const reloadMenu = () => {
    if (!params.account_id) return
    if (!dirty) {
      void loadMenu(params.account_id)
      return
    }
    Modal.confirm({
      title: t('重新加载菜单'),
      content: t('当前菜单有未保存修改，重新加载后将丢失。'),
      okText: t('放弃并重载'),
      cancelText: t('继续编辑'),
      onOk: () => loadMenu(params.account_id as number),
    })
  }

  const saveDraft = async () => {
    if (!params.account_id) return false
    const accountId = params.account_id
    const validation = validateButtons(buttons)
    if (validation) {
      message.error(t(validation))
      return false
    }
    setSaving(true)
    try {
      const result = await saveMpMenu(accountId, { buttons })
      if (currentAccountID.current !== accountId) return true
      setButtons(result.buttons)
      setPublished(result.published)
      setUpdatedAt(result.updated_at)
      setDirty(false)
      message.success(t('菜单草稿已保存'))
      return true
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : t('保存菜单草稿失败'))
      return false
    } finally {
      setSaving(false)
    }
  }

  const publish = async () => {
    if (!params.account_id) return
    const accountId = params.account_id
    const validation = validateButtons(buttons)
    if (validation) {
      message.error(t(validation))
      return
    }
    setPublishing(true)
    try {
      if (dirty) {
        const saved = await saveMpMenu(accountId, { buttons })
        if (currentAccountID.current !== accountId) return
        setButtons(saved.buttons)
        setPublished(saved.published)
        setUpdatedAt(saved.updated_at)
        setDirty(false)
      }
      const result = await publishMpMenu(accountId)
      if (currentAccountID.current !== accountId) return
      setButtons(result.buttons)
      setPublished(result.published)
      setUpdatedAt(result.updated_at)
      setDirty(false)
      message.success(t('菜单已发布到微信'))
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : t('发布菜单失败，草稿已保留'))
    } finally {
      setPublishing(false)
    }
  }

  const removeMenu = () => {
    if (!params.account_id) return
    const accountId = params.account_id
    Modal.confirm({
      title: t('删除微信自定义菜单'),
      content: t('将删除当前公众号的线上菜单和本地草稿。'),
      okText: t('删除'),
      okButtonProps: { danger: true, loading: deleting },
      cancelText: t('取消'),
      async onOk() {
        setDeleting(true)
        try {
          await deleteMpMenu(accountId)
          if (currentAccountID.current !== accountId) return
          setButtons([])
          setPublished(false)
          setUpdatedAt(undefined)
          setDirty(false)
          message.success(t('微信菜单已删除'))
        } catch (error: unknown) {
          message.error(error instanceof Error ? error.message : t('删除菜单失败'))
          throw error
        } finally {
          setDeleting(false)
        }
      },
    })
  }

  const status = dirty
    ? <StatusPill tone="warning" label="未保存" />
    : published
      ? <StatusPill tone="success" label="已发布" />
      : <StatusPill tone="muted" label={buttons.length ? '草稿' : '未配置'} />

  return (
    <ListPageShell
      className="mp-menu-page"
      filter={
        <Space wrap className="mp-menu-account-filter">
          <Select
            aria-label={t('公众号账号')}
            disabled={operationBusy}
            loading={accountsLoading}
            placeholder={t('选择公众号账号')}
            showSearch
            optionFilterProp="label"
            value={params.account_id}
            options={accounts.map((account) => ({ value: account.id, label: account.name || account.app_id }))}
            onChange={chooseAccount}
          />
          {selectedAccount && <Tag variant="filled">{selectedAccount.type === 'service' ? t('服务号') : t('订阅号')}</Tag>}
          {selectedAccount && <Tag color={selectedAccountActive ? 'success' : 'default'}>{selectedAccountActive ? t('账号启用') : t('账号停用')}</Tag>}
          {selectedAccount && status}
          {updatedAt && <Text type="secondary">{t('更新于 {{time}}', { time: formatDateTime(updatedAt) })}</Text>}
        </Space>
      }
      toolbar={
        <TableToolbar
          title="自定义菜单"
          icon={<AppstoreOutlined />}
          extra={params.account_id ? (
            <Space wrap>
              <Button icon={<ReloadOutlined />} disabled={loading || operationBusy} onClick={reloadMenu}>{t('重载')}</Button>
              {canWrite && <Button icon={<SaveOutlined />} loading={saving} disabled={loading || publishing || deleting} onClick={() => void saveDraft()}>{t('保存草稿')}</Button>}
              {canWrite && <Button type="primary" icon={<CloudUploadOutlined />} loading={publishing} disabled={loading || saving || deleting || !selectedAccountActive} title={!selectedAccountActive ? t('公众号账号已停用') : undefined} onClick={() => void publish()}>{t('发布')}</Button>}
              {canWrite && <Button danger icon={<DeleteOutlined />} loading={deleting} disabled={loading || saving || publishing} onClick={removeMenu}>{t('删除')}</Button>}
            </Space>
          ) : undefined}
        />}
      >

        {!accountsLoading && accounts.length === 0 ? (
          <div className="mp-menu-no-account">
            <GlassEmpty text="暂无公众号账号" compact />
            <Button type="primary" onClick={() => navigate('/mp/accounts')}>{t('配置公众号账号')}</Button>
          </div>
        ) : loading ? (
          <div className="mp-menu-loading"><Spin /></div>
        ) : params.account_id ? (
          <div className="mp-menu-workspace">
            <div className="mp-menu-editor" aria-label={t('菜单编辑器')}>
              {buttons.map((button, rootIndex) => {
                const children = button.sub_button ?? []
                return (
                  <section className="mp-menu-group" key={`root-${rootIndex}`}>
                    <div className="mp-menu-group-header">
                      <Text strong>{t('一级菜单 {{n}}', { n: rootIndex + 1 })}</Text>
                      <Space size={2}>
                        <Tooltip title={t('上移')}>
                          <Button aria-label={t('上移一级菜单')} type="text" icon={<ArrowUpOutlined />} disabled={!canEdit || rootIndex === 0} onClick={() => moveRoot(rootIndex, -1)} />
                        </Tooltip>
                        <Tooltip title={t('下移')}>
                          <Button aria-label={t('下移一级菜单')} type="text" icon={<ArrowDownOutlined />} disabled={!canEdit || rootIndex === buttons.length - 1} onClick={() => moveRoot(rootIndex, 1)} />
                        </Tooltip>
                        <Tooltip title={t('删除')}>
                          <Button aria-label={t('删除一级菜单')} type="text" danger icon={<DeleteOutlined />} disabled={!canEdit} onClick={() => replaceButtons(buttons.filter((_, index) => index !== rootIndex))} />
                        </Tooltip>
                      </Space>
                    </div>
                    <Form.Item label={t('菜单名称')} required>
                      <Input
                        disabled={!canEdit}
                        aria-label={t('一级菜单 {{n}} 名称', { n: rootIndex + 1 })}
                        maxLength={8}
                        value={button.name}
                        onChange={(event) => {
                          if (nameWeight(event.target.value) <= 8) updateRoot(rootIndex, { ...button, name: event.target.value })
                        }}
                      />
                    </Form.Item>
                    {children.length === 0 ? (
                      <ActionFields button={button} disabled={!canEdit} pathLabel={t('一级菜单 {{n}}', { n: rootIndex + 1 })} onChange={(next) => updateRoot(rootIndex, next)} />
                    ) : (
                      <div className="mp-menu-children">
                        {children.map((child, childIndex) => (
                          <div className="mp-menu-child" key={`child-${rootIndex}-${childIndex}`}>
                            <div className="mp-menu-child-header">
                              <Text strong>{t('二级菜单 {{n}}.{{m}}', { n: rootIndex + 1, m: childIndex + 1 })}</Text>
                              <Space size={2}>
                                <Tooltip title={t('上移')}>
                                  <Button aria-label={t('上移二级菜单')} type="text" icon={<ArrowUpOutlined />} disabled={!canEdit || childIndex === 0} onClick={() => moveChild(rootIndex, childIndex, -1)} />
                                </Tooltip>
                                <Tooltip title={t('下移')}>
                                  <Button aria-label={t('下移二级菜单')} type="text" icon={<ArrowDownOutlined />} disabled={!canEdit || childIndex === children.length - 1} onClick={() => moveChild(rootIndex, childIndex, 1)} />
                                </Tooltip>
                                <Tooltip title={t('删除')}>
                                  <Button aria-label={t('删除二级菜单')} type="text" danger icon={<DeleteOutlined />} disabled={!canEdit} onClick={() => removeChild(rootIndex, childIndex)} />
                                </Tooltip>
                              </Space>
                            </div>
                            <Form.Item label={t('菜单名称')} required>
                              <Input
                                disabled={!canEdit}
                                aria-label={t('二级菜单 {{n}}.{{m}} 名称', { n: rootIndex + 1, m: childIndex + 1 })}
                                maxLength={16}
                                value={child.name}
                                onChange={(event) => {
                                  if (nameWeight(event.target.value) <= 16) updateChild(rootIndex, childIndex, { ...child, name: event.target.value })
                                }}
                              />
                            </Form.Item>
                            <ActionFields button={child} disabled={!canEdit} pathLabel={t('二级菜单 {{n}}.{{m}}', { n: rootIndex + 1, m: childIndex + 1 })} onChange={(next) => updateChild(rootIndex, childIndex, next)} />
                          </div>
                        ))}
                      </div>
                    )}
                    {canWrite && children.length < 5 && (
                      <Button className="mp-menu-add-child" icon={<PlusOutlined />} disabled={!canEdit} onClick={() => addChild(rootIndex)}>{t('添加二级菜单')}</Button>
                    )}
                  </section>
                )
              })}
              {canWrite && buttons.length < 3 && (
                <Button className="mp-menu-add-root" type="dashed" icon={<PlusOutlined />} disabled={!canEdit} onClick={() => replaceButtons([...buttons, newLeaf()])}>
                  {t('添加一级菜单')}
                </Button>
              )}
              {!canWrite && buttons.length === 0 && <GlassEmpty text="当前账号尚未配置菜单" compact />}
            </div>

            <aside className="mp-menu-preview" aria-label={t('菜单预览')}>
              <div className="mp-menu-preview-head">
                <Text strong>{selectedAccount?.name || t('公众号')}</Text>
                {status}
              </div>
              <div className="mp-menu-preview-body">
                <AppstoreOutlined />
              </div>
              <div className="mp-menu-preview-nav">
                {buttons.length === 0 ? <Text type="secondary">{t('未配置菜单')}</Text> : buttons.map((button, index) => (
                  <div className="mp-menu-preview-item" key={`preview-${index}`}>
                    <span>{button.name || t('菜单 {{n}}', { n: index + 1 })}</span>
                    {(button.sub_button?.length ?? 0) > 0 && <small>{button.sub_button?.length}</small>}
                  </div>
                ))}
              </div>
            </aside>
          </div>
        ) : null}
    </ListPageShell>
  )
}
