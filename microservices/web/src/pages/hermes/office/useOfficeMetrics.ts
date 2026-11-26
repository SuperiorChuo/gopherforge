// Hermes 办公室统计与视图推导：按工位草稿、部门与员工计算在线率、部门岛、风险区域与统计卡片。
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { seatStatus, type SeatStatus } from '@/api/ai/hermes-office'
import type { Department, SystemUser } from '@/types'
import {
  DEPT_COLORS,
  DEPT_FALLBACK,
  SEAT_STATUS_COLORS,
  STATUS_META,
  type DeptStat,
  type OfficeDepartmentPod,
  type RiskAreaItem,
  type RiskSeatItem,
  type StatTileItem,
} from './constants'
import type { SeatDraft } from './useSeatEditor'

export interface UseOfficeMetricsInput {
  draft: SeatDraft[]
  departments: Department[]
  users: SystemUser[]
  themeMode: 'light' | 'dark'
  editing: boolean
  selectedPodKey: string
}

export function useOfficeMetrics({
  draft,
  departments,
  users,
  themeMode,
  editing,
  selectedPodKey,
}: UseOfficeMetricsInput) {
  const { t } = useTranslation()

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

  const deptStats = useMemo<DeptStat[]>(() => {
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

  const activePodKey = useMemo(() => {
    if (editing || (selectedPodKey !== 'all' && !departmentPods.some((pod) => pod.key === selectedPodKey))) {
      return 'all'
    }
    return selectedPodKey
  }, [departmentPods, editing, selectedPodKey])

  const seatPodKey = useMemo(() => {
    const map = new Map<number, string>()
    departmentPods.forEach((pod) => pod.seatIds.forEach((seatId) => map.set(seatId, pod.key)))
    return map
  }, [departmentPods])

  /** 异常优先 > 未开通；点击定位到工位详情 */
  const riskSeats = useMemo<RiskSeatItem[]>(
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

  const riskAreas = useMemo<RiskAreaItem[]>(() => {
    const podByKey = new Map(departmentPods.map((pod) => [pod.key, pod]))
    const groups = new Map<string, RiskAreaItem>()
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

  const statTiles = useMemo<StatTileItem[]>(
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

  return {
    counts,
    staffedCount,
    onlineRate,
    riskCount,
    deptName,
    userDeptId,
    accents,
    deptStats,
    departmentPods,
    activePodKey,
    visiblePods,
    visibleRiskSeats,
    riskAreas,
    statTiles,
  }
}
