import { useCallback, useEffect, useRef, useState } from 'react'
import { message } from '@/utils/feedback'
import * as API from '@/api/system/edgeCert'
import type { EdgeCertCapabilities, EdgeCertTask, EdgeCertTaskKind } from '@/api/system/edgeCert'
import {
  capabilityOff,
  normalizeRow,
  readError,
  saveBlob,
  taskKindMeta,
  terminalTaskStatuses,
  type CapabilityName,
  type CertRow,
  type TaskModalState,
} from './helpers'

export function useEdgeCerts() {
  const [rows, setRows] = useState<CertRow[]>([])
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [capabilities, setCapabilities] = useState<EdgeCertCapabilities | null>(null)
  const [capabilitiesReason, setCapabilitiesReason] = useState('')
  const [runningAction, setRunningAction] = useState<string | null>(null)
  const [taskModal, setTaskModal] = useState<TaskModalState>(null)
  const listRequest = useRef(0)
  const activePolls = useRef(new Map<number, number>())

  const capability = useCallback((name: CapabilityName) => (
    capabilities?.[name] ?? { ...capabilityOff, reason: capabilitiesReason || capabilityOff.reason }
  ), [capabilities, capabilitiesReason])

  const fetchData = useCallback(async (quiet = false) => {
    const requestId = ++listRequest.current
    if (!quiet) setLoading(true)
    try {
      const list = await API.listEdgeCerts()
      if (requestId !== listRequest.current) return
      setRows((Array.isArray(list) ? list : []).map(normalizeRow))
      setLoaded(true)
      setLoadFailed(false)
    } catch {
      if (requestId !== listRequest.current) return
      setLoadFailed(true)
      if (!quiet) message.error('加载边缘证书失败')
    } finally {
      if (requestId === listRequest.current && !quiet) setLoading(false)
    }
  }, [])

  const fetchCapabilities = useCallback(async () => {
    try {
      const result = await API.getEdgeCertCapabilities()
      setCapabilities(result)
      setCapabilitiesReason('')
    } catch (error) {
      setCapabilities(null)
      setCapabilitiesReason(readError(error, '能力探测失败，危险操作已关闭'))
    }
  }, [])

  const refreshAll = useCallback(() => {
    void Promise.all([fetchData(), fetchCapabilities()])
  }, [fetchCapabilities, fetchData])

  const pollTask = useCallback((certificateId: number, initial: EdgeCertTask) => {
    const existing = activePolls.current.get(certificateId)
    if (existing) window.clearInterval(existing)

    let task = initial
    let failureCount = 0
    const poll = async () => {
      if (terminalTaskStatuses.has(task.status)) return
      try {
        task = await API.getEdgeCertTask(certificateId, task.id)
        failureCount = 0
        setRows((current) => current.map((row) => (
          row.id === certificateId ? { ...row, active_task: task } : row
        )))
        setTaskModal((current) => current?.row.id === certificateId ? {
          ...current,
          tasks: current.tasks.map((item) => item.id === task.id ? task : item),
        } : current)
        if (!terminalTaskStatuses.has(task.status)) return

        const timer = activePolls.current.get(certificateId)
        if (timer) window.clearInterval(timer)
        activePolls.current.delete(certificateId)
        setRunningAction(null)
        if (task.status === 'succeeded') {
          message.success(`${taskKindMeta[task.kind]?.label || '任务'}已完成`)
        } else {
          message.error(task.error_message || `${taskKindMeta[task.kind]?.label || '任务'}失败`)
        }
        await fetchData(true)
      } catch {
        failureCount += 1
        if (failureCount < 3) return
        const timer = activePolls.current.get(certificateId)
        if (timer) window.clearInterval(timer)
        activePolls.current.delete(certificateId)
        setRunningAction(null)
        message.warning('任务状态暂时无法刷新，请稍后手动刷新')
      }
    }

    if (terminalTaskStatuses.has(task.status)) return
    const timer = window.setInterval(() => void poll(), 2_000)
    activePolls.current.set(certificateId, timer)
    void poll()
  }, [fetchData])

  useEffect(() => {
    let cancelled = false
    const requestId = ++listRequest.current
    void API.listEdgeCerts().then(
      (list) => {
        if (cancelled || requestId !== listRequest.current) return
        setRows((Array.isArray(list) ? list : []).map(normalizeRow))
        setLoaded(true)
        setLoadFailed(false)
        setLoading(false)
      },
      () => {
        if (cancelled || requestId !== listRequest.current) return
        setLoadFailed(true)
        setLoading(false)
        message.error('加载边缘证书失败')
      },
    )
    void API.getEdgeCertCapabilities().then(
      (result) => {
        if (cancelled) return
        setCapabilities(result)
        setCapabilitiesReason('')
      },
      (error) => {
        if (cancelled) return
        setCapabilities(null)
        setCapabilitiesReason(readError(error, '能力探测失败，危险操作已关闭'))
      },
    )
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    rows.forEach((row) => {
      if (row.active_task && !terminalTaskStatuses.has(row.active_task.status) && !activePolls.current.has(row.id)) {
        pollTask(row.id, row.active_task)
      }
    })
  }, [pollTask, rows])

  useEffect(() => {
    const polls = activePolls.current
    return () => {
      polls.forEach((timer) => window.clearInterval(timer))
      polls.clear()
    }
  }, [])

  const enqueue = useCallback(async (row: CertRow, kind: EdgeCertTaskKind) => {
    const key = `${row.id}:${kind}`
    setRunningAction(key)
    try {
      const result = kind === 'issue' ? await API.issueEdgeCert(row.id)
        : kind === 'renew' ? await API.renewEdgeCert(row.id)
          : kind === 'deploy' ? await API.deployEdgeCert(row.id)
            : await API.probeEdgeCert(row.id)
      const task = result.task
      setRows((current) => current.map((item) => item.id === row.id ? { ...item, active_task: task } : item))
      message.success(result.reused ? '已有任务正在执行，已继续跟踪' : taskKindMeta[kind].submitted)
      setRunningAction(null)
      pollTask(row.id, task)
    } catch (error) {
      setRunningAction(null)
      message.error(readError(error, `${taskKindMeta[kind].label}任务提交失败`))
    }
  }, [pollTask])

  const downloadCertificate = useCallback(async (row: CertRow) => {
    setRunningAction(`${row.id}:certificate`)
    try {
      const blob = await API.downloadEdgeCertificate(row.id)
      saveBlob(blob, `${row.domain}.fullchain.pem`)
      message.success('公钥证书链已下载（不含私钥）')
    } catch {
      message.error('证书链下载失败')
    } finally {
      setRunningAction(null)
    }
  }, [])

  const loadTaskHistory = useCallback(async (row: CertRow) => {
    setTaskModal({ row, tasks: [], loading: true })
    try {
      const result = await API.listEdgeCertTasks(row.id)
      const tasks = Array.isArray(result) ? result : result.list ?? []
      setTaskModal({ row, tasks, loading: false })
    } catch {
      setTaskModal({ row, tasks: [], loading: false })
      message.error('任务记录加载失败')
    }
  }, [])

  const closeTaskHistory = useCallback(() => {
    setTaskModal(null)
  }, [])

  return {
    rows,
    loading,
    loaded,
    loadFailed,
    capabilities,
    capabilitiesReason,
    runningAction,
    taskModal,
    capability,
    fetchData,
    refreshAll,
    enqueue,
    downloadCertificate,
    loadTaskHistory,
    closeTaskHistory,
  }
}
