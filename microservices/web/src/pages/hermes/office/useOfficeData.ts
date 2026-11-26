// 办公室页面基础数据加载：楼层列表、全量员工（分页拉取）、扁平部门树与当前楼层工位。
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  listOfficeFloors,
  listOfficeSeats,
  type HermesOfficeFloor,
  type HermesOfficeSeatView,
} from '@/api/ai/hermes-office'
import { getDepartmentTree } from '@/api/system/department'
import { getUserList } from '@/api/system/user'
import type { Department, SystemUser } from '@/types'
import { fetchAllHermesPages } from '../admin-contract'
import { flattenDepts } from './constants'

export function useOfficeData(onResetSeats: (seats: HermesOfficeSeatView[]) => void) {
  const [loading, setLoading] = useState(false)
  const [floors, setFloors] = useState<HermesOfficeFloor[]>([])
  const [floorId, setFloorId] = useState<number | null>(null)
  const [users, setUsers] = useState<SystemUser[]>([])
  const [departments, setDepartments] = useState<Department[]>([])

  const floor = useMemo(() => floors.find((f) => f.id === floorId) ?? null, [floors, floorId])

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
        onResetSeats(list ?? [])
      } catch {
        onResetSeats([])
      } finally {
        setLoading(false)
      }
    },
    [onResetSeats],
  )

  useEffect(() => {
    let active = true
    void Promise.resolve().then(() => {
      if (active) void loadFloors()
    })
    // 3D 办公室需要全部员工的工位状态：逐页真实拉取，不放大 page_size 伪装全量
    fetchAllHermesPages<SystemUser>((page, pageSize) => getUserList({ page, page_size: pageSize }))
      .then((all) => {
        if (active) setUsers(all)
      })
      .catch(() => {
        if (active) setUsers([])
      })
    void getDepartmentTree()
      .then((tree) => {
        if (active) setDepartments(flattenDepts(tree ?? []))
      })
      .catch(() => {
        if (active) setDepartments([])
      })
    return () => {
      active = false
    }
  }, [loadFloors])

  useEffect(() => {
    if (floorId == null) return
    let active = true
    void Promise.resolve().then(() => {
      if (active) void loadSeats(floorId)
    })
    return () => {
      active = false
    }
  }, [floorId, loadSeats])

  return {
    loading,
    floors,
    floorId,
    setFloorId,
    floor,
    users,
    departments,
    loadFloors,
    loadSeats,
  }
}
