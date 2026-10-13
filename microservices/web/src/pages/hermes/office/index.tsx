// 3D 办公室地图（管理员运维视图）：工位按绑定员工的 Hermes 设备状态着色。
// 渲染层在 Scene.tsx，暂存层在 useSeatEditor.ts，本文件负责数据、楼层、HUD 与保存。
//
// 视觉参照 Hive Office：场景四角浮 HUD（楼层信息卡 / 视图切换 / 风险工位栏），
// 底部三栏统计（实时概览 / 部门分布 / 优先关注）。部门用固定顺序的分类色板，
// 超过 8 个的部门统一折入中性灰——分类色循环复用会撞色，宁可折叠。
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Tooltip,
} from 'antd'
import ErrorBoundary from '@/components/common/ErrorBoundary'
import {
  AimOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  MinusOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons'
import {
  bulkUpsertOfficeSeats,
  createOfficeFloor,
  deleteOfficeFloor,
  listOfficeFloors,
  listOfficeSeats,
  seatStatus,
  updateOfficeFloor,
  type HermesOfficeFloor,
  type SeatStatus,
} from '@/api/ai/hermes-office'
import { getUserList } from '@/api/system/user'
import { getDepartmentTree } from '@/api/system/department'
import type { Department, SystemUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill, { type StatusTone } from '@/components/common/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { useCountUp } from '@/hooks/useCountUp'
import { useThemeMode } from '@/theme/ThemeContext'
import { fetchAllHermesPages } from '../admin-contract'
import Scene, {
  SEAT_STATUS_COLORS,
  type OfficeDepartmentPod,
} from './Scene'
import { useSeatEditor } from './useSeatEditor'

const STATUS_META: Record<SeatStatus, { label: string; tone: StatusTone }> = {
  online: { label: '在线', tone: 'success' },
  offline: { label: '离线', tone: 'muted' },
  error: { label: '异常', tone: 'danger' },
  unprovisioned: { label: '未开通', tone: 'warning' },
  empty: { label: '空位', tone: 'muted' },
}

// 部门分类色板（dataviz 校验通过的 8 槽，明暗各一组；按全局部门顺序固定分配，不循环）。
// light 组里 aqua/yellow/magenta 三槽对浅底不足 3:1，因此色点永远和文字标签成对出现。
const DEPT_COLORS: Record<'light' | 'dark', string[]> = {
  light: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#4a3aa7', '#e34948'],
  dark: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'],
}
const DEPT_FALLBACK: Record<'light' | 'dark', string> = { light: '#8b93a7', dark: '#6b7280' }

/** 统计 tile 的数值滚动动画（useCountUp 是 hook，只能包在组件里逐个用） */
function StatValue({ value }: { value: number }) {
  const display = useCountUp(value)
  return <div className="hermes-stat-value">{display}</div>
}

function flattenDepts(list: Department[], out: Department[] = []): Department[] {
  list.forEach((d) => {
    out.push(d)
    if (d.children?.length) flattenDepts(d.children, out)
  })
  return out
}

export default function HermesOfficePage() {
  const { t } = useTranslation()
  const { hasPerm, isSuperAdmin } = usePermission()
  const { mode: themeMode } = useThemeMode()
  const canEdit = isSuperAdmin || hasPerm('ai:hermes:office:write')

  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [floors, setFloors] = useState<HermesOfficeFloor[]>([])
  const [floorId, setFloorId] = useState<number | null>(null)
  const [users, setUsers] = useState<SystemUser[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [selectedSeatId, setSelectedSeatId] = useState<number | null>(null)
  const [editing, setEditing] = useState(false)
  const [activePodKey, setActivePodKey] = useState('all')
  const [resetKey, setResetKey] = useState(0)
  // 左上 HUD 面板默认收起（不挡 3D 场景），点小 chip 展开、卡片内收起
  const [hudCollapsed, setHudCollapsed] = useState(true)
  const [floorModalOpen, setFloorModalOpen] = useState(false)
  const [floorModalIsEdit, setFloorModalIsEdit] = useState(false)
  const [floorForm] = Form.useForm()

  const editor = useSeatEditor([])
  const { draft, dirty, reset, moveSeat, rotateSeat, setLabel, bindUser, addSeat, removeSeat, payload } = editor

  const floor = useMemo(() => floors.find((f) => f.id === floorId) ?? null, [floors, floorId])
  const selected = useMemo(() => draft.find((s) => s.seat.id === selectedSeatId) ?? null, [draft, selectedSeatId])

  const loadFloors = useCallback(async () => {
    try {
      const list = await listOfficeFloors()
      setFloors(list ?? [])
      setFloorId((prev) => prev ?? list?.[0]?.id ?? null)
    } catch {
      // Request layer already reports the server message.
    }
  }, [])

  const loadSeats = useCallback(
    async (id: number) => {
      setLoading(true)
      try {
        const list = await listOfficeSeats(id)
        reset(list ?? [])
      } catch {
        reset([])
      } finally {
        setLoading(false)
      }
    },
    [reset],
  )

  useEffect(() => {
    void loadFloors()
    // 3D 办公室需要全部员工的工位状态：逐页真实拉取，不放大 page_size 伪装全量
    fetchAllHermesPages<SystemUser>((page, pageSize) => getUserList({ page, page_size: pageSize }))
      .then((all) => setUsers(all))
      .catch(() => setUsers([]))
    void getDepartmentTree()
      .then((tree) => setDepartments(flattenDepts(tree ?? [])))
      .catch(() => setDepartments([]))
  }, [loadFloors])

  useEffect(() => {
    if (floorId != null) void loadSeats(floorId)
  }, [floorId, loadSeats])

  const counts = useMemo(() => {
    const acc: Record<SeatStatus, number> = { online: 0, offline: 0, error: 0, unprovisioned: 0, empty: 0 }
    draft.forEach((s) => {
      acc[seatStatus(s)] += 1
    })
    return acc
  }, [draft])
  const staffedCount = Math.max(0, draft.length - counts.empty)
  const onlineRate = staffedCount > 0 ? Math.round((counts.online / staffedCount) * 100) : 0
  const riskCount = counts.error + counts.unprovisioned

  // ---- 部门配色与统计 ----

  /** 部门 id → 分类色。槽位按全局部门树的固定顺序分配，与楼层无关，保证跨楼层同色。 */
  const deptColor = useMemo(() => {
    const palette = DEPT_COLORS[themeMode]
    const map = new Map<number, string>()
    departments.forEach((d, i) => {
      map.set(d.id, i < palette.length ? palette[i] : DEPT_FALLBACK[themeMode])
    })
    return map
  }, [departments, themeMode])

  const deptName = useMemo(() => new Map(departments.map((d) => [d.id, d.name])), [departments])
  const userDeptId = useMemo(() => new Map(users.map((u) => [u.id, u.department_id ?? 0])), [users])

  /** seatId → 部门分类色（工位底垫前缘的 accent 条） */
  const accents = useMemo(() => {
    const map = new Map<number, string>()
    draft.forEach((v) => {
      if (!v.user) return
      const deptId = userDeptId.get(v.user.id) ?? 0
      const color = deptId ? deptColor.get(deptId) : undefined
      if (color) map.set(v.seat.id, color)
    })
    return map
  }, [draft, userDeptId, deptColor])

  const deptStats = useMemo(() => {
    const acc = new Map<number, number>()
    draft.forEach((v) => {
      if (!v.user) return
      const deptId = userDeptId.get(v.user.id) ?? 0
      acc.set(deptId, (acc.get(deptId) ?? 0) + 1)
    })
    return [...acc.entries()]
      .map(([deptId, count]) => ({
        deptId,
        name: deptId ? (deptName.get(deptId) ?? t('部门 {{n}}', { n: deptId })) : t('未分组'),
        color: deptId ? (deptColor.get(deptId) ?? DEPT_FALLBACK[themeMode]) : DEPT_FALLBACK[themeMode],
        count,
      }))
      .sort((a, b) => b.count - a.count)
  }, [draft, userDeptId, deptName, deptColor, themeMode, t])

  /** 浏览态部门岛。空位与未挂部门员工各自成组，不伪造员工或设备状态。 */
  const departmentPods = useMemo<OfficeDepartmentPod[]>(() => {
    const groups = new Map<string, OfficeDepartmentPod>()
    draft.forEach((view) => {
      const deptId = view.user ? (userDeptId.get(view.user.id) ?? 0) : 0
      const key = view.user ? (deptId ? `dept:${deptId}` : 'unassigned') : 'shared'
      const group = groups.get(key) ?? {
        key,
        name: view.user
          ? deptId
            ? (deptName.get(deptId) ?? t('部门 {{n}}', { n: deptId }))
            : t('未分组')
          : t('共享工位'),
        color: deptId ? (deptColor.get(deptId) ?? DEPT_FALLBACK[themeMode]) : DEPT_FALLBACK[themeMode],
        seatIds: [],
      }
      group.seatIds.push(view.seat.id)
      groups.set(key, group)
    })
    return [...groups.values()].sort((a, b) => b.seatIds.length - a.seatIds.length || a.name.localeCompare(b.name))
  }, [draft, userDeptId, deptName, deptColor, themeMode, t])

  useEffect(() => {
    if (editing || (activePodKey !== 'all' && !departmentPods.some((pod) => pod.key === activePodKey))) {
      setActivePodKey('all')
    }
  }, [activePodKey, departmentPods, editing])

  const seatPodKey = useMemo(() => {
    const map = new Map<number, string>()
    departmentPods.forEach((pod) => pod.seatIds.forEach((seatId) => map.set(seatId, pod.key)))
    return map
  }, [departmentPods])

  /** 异常优先 > 未开通；点击定位到工位详情 */
  const riskSeats = useMemo(
    () =>
      draft
        .map((v) => ({ view: v, status: seatStatus(v) }))
        .filter((r) => r.status === 'error' || r.status === 'unprovisioned')
        .sort((a, b) => (a.status === b.status ? 0 : a.status === 'error' ? -1 : 1))
        .map((r) => {
          const deptId = r.view.user ? (userDeptId.get(r.view.user.id) ?? 0) : 0
          return {
            id: r.view.seat.id,
            name:
              r.view.seat.label ||
              (r.view.user ? r.view.user.nickname || r.view.user.username : t('工位 {{n}}', { n: r.view.seat.id })),
            dept: deptId ? (deptName.get(deptId) ?? '') : '',
            podKey: seatPodKey.get(r.view.seat.id) ?? 'shared',
            status: r.status,
            statusLabel: STATUS_META[r.status].label,
            color: SEAT_STATUS_COLORS[r.status],
          }
        }),
    [draft, userDeptId, deptName, seatPodKey, t],
  )

  const visiblePods = useMemo(
    () => (activePodKey === 'all' ? departmentPods : departmentPods.filter((pod) => pod.key === activePodKey)),
    [activePodKey, departmentPods],
  )

  const visibleRiskSeats = useMemo(
    () => (activePodKey === 'all' ? riskSeats : riskSeats.filter((seat) => seat.podKey === activePodKey)),
    [activePodKey, riskSeats],
  )

  const focusRiskSeat = (seat: { id: number; podKey: string }) => {
    setActivePodKey(seat.podKey)
    setSelectedSeatId(seat.id)
  }

  const riskAreas = useMemo(() => {
    const podByKey = new Map(departmentPods.map((pod) => [pod.key, pod]))
    const groups = new Map<string, { key: string; name: string; color: string; count: number; errors: number }>()
    visibleRiskSeats.forEach((seat) => {
      const pod = podByKey.get(seat.podKey)
      const group = groups.get(seat.podKey) ?? {
        key: seat.podKey,
        name: (pod?.name ?? seat.dept) || t('共享工位'),
        color: pod?.color ?? seat.color,
        count: 0,
        errors: 0,
      }
      group.count += 1
      if (seat.status === 'error') group.errors += 1
      groups.set(seat.podKey, group)
    })
    return [...groups.values()].sort((a, b) => b.errors - a.errors || b.count - a.count)
  }, [departmentPods, visibleRiskSeats, t])

  const statTiles = useMemo(
    () => [
      { key: 'online', label: '在线', value: counts.online, dot: SEAT_STATUS_COLORS.online },
      { key: 'offline', label: '离线', value: counts.offline, dot: SEAT_STATUS_COLORS.offline },
      { key: 'error', label: '异常', value: counts.error, dot: SEAT_STATUS_COLORS.error },
      { key: 'unprovisioned', label: '未开通', value: counts.unprovisioned, dot: SEAT_STATUS_COLORS.unprovisioned },
      { key: 'empty', label: '空位', value: counts.empty, dot: SEAT_STATUS_COLORS.empty },
      { key: 'department', label: '部门', value: deptStats.length, dot: null },
    ],
    [counts, deptStats.length],
  )

  // ---- 楼层 ----

  const openFloorModal = (target?: HermesOfficeFloor) => {
    floorForm.setFieldsValue({
      name: target?.name ?? '',
      width: target?.width ?? 20,
      depth: target?.depth ?? 15,
      sort: target?.sort ?? 0,
    })
    setFloorModalOpen(true)
  }

  const submitFloor = async () => {
    const values = await floorForm.validateFields().catch(() => null)
    if (!values) return
    setSaving(true)
    try {
      if (floor && floorModalIsEdit) {
        await updateOfficeFloor(floor.id, values)
        message.success(t('楼层已更新'))
      } else {
        const created = await createOfficeFloor(values)
        message.success(t('楼层已创建'))
        setFloorId(created.id)
      }
      setFloorModalOpen(false)
      await loadFloors()
      if (floorId != null) await loadSeats(floorId)
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const removeFloor = async () => {
    if (!floor) return
    try {
      await deleteOfficeFloor(floor.id)
      message.success(t('楼层已删除'))
      setFloorId(null)
      reset([])
      await loadFloors()
    } catch {
      // Request layer already reports the server message.
    }
  }

  // ---- 工位保存 ----

  const saveSeats = async () => {
    if (floorId == null) return
    setSaving(true)
    try {
      await bulkUpsertOfficeSeats(floorId, payload)
      message.success(t('工位布局已保存'))
      await loadSeats(floorId)
      setEditing(false)
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const leaveEditing = () => {
    if (!dirty) {
      setEditing(false)
      return
    }
    Modal.confirm({
      title: t('放弃未保存的布局改动？'),
      content: t('拖动、旋转、绑定与新增的工位都会丢失。'),
      okText: t('放弃改动'),
      cancelText: t('继续编辑'),
      onOk: () => {
        if (floorId != null) void loadSeats(floorId)
        setEditing(false)
      },
    })
  }

  const sceneHeight = 'clamp(620px, calc(100vh - 156px), 880px)'

  return (
    <div className="page-list hermes-office-page">
      <div className="hermes-office-shell">
        {floor ? (
          <Spin spinning={loading}>
            <div className="hermes-office-stage" style={{ height: sceneHeight, minHeight: 500 }}>
              <div className="hermes-office-canvas">
                <ErrorBoundary
                  inline
                  title={t('3D 场景渲染异常')}
                  description={t('WebGL 上下文异常或显卡资源不足，请点击重试')}
                >
                  <Scene
                    key={resetKey}
                    floor={floor}
                    seats={draft}
                    selectedSeatId={selectedSeatId}
                    editing={editing}
                    themeMode={themeMode}
                    accents={accents}
                    pods={editing ? departmentPods : visiblePods}
                    onSelectSeat={setSelectedSeatId}
                    onMoveSeat={(id, x, y) => moveSeat(id, x, y, floor.width, floor.depth)}
                    onAddSeatAt={(x, y) => setSelectedSeatId(addSeat(floor.id, x, y))}
                  />
                </ErrorBoundary>

                {hudCollapsed ? (
                  <button
                    type="button"
                    className="hermes-hud-chip"
                    onClick={() => setHudCollapsed(false)}
                    title={t('展开办公室面板')}
                    aria-label={t('展开办公室面板')}
                  >
                    <span className="hermes-hud-chip-dot" aria-hidden="true" />
                    <span className="hermes-hud-chip-name">{floor.name}</span>
                    {riskCount > 0 && <span className="hermes-hud-chip-risk">{riskCount}</span>}
                  </button>
                ) : (
                <div className="hermes-hud hermes-hud-tl">
                  <button
                    type="button"
                    className="hermes-hud-collapse"
                    onClick={() => setHudCollapsed(true)}
                    aria-label={t('收起面板')}
                    title={t('收起面板')}
                  >
                    <MinusOutlined />
                  </button>
                  <div className="hermes-hud-card">
                    <div className="hermes-hud-eyebrow">HERMES OFFICE</div>
                    <div className="hermes-hud-title">{floor.name}</div>
                    <div className="hermes-hud-sub">
                      {t('{{a}} 个工位 · {{b}} 在线 · {{c}} 风险', { a: draft.length, b: counts.online, c: riskCount })}
                    </div>
                    <div className="hermes-health-row">
                      <span>{t('在线率')}</span>
                      <div className="hermes-health-track" aria-hidden="true">
                        <span style={{ width: `${onlineRate}%` }} />
                      </div>
                      <strong>{onlineRate}%</strong>
                    </div>
                    <div className="hermes-floor-controls">
                      <Select
                        size="small"
                        value={floorId ?? undefined}
                        placeholder={t('选择楼层')}
                        onChange={(value) => {
                          setFloorId(value)
                          setSelectedSeatId(null)
                          setEditing(false)
                          setActivePodKey('all')
                        }}
                        options={floors.map((item) => ({ value: item.id, label: item.name }))}
                      />
                      {canEdit && !editing && (
                        <Tooltip title={t('新建楼层')}>
                          <Button
                            size="small"
                            icon={<PlusOutlined />}
                            aria-label={t('新建楼层')}
                            onClick={() => {
                              setFloorModalIsEdit(false)
                              openFloorModal()
                            }}
                          />
                        </Tooltip>
                      )}
                      {canEdit && !editing && (
                        <Tooltip title={t('编辑楼层')}>
                          <Button
                            size="small"
                            icon={<EditOutlined />}
                            aria-label={t('编辑楼层')}
                            onClick={() => {
                              setFloorModalIsEdit(true)
                              openFloorModal(floor)
                            }}
                          />
                        </Tooltip>
                      )}
                      {canEdit && !editing && (
                        <Popconfirm title={t('删除此楼层？该层工位会一并删除。')} onConfirm={() => void removeFloor()}>
                          <Tooltip title={t('删除楼层')}>
                            <Button size="small" danger icon={<DeleteOutlined />} aria-label={t('删除楼层')} />
                          </Tooltip>
                        </Popconfirm>
                      )}
                      <Tooltip title={t('刷新数据')}>
                        <Button
                          size="small"
                          icon={<ReloadOutlined />}
                          aria-label={t('刷新数据')}
                          loading={loading}
                          onClick={() => floorId != null && void loadSeats(floorId)}
                        />
                      </Tooltip>
                    </div>
                    {canEdit && (
                      <div className="hermes-edit-actions">
                        {editing ? (
                          <>
                            <Button
                              size="small"
                              type="primary"
                              icon={<SaveOutlined />}
                              loading={saving}
                              disabled={!dirty}
                              onClick={() => void saveSeats()}
                            >
                              {t('保存布局')}
                            </Button>
                            <Button size="small" icon={<EyeOutlined />} onClick={leaveEditing}>
                              {t('退出编辑')}
                            </Button>
                          </>
                        ) : (
                          <Button
                            size="small"
                            type="primary"
                            icon={<EditOutlined />}
                            onClick={() => {
                              setActivePodKey('all')
                              setEditing(true)
                            }}
                          >
                            {t('编辑布局')}
                          </Button>
                        )}
                      </div>
                    )}
                  </div>
                </div>
                )}
              </div>

              <aside className="hermes-office-rail">
                <div className="hermes-rail-overview">
                  <div className="hermes-view-controls">
                    <div className="hermes-live-status"><span />{t('实时')}</div>
                    <Tooltip title={t('重置视角')}>
                      <Button
                        size="small"
                        icon={<AimOutlined />}
                        aria-label={t('重置视角')}
                        onClick={() => setResetKey((key) => key + 1)}
                      />
                    </Tooltip>
                  </div>

                  <div className="hermes-rail-health" aria-label={t('设备在线率 {{n}}%', { n: onlineRate })}>
                    <div className="hermes-rail-health-head">
                      <span>{t('设备健康度')}</span>
                      <strong>{onlineRate}%</strong>
                    </div>
                    <div className="hermes-status-stack" aria-hidden="true">
                      {([
                        ['online', counts.online],
                        ['error', counts.error],
                        ['unprovisioned', counts.unprovisioned],
                        ['offline', counts.offline],
                        ['empty', counts.empty],
                      ] as Array<[SeatStatus, number]>).map(([status, count]) => (
                        count > 0 && (
                          <span
                            key={status}
                            style={{
                              width: `${(count / Math.max(1, draft.length)) * 100}%`,
                              background: SEAT_STATUS_COLORS[status],
                            }}
                          />
                        )
                      ))}
                    </div>
                    <div className="hermes-rail-health-meta">
                      <span><i style={{ background: SEAT_STATUS_COLORS.online }} />{t('{{n}} 在线', { n: counts.online })}</span>
                      <span><i style={{ background: riskCount ? SEAT_STATUS_COLORS.error : SEAT_STATUS_COLORS.empty }} />{t('{{n}} 风险', { n: riskCount })}</span>
                      <span><i style={{ background: SEAT_STATUS_COLORS.offline }} />{t('{{n}} 离线', { n: counts.offline })}</span>
                    </div>
                  </div>
                </div>

                {!editing && (
                  <>
                    <div className="hermes-rail-section">
                      <div className="hermes-hud-rail-title">{t('区域')}</div>
                      <div className="hermes-region-list">
                        <button
                          type="button"
                          className={`hermes-region-option${activePodKey === 'all' ? ' is-active' : ''}`}
                          onClick={() => {
                            setActivePodKey('all')
                            setSelectedSeatId(null)
                          }}
                        >
                          {t('全景')}
                          <span className="hermes-region-count">{draft.length}</span>
                        </button>
                        {departmentPods.map((pod) => (
                          <button
                            key={pod.key}
                            type="button"
                            className={`hermes-region-option${activePodKey === pod.key ? ' is-active' : ''}`}
                            onClick={() => {
                              setActivePodKey(pod.key)
                              setSelectedSeatId(null)
                            }}
                          >
                            <span className="hermes-seat-pill-dot" style={{ background: pod.color }} />
                            <span>{pod.name}</span>
                            <span className="hermes-region-count">{pod.seatIds.length}</span>
                          </button>
                        ))}
                      </div>
                    </div>

                    <div className="hermes-rail-section hermes-risk-area-section">
                      <div className="hermes-hud-rail-title">{t('风险区域')}</div>
                      {riskAreas.length ? (
                        <div className="hermes-risk-area-list">
                          {riskAreas.slice(0, 4).map((area) => (
                            <button
                              key={area.key}
                              type="button"
                              className={`hermes-risk-area${activePodKey === area.key ? ' is-active' : ''}`}
                              onClick={() => {
                                setActivePodKey(area.key)
                                setSelectedSeatId(null)
                              }}
                            >
                              <span className="hermes-seat-pill-dot" style={{ background: area.color }} />
                              <span className="hermes-risk-name">{area.name}</span>
                              <span className="hermes-risk-count">{t('{{n}} 风险', { n: area.count })}</span>
                              <span className="hermes-risk-area-sub">{t('{{n}} 异常', { n: area.errors })}</span>
                            </button>
                          ))}
                        </div>
                      ) : (
                        <div className="hermes-risk-clear">
                          <span className="hermes-seat-pill-dot" />
                          {t('当前区域状态正常')}
                        </div>
                      )}
                    </div>

                    <div className="hermes-rail-section hermes-priority-section">
                      <div className="hermes-hud-rail-title">{t('异常优先工位')}</div>
                      {visibleRiskSeats.length ? (
                        <div className="hermes-priority-list">
                          {visibleRiskSeats.slice(0, 5).map((seat) => (
                            <button
                              key={seat.id}
                              type="button"
                              className="hermes-risk-chip"
                              onClick={() => focusRiskSeat(seat)}
                            >
                              <span className="hermes-seat-pill-dot" style={{ background: seat.color }} />
                              <span className="hermes-risk-name">{seat.name}</span>
                              <span className="hermes-risk-meta">{t(seat.statusLabel)}</span>
                            </button>
                          ))}
                          {visibleRiskSeats.length > 5 && (
                            <div className="hermes-risk-more">{t('还有 {{n}} 个', { n: visibleRiskSeats.length - 5 })}</div>
                          )}
                        </div>
                      ) : (
                        <div className="hermes-risk-empty">{t('暂无优先项')}</div>
                      )}
                    </div>
                  </>
                )}

                {editing && (
                  <div className="hermes-edit-mode-panel">
                    <div className="hermes-hud-rail-title">{t('编辑模式')}</div>
                    <div className="hermes-edit-mode-state">{dirty ? t('布局有未保存改动') : t('布局尚未修改')}</div>
                  </div>
                )}
              </aside>
            </div>
          </Spin>
        ) : (
          <div className="hermes-office-empty" style={{ height: sceneHeight, minHeight: 500 }}>
            <GlassEmpty text={loading ? '加载中' : '还没有办公室楼层'} />
            {canEdit && !loading && (
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => {
                  setFloorModalIsEdit(false)
                  openFloorModal()
                }}
              >
                {t('新建楼层')}
              </Button>
            )}
          </div>
        )}

        {floor && (
          <div className="hermes-office-metrics">
            <div className="hermes-metric-panel hermes-metric-panel-wide">
              <div className="hermes-metric-title">{t('实时概览')}</div>
              <div className="hermes-stat-row">
                {statTiles.map((tile) => (
                  <div key={tile.key} className={`hermes-stat-tile is-${tile.key}`}>
                    <div className="hermes-stat-label">
                      {tile.dot && <span className="hermes-seat-pill-dot" style={{ background: tile.dot }} />}
                      {t(tile.label)}
                    </div>
                    <StatValue value={tile.value} />
                  </div>
                ))}
              </div>
            </div>

            <div className="hermes-metric-panel">
              <div className="hermes-metric-title">{t('部门分布')}</div>
              {deptStats.length ? (
                <div className="hermes-dept-list">
                  {deptStats.slice(0, 6).map((d) => (
                    <button
                      key={d.deptId}
                      type="button"
                      className={`hermes-dept-row hermes-dept-row-btn hermes-dept-filter${activePodKey === (d.deptId ? `dept:${d.deptId}` : 'unassigned') ? ' is-active' : ''}`}
                      onClick={() => {
                        setActivePodKey(d.deptId ? `dept:${d.deptId}` : 'unassigned')
                        setSelectedSeatId(null)
                      }}
                    >
                      <span className="hermes-seat-pill-dot" style={{ background: d.color }} />
                      <span className="hermes-dept-copy">
                        <span className="hermes-dept-name">{d.name}</span>
                        <span className="hermes-dept-track" aria-hidden="true">
                          <span style={{ width: `${Math.round((d.count / Math.max(1, staffedCount)) * 100)}%`, background: d.color }} />
                        </span>
                      </span>
                      <span className="hermes-dept-count">{d.count}</span>
                    </button>
                  ))}
                  {deptStats.length > 6 && (
                    <div className="hermes-dept-row">
                      <span className="hermes-seat-pill-dot" style={{ background: DEPT_FALLBACK[themeMode] }} />
                      <span className="hermes-dept-name">{t('其他')}</span>
                      <span className="hermes-dept-count">
                        {deptStats.slice(6).reduce((sum, d) => sum + d.count, 0)}
                      </span>
                    </div>
                  )}
                </div>
              ) : (
                <div className="cell-muted">{t('还没有工位绑定员工')}</div>
              )}
            </div>

            <div className="hermes-metric-panel">
              <div className="hermes-metric-title">{t('优先关注')}</div>
              {visibleRiskSeats.length ? (
                <div className="hermes-dept-list">
                  {visibleRiskSeats.slice(0, 4).map((r) => (
                    <button
                      key={r.id}
                      type="button"
                      className="hermes-dept-row hermes-dept-row-btn"
                      onClick={() => focusRiskSeat(r)}
                    >
                      <span className="hermes-seat-pill-dot" style={{ background: r.color }} />
                      <span className="hermes-priority-copy">
                        <span className="hermes-dept-name">{r.name}</span>
                        <span className="hermes-priority-dept">{r.dept || t('共享工位')}</span>
                      </span>
                      <span className="hermes-dept-count">{t(r.statusLabel)}</span>
                    </button>
                  ))}
                </div>
              ) : (
                <div className="cell-muted">{t('当前区域状态正常')}</div>
              )}
            </div>
          </div>
        )}
      </div>

      <Drawer
        title={selected?.seat.label || t('工位详情')}
        open={!!selected}
        size={420}
        onClose={() => setSelectedSeatId(null)}
        destroyOnHidden
        extra={
          editing && selected ? (
            <Popconfirm
              title={t('从布局中移除此工位？')}
              onConfirm={() => {
                removeSeat(selected.seat.id)
                setSelectedSeatId(null)
              }}
            >
              <Button danger size="small" icon={<DeleteOutlined />}>
                {t('移除')}
              </Button>
            </Popconfirm>
          ) : null
        }
      >
        {selected && (
          <>
            {editing && (
              <Space direction="vertical" style={{ width: '100%', marginBottom: 20 }}>
                <div>
                  <div className="cell-muted" style={{ marginBottom: 4 }}>{t('工位号')}</div>
                  <Input
                    value={selected.seat.label}
                    onChange={(e) => setLabel(selected.seat.id, e.target.value)}
                    maxLength={40}
                  />
                </div>
                <div>
                  <div className="cell-muted" style={{ marginBottom: 4 }}>{t('朝向（度）')}</div>
                  <InputNumber
                    style={{ width: '100%' }}
                    min={0}
                    max={359}
                    step={15}
                    value={selected.seat.rotation}
                    onChange={(v) => rotateSeat(selected.seat.id, Number(v ?? 0))}
                  />
                </div>
                <div>
                  <div className="cell-muted" style={{ marginBottom: 4 }}>{t('绑定员工')}</div>
                  <Select
                    style={{ width: '100%' }}
                    allowClear
                    showSearch
                    optionFilterProp="label"
                    placeholder={t('空位')}
                    value={selected.seat.user_id || undefined}
                    onChange={(v) => {
                      const u = users.find((item) => item.id === v)
                      bindUser(
                        selected.seat.id,
                        v ?? 0,
                        u ? { id: u.id, username: u.username, nickname: u.nickname ?? '' } : null,
                      )
                    }}
                    options={users.map((u) => ({ value: u.id, label: `${u.nickname || u.username} (${u.username})` }))}
                  />
                </div>
              </Space>
            )}

            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label={t('状态')}>
                <StatusPill
                  tone={STATUS_META[seatStatus(selected)].tone}
                  label={STATUS_META[seatStatus(selected)].label}
                  pulse={false}
                />
              </Descriptions.Item>
              <Descriptions.Item label={t('坐标')}>
                {selected.seat.x.toFixed(1)} , {selected.seat.y.toFixed(1)}
              </Descriptions.Item>
              <Descriptions.Item label={t('员工')}>
                {selected.user ? selected.user.nickname || selected.user.username : t('空位')}
              </Descriptions.Item>
              <Descriptions.Item label={t('部门')}>
                {selected.user ? (deptName.get(userDeptId.get(selected.user.id) ?? 0) ?? '—') : '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('设备')}>{selected.device?.device_id || t('未注册')}</Descriptions.Item>
              <Descriptions.Item label={t('Hermes 版本')}>{selected.device?.hermes_version || '—'}</Descriptions.Item>
              <Descriptions.Item label={t('最近心跳')}>
                {selected.device?.last_seen_at ? formatDateTime(selected.device.last_seen_at) : '—'}
              </Descriptions.Item>
            </Descriptions>
          </>
        )}
      </Drawer>

      <Modal
        title={floorModalIsEdit ? t('编辑楼层') : t('新建楼层')}
        open={floorModalOpen}
        onOk={() => void submitFloor()}
        onCancel={() => setFloorModalOpen(false)}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={floorForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label={t('楼层名称')} rules={[{ required: true, max: 80 }]}>
            <Input placeholder={t('如：3 楼研发区')} />
          </Form.Item>
          <Form.Item name="width" label={t('平面宽度（米）')} rules={[{ required: true }]}>
            <InputNumber min={1} max={500} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="depth" label={t('平面深度（米）')} rules={[{ required: true }]}>
            <InputNumber min={1} max={500} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="sort" label={t('排序')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
