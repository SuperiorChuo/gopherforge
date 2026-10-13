// 拖拽编辑器的本地暂存层：所有改动先落在 draft 里，点「保存」才整层提交一次。
// 新建工位用递减的负数临时 id 占位，提交时转成无 id 的新增项。
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { HermesOfficeSeat, HermesOfficeSeatView } from '@/api/ai/hermes-office'

export interface SeatDraft extends HermesOfficeSeatView {
  /** 临时 id（<0）表示尚未落库 */
  isNew?: boolean
}

function clamp(v: number, max: number) {
  if (Number.isNaN(v)) return 0
  return Math.min(Math.max(v, 0), max)
}

export function useSeatEditor(serverSeats: HermesOfficeSeatView[]) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<SeatDraft[]>(serverSeats)
  const [dirty, setDirty] = useState(false)
  const tempId = useRef(-1)

  /** 服务端数据到达或切换楼层时重置暂存 */
  const reset = useCallback((seats: HermesOfficeSeatView[]) => {
    setDraft(seats)
    setDirty(false)
    tempId.current = -1
  }, [])

  const patchSeat = useCallback((id: number, patch: Partial<HermesOfficeSeat>) => {
    setDraft((prev) =>
      prev.map((item) => (item.seat.id === id ? { ...item, seat: { ...item.seat, ...patch } } : item)),
    )
    setDirty(true)
  }, [])

  const moveSeat = useCallback(
    (id: number, x: number, y: number, width: number, depth: number) => {
      patchSeat(id, { x: clamp(x, width), y: clamp(y, depth) })
    },
    [patchSeat],
  )

  const rotateSeat = useCallback(
    (id: number, deg: number) => {
      const r = ((deg % 360) + 360) % 360
      patchSeat(id, { rotation: r })
    },
    [patchSeat],
  )

  const setLabel = useCallback((id: number, label: string) => patchSeat(id, { label }), [patchSeat])

  /** 绑定员工。一人一工位：先解掉该员工在别处的绑定，避免提交时被后端整批拒绝。 */
  const bindUser = useCallback(
    (id: number, userId: number, user: SeatDraft['user']) => {
      setDraft((prev) =>
        prev.map((item) => {
          if (item.seat.id === id) {
            return { ...item, seat: { ...item.seat, user_id: userId }, user: userId > 0 ? user : null, device: null }
          }
          if (userId > 0 && item.seat.user_id === userId) {
            return { ...item, seat: { ...item.seat, user_id: 0 }, user: null, device: null }
          }
          return item
        }),
      )
      setDirty(true)
    },
    [],
  )

  const addSeat = useCallback((floorId: number, x: number, y: number) => {
    const id = tempId.current--
    setDraft((prev) => [
      ...prev,
      {
        isNew: true,
        seat: {
          id,
          tenant_id: 0,
          floor_id: floorId,
          label: t('工位 {{n}}', { n: prev.length + 1 }),
          x,
          y,
          rotation: 0,
          user_id: 0,
        },
        user: null,
        device: null,
      },
    ])
    setDirty(true)
    return id
  }, [t])

  const removeSeat = useCallback((id: number) => {
    setDraft((prev) => prev.filter((item) => item.seat.id !== id))
    setDirty(true)
  }, [])

  /** 整层提交载荷：新建项不带 id，既有项带 id 走更新 */
  const payload = useMemo(
    () =>
      draft.map((item) => ({
        ...(item.seat.id > 0 ? { id: item.seat.id } : {}),
        label: item.seat.label,
        x: item.seat.x,
        y: item.seat.y,
        rotation: item.seat.rotation,
        user_id: item.seat.user_id,
      })),
    [draft],
  )

  return { draft, dirty, reset, moveSeat, rotateSeat, setLabel, bindUser, addSeat, removeSeat, payload }
}
