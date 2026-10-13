import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Input, Space, Tag, Tooltip, Form, Modal, message as antdMessage } from 'antd'
import {
  PhoneOutlined, PhoneFilled, CloseCircleOutlined, UserOutlined, LoginOutlined,
  LogoutOutlined, AudioMutedOutlined, AudioOutlined, PauseCircleOutlined, PlayCircleOutlined,
  SwapOutlined, DashboardOutlined,
} from '@ant-design/icons'
import {
  UserAgent, Registerer, RegistererState, Inviter, Invitation, SessionState,
  type Session,
} from 'sip.js'
import { message } from '@/utils/feedback'

// 网页坐席条：SIP over WebSocket 直连 FreeSWITCH（sofia internal ws-binding
// :5066）。注册后浏览器就是一台话机——队列派单、点击外呼、内线互拨都能接。
// 注意：非 HTTPS 环境浏览器要求地址是本机或受信来源才放开麦克风。
//
// 批次 G 强化：保持（re-INVITE hold）、盲转（REFER）、协商转（REFER w/Replaces）、
// DTMF 键盘。主/咨询两路会话：sessionRef 是主叫；consultRef 是协商转时新建的咨询腿。

type PhoneState = 'unregistered' | 'registering' | 'idle' | 'ringing' | 'calling' | 'incall'

// SIP 域默认取网关同主机；WebSocket 可独立指向启用 WSS 的 FreeSWITCH 入口。
const FS_HOST = window.location.hostname
const runtimeConfig = (window as typeof window & {
  __GAK_RUNTIME_CONFIG__?: { ccSipWsUrl?: unknown }
}).__GAK_RUNTIME_CONFIG__
const runtimeWsServer = typeof runtimeConfig?.ccSipWsUrl === 'string'
  ? runtimeConfig.ccSipWsUrl.trim()
  : ''
const configuredWsServer = runtimeWsServer || import.meta.env.VITE_CC_SIP_WS_URL?.trim()

function resolveWsServer(): { server: string; error?: string } {
  const fallbackProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const candidate = configuredWsServer || `${fallbackProtocol}//${FS_HOST}:5066`
  try {
    const parsed = new URL(candidate)
    if (parsed.protocol !== 'ws:' && parsed.protocol !== 'wss:') {
      return { server: candidate, error: 'SIP WebSocket 地址必须使用 ws:// 或 wss://' }
    }
    if (window.location.protocol === 'https:' && parsed.protocol !== 'wss:') {
      return { server: candidate, error: 'HTTPS 页面必须配置 wss:// SIP WebSocket 地址' }
    }
    return { server: parsed.toString() }
  } catch {
    return { server: candidate, error: 'SIP WebSocket 地址格式无效' }
  }
}

const wsEndpoint = resolveWsServer()
const WS_SERVER = wsEndpoint.server

// DTMF 键盘 3x4 布局
const DTMF_KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#']

// re-INVITE 保持/恢复选项：core 的 SessionDescriptionHandlerOptions 是空接口，
// web 平台的 hold 字段不在其类型里（运行时支持），这里构造带类型的选项。
function holdInviteOptions(hold: boolean) {
  return { sessionDescriptionHandlerOptions: { hold } } as unknown as Parameters<Session['invite']>[0]
}

export default function WebPhone() {
  const { t } = useTranslation()
  const [state, setState] = useState<PhoneState>('unregistered')
  const [ext, setExt] = useState('')
  const [peer, setPeer] = useState('') // 对端号码（振铃/通话中显示）
  const [muted, setMuted] = useState(false)
  const [held, setHeld] = useState(false) // 主叫是否保持中
  const [form] = Form.useForm<{ ext: string; password: string }>()

  // DTMF 面板
  const [dtmfOpen, setDtmfOpen] = useState(false)
  const [dtmfSent, setDtmfSent] = useState('') // 已拨回显
  // 转移弹窗
  const [xferOpen, setXferOpen] = useState(false)
  const [xferTo, setXferTo] = useState('')
  const [xferMode, setXferMode] = useState<'blind' | 'attended'>('blind')
  // 协商转：咨询腿建立后显示「完成转接/取消」
  const [consultActive, setConsultActive] = useState(false)
  const [consultPeer, setConsultPeer] = useState('')

  const uaRef = useRef<UserAgent | null>(null)
  const registererRef = useRef<Registerer | null>(null)
  const sessionRef = useRef<Session | null>(null)   // 主叫会话
  const consultRef = useRef<Session | null>(null)   // 协商转咨询会话
  const audioRef = useRef<HTMLAudioElement | null>(null)

  // 把某会话的远端音频挂到 <audio>
  const attachRemoteAudio = useCallback((session: Session) => {
    const sdh = session.sessionDescriptionHandler as unknown as { peerConnection?: RTCPeerConnection }
    const pc = sdh?.peerConnection
    if (pc && audioRef.current) {
      const stream = new MediaStream()
      pc.getReceivers().forEach((r) => { if (r.track) stream.addTrack(r.track) })
      audioRef.current.srcObject = stream
      audioRef.current.play().catch(() => {})
    }
  }, [])

  // 会话状态机：接通挂载远端媒体，结束清理
  const wireSession = useCallback((session: Session, remote: string) => {
    sessionRef.current = session
    setPeer(remote)
    session.stateChange.addListener((s) => {
      if (s === SessionState.Established) {
        setState('incall')
        attachRemoteAudio(session)
      } else if (s === SessionState.Terminated) {
        sessionRef.current = null
        setPeer('')
        setMuted(false)
        setHeld(false)
        setDtmfOpen(false)
        setDtmfSent('')
        setState((prev) => (prev === 'unregistered' ? prev : 'idle'))
      }
    })
  }, [attachRemoteAudio])

  const register = async (values: { ext: string; password: string }) => {
    setState('registering')
    if (wsEndpoint.error) {
      setState('unregistered')
      message.error(t(wsEndpoint.error))
      return
    }
    try {
      const ua = new UserAgent({
        uri: UserAgent.makeURI(`sip:${values.ext}@${FS_HOST}`),
        transportOptions: { server: WS_SERVER },
        authorizationUsername: values.ext,
        authorizationPassword: values.password,
        delegate: {
          onInvite(invitation: Invitation) {
            // 来电：自动进振铃态，手动接听
            const from = invitation.remoteIdentity.uri.user ?? t('未知')
            wireSession(invitation, from)
            setState('ringing')
            antdMessage.info(t('来电：{{from}}', { from }))
          },
        },
      })
      await ua.start()
      const registerer = new Registerer(ua)
      registerer.stateChange.addListener((s) => {
        if (s === RegistererState.Registered) {
          setState('idle')
          message.success(t('分机 {{ext}} 已上线，浏览器即话机', { ext: values.ext }))
        } else if (s === RegistererState.Terminated || s === RegistererState.Unregistered) {
          setState('unregistered')
        }
      })
      await registerer.register()
      uaRef.current = ua
      registererRef.current = registerer
      setExt(values.ext)
    } catch (e) {
      setState('unregistered')
      message.error(t('注册失败：{{err}}（确认 {{ws}} 可达）', { err: (e as Error).message, ws: WS_SERVER }))
    }
  }

  const unregister = useCallback(async () => {
    try {
      await consultRef.current?.bye?.().catch(() => {})
      await sessionRef.current?.bye?.().catch(() => {})
      await registererRef.current?.unregister().catch(() => {})
      await uaRef.current?.stop()
    } finally {
      uaRef.current = null
      registererRef.current = null
      sessionRef.current = null
      consultRef.current = null
      setState('unregistered')
      setExt('')
      setPeer('')
      setConsultActive(false)
      setConsultPeer('')
    }
  }, [])

  useEffect(() => () => { void unregister() }, [unregister])

  const dial = async (to: string) => {
    const ua = uaRef.current
    if (!ua || !to) return
    const target = UserAgent.makeURI(`sip:${to}@${FS_HOST}`)
    if (!target) return
    const inviter = new Inviter(ua, target, {
      sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } },
    })
    wireSession(inviter, to)
    setState('calling')
    try {
      await inviter.invite()
    } catch (e) {
      message.error(t('呼叫失败：{{err}}', { err: (e as Error).message }))
      setState('idle')
    }
  }

  const answer = async () => {
    const session = sessionRef.current
    if (session && 'accept' in session) {
      await (session as Invitation).accept({
        sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } },
      })
    }
  }

  const hangup = async () => {
    const session = sessionRef.current
    if (!session) return
    if (session.state === SessionState.Established) {
      await session.bye()
    } else if ('reject' in session && session.state === SessionState.Initial) {
      await (session as Invitation).reject()
    } else if ('cancel' in session) {
      await (session as Inviter).cancel()
    }
  }

  const toggleMute = () => {
    const session = sessionRef.current
    if (!session) return
    const sdh = session.sessionDescriptionHandler as unknown as { peerConnection?: RTCPeerConnection }
    sdh?.peerConnection?.getSenders().forEach((s) => {
      if (s.track?.kind === 'audio') s.track.enabled = muted
    })
    setMuted(!muted)
  }

  // 保持/恢复：re-INVITE 改协商方向。hold:true 进保持，false 恢复。
  const toggleHold = async () => {
    const session = sessionRef.current
    if (!session || session.state !== SessionState.Established) return
    const next = !held
    try {
      await session.invite(holdInviteOptions(next))
      setHeld(next)
      message.success(next ? t('已保持通话') : t('已恢复通话'))
    } catch (e) {
      message.error(t('保持操作失败：{{err}}', { err: (e as Error).message }))
    }
  }

  // DTMF：SDH 直接发 RFC2833；已拨回显
  const sendDtmf = (tone: string) => {
    const session = sessionRef.current
    if (!session || session.state !== SessionState.Established) return
    const sdh = session.sessionDescriptionHandler as unknown as { sendDtmf?: (t: string) => boolean }
    if (sdh?.sendDtmf?.(tone)) {
      setDtmfSent((prev) => (prev + tone).slice(-32))
    }
  }

  // 盲转：REFER 到目标 URI，本端随即挂断
  const doBlindTransfer = async (to: string) => {
    const session = sessionRef.current
    if (!session || session.state !== SessionState.Established) return
    const target = UserAgent.makeURI(`sip:${to}@${FS_HOST}`)
    if (!target) {
      message.error(t('目标号码非法'))
      return
    }
    try {
      await session.refer(target)
      message.success(t('已盲转到 {{to}}', { to }))
    } catch (e) {
      message.error(t('盲转失败：{{err}}', { err: (e as Error).message }))
    }
  }

  // 协商转第一步：保持主叫 → 新建咨询 session 呼目标
  const startConsult = async (to: string) => {
    const ua = uaRef.current
    const main = sessionRef.current
    if (!ua || !main || main.state !== SessionState.Established) return
    const target = UserAgent.makeURI(`sip:${to}@${FS_HOST}`)
    if (!target) {
      message.error(t('目标号码非法'))
      return
    }
    try {
      // 先把主叫保持，避免两路音频串台
      if (!held) {
        await main.invite(holdInviteOptions(true))
        setHeld(true)
      }
      const consult = new Inviter(ua, target, {
        sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } },
      })
      consultRef.current = consult
      consult.stateChange.addListener((s) => {
        if (s === SessionState.Established) {
          setConsultActive(true)
          attachRemoteAudio(consult)
        } else if (s === SessionState.Terminated) {
          consultRef.current = null
          setConsultActive(false)
          setConsultPeer('')
        }
      })
      setConsultPeer(to)
      await consult.invite()
      message.info(t('正在咨询 {{to}}，接通后可「完成转接」或「取消咨询」', { to }))
    } catch (e) {
      message.error(t('发起咨询失败：{{err}}', { err: (e as Error).message }))
    }
  }

  // 协商转第二步：REFER w/Replaces——把主叫替换到咨询腿，坐席退出
  const completeAttended = async () => {
    const main = sessionRef.current
    const consult = consultRef.current
    if (!main || !consult) return
    try {
      await main.refer(consult)
      message.success(t('已完成协商转接'))
      // 转接后咨询腿也交给对端，本端两路都会陆续结束
    } catch (e) {
      message.error(t('完成转接失败：{{err}}', { err: (e as Error).message }))
    }
  }

  // 取消咨询：挂断咨询腿，恢复主叫
  const cancelConsult = async () => {
    const consult = consultRef.current
    const main = sessionRef.current
    try {
      if (consult) {
        if (consult.state === SessionState.Established) await consult.bye()
        else if ('cancel' in consult) await (consult as Inviter).cancel()
      }
      if (main && held) {
        await main.invite(holdInviteOptions(false))
        setHeld(false)
      }
    } catch { /* 忽略清理错误 */ } finally {
      consultRef.current = null
      setConsultActive(false)
      setConsultPeer('')
    }
  }

  const submitTransfer = async () => {
    const to = xferTo.trim()
    if (!/^[0-9*#+]{2,32}$/.test(to)) {
      message.error(t('请填写合法的目标号码'))
      return
    }
    setXferOpen(false)
    if (xferMode === 'blind') {
      await doBlindTransfer(to)
    } else {
      await startConsult(to)
    }
    setXferTo('')
  }

  const stateTag = () => {
    switch (state) {
      case 'unregistered': return <Tag variant="filled">{t('离线')}</Tag>
      case 'registering': return <Tag color="processing" variant="filled">{t('注册中…')}</Tag>
      case 'idle': return <Tag color="success" variant="filled">{t('在线')} · {ext}</Tag>
      case 'ringing': return <Tag color="warning" variant="filled" className="webphone-tag-ringing">{t('来电')} · {peer}</Tag>
      case 'calling': return <Tag color="processing" variant="filled">{t('呼叫 {{peer}}…', { peer })}</Tag>
      case 'incall': return <Tag color={held ? 'warning' : 'success'} variant="filled" className="webphone-tag-incall">{held ? t('保持中') : t('通话中')} · {peer}</Tag>
    }
  }

  return (
    <Card className="list-filter-card webphone-card" bordered={false} style={{ marginBottom: 16 }}>
      {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
      <audio ref={audioRef} autoPlay style={{ display: 'none' }} />
      <Space size={16} wrap>
        <Space size={8}>
          <PhoneFilled style={{ color: state === 'unregistered' ? undefined : 'var(--c-success)' }} />
          <b>{t('网页坐席')}</b>
          {stateTag()}
        </Space>

        {state === 'unregistered' && (
          <Form form={form} layout="inline" onFinish={register}>
            <Form.Item name="ext" rules={[{ required: true, message: t('分机') }, { pattern: /^\d{2,16}$/, message: t('数字') }]}>
              <Input placeholder={t('分机号（如 1000）')} prefix={<UserOutlined />} style={{ width: 150 }} />
            </Form.Item>
            <Form.Item name="password" rules={[{ required: true, message: t('密码') }]}>
              <Input.Password placeholder={t('SIP 密码')} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" icon={<LoginOutlined />}>{t('上线')}</Button>
            </Form.Item>
          </Form>
        )}

        {state === 'idle' && (
          <>
            <Input.Search
              placeholder={t('拨号（分机/队列/IVR/9+外线）')}
              enterButton={<><PhoneOutlined /> {t('呼叫')}</>}
              style={{ width: 280 }}
              onSearch={(v) => v.trim() && dial(v.trim())}
              allowClear
            />
            <Button icon={<LogoutOutlined />} onClick={unregister}>{t('下线')}</Button>
          </>
        )}

        {state === 'ringing' && (
          <Space size={8}>
            <Button type="primary" icon={<PhoneOutlined />} onClick={answer}>{t('接听')}</Button>
            <Button danger icon={<CloseCircleOutlined />} onClick={hangup}>{t('拒接')}</Button>
          </Space>
        )}

        {(state === 'calling' || state === 'incall') && (
          <Space size={8} wrap>
            {state === 'incall' && (
              <>
                <Tooltip title={muted ? t('取消静音') : t('静音')}>
                  <Button
                    icon={muted ? <AudioMutedOutlined /> : <AudioOutlined />}
                    onClick={toggleMute}
                    danger={muted}
                  />
                </Tooltip>
                <Tooltip title={held ? t('恢复通话') : t('保持通话')}>
                  <Button
                    icon={held ? <PlayCircleOutlined /> : <PauseCircleOutlined />}
                    onClick={toggleHold}
                  >
                    {held ? t('恢复') : t('保持')}
                  </Button>
                </Tooltip>
                <Tooltip title={t('盲转 / 协商转')}>
                  <Button
                    icon={<SwapOutlined />}
                    onClick={() => { setXferMode('blind'); setXferTo(''); setXferOpen(true) }}
                    disabled={consultActive}
                  >
                    {t('转接')}
                  </Button>
                </Tooltip>
                <Tooltip title={t('拨号键盘（DTMF）')}>
                  <Button icon={<DashboardOutlined />} onClick={() => setDtmfOpen(true)}>{t('键盘')}</Button>
                </Tooltip>
              </>
            )}
            <Button danger icon={<CloseCircleOutlined />} onClick={hangup}>{t('挂断')}</Button>
          </Space>
        )}

        {consultActive && (
          <Space size={8}>
            <Tag color="processing" variant="filled">{t('咨询中 · {{peer}}', { peer: consultPeer })}</Tag>
            <Button type="primary" size="small" icon={<SwapOutlined />} onClick={completeAttended}>{t('完成转接')}</Button>
            <Button size="small" onClick={cancelConsult}>{t('取消咨询')}</Button>
          </Space>
        )}

        {state === 'unregistered' && (
          <span className="cell-dim" style={{ fontSize: 12 }}>
            {t('浏览器直连 FS（{{ws}}）;需允许麦克风权限', { ws: WS_SERVER })}
          </span>
        )}
      </Space>

      {/* DTMF 键盘面板 */}
      <Modal
        title={t('拨号键盘（DTMF）')}
        open={dtmfOpen}
        onCancel={() => setDtmfOpen(false)}
        footer={null}
        width={280}
        destroyOnHidden
      >
        <div style={{ textAlign: 'center', marginBottom: 12 }}>
          <Input
            value={dtmfSent}
            readOnly
            placeholder={t('按键回显')}
            style={{ textAlign: 'center', fontFamily: 'monospace', fontSize: 18 }}
          />
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10 }}>
          {DTMF_KEYS.map((k) => (
            <Button key={k} size="large" onClick={() => sendDtmf(k)} className="webphone-dtmf-btn">{k}</Button>
          ))}
        </div>
      </Modal>

      {/* 转接弹窗 */}
      <Modal
        title={t('转接通话')}
        open={xferOpen}
        onCancel={() => setXferOpen(false)}
        onOk={submitTransfer}
        okText={xferMode === 'blind' ? t('盲转') : t('发起咨询')}
        destroyOnHidden
      >
        <Space direction="vertical" size={14} style={{ width: '100%', marginTop: 8 }}>
          <Space size={8}>
            <Button
              type={xferMode === 'blind' ? 'primary' : 'default'}
              onClick={() => setXferMode('blind')}
            >
              {t('盲转（直接转走）')}
            </Button>
            <Button
              type={xferMode === 'attended' ? 'primary' : 'default'}
              onClick={() => setXferMode('attended')}
            >
              {t('协商转（先咨询）')}
            </Button>
          </Space>
          <Input
            placeholder={t('目标号码（分机/队列/9+外线）')}
            prefix={<PhoneOutlined />}
            value={xferTo}
            onChange={(e) => setXferTo(e.target.value)}
            onPressEnter={submitTransfer}
            allowClear
          />
          <span className="cell-dim" style={{ fontSize: 12 }}>
            {xferMode === 'blind'
              ? t('盲转：立即把客户转到目标号码，坐席退出。')
              : t('协商转：先保持客户、私聊目标坐席，确认后「完成转接」。')}
          </span>
        </Space>
      </Modal>
    </Card>
  )
}
