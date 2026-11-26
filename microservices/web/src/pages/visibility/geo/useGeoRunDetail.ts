import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { message } from '@/utils/feedback'
import { usePollingTask } from '@/hooks/usePollingTask'
import {
  compareGeoRun,
  createGeoRun,
  generateGeoAdvice,
  getGeoRun,
  saveGeoAdviceItems,
  type VisAdviceItem,
  type VisGeoCompare,
  type VisGeoResult,
  type VisGeoRun,
} from '@/api/visibility'
import type { GeoRunEngine } from './helpers'

export function useGeoRunDetail(onRefreshList: () => Promise<void>) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRun, setDetailRun] = useState<VisGeoRun | null>(null)
  const [results, setResults] = useState<VisGeoResult[]>([])
  const [activeRunId, setActiveRunId] = useState<number | null>(null)
  // AI 优化建议 + 可执行清单 + 历史对比
  const [advising, setAdvising] = useState(false)
  const [adviceItems, setAdviceItems] = useState<VisAdviceItem[]>([])
  const [compare, setCompare] = useState<VisGeoCompare | null>(null)
  const [comparePrev, setComparePrev] = useState<VisGeoRun | null>(null)
  const [compareLoading, setCompareLoading] = useState(false)
  // 详情抽屉当前展示的 run id：openRun 竞态与 onAdvice 长请求写回都以它为准
  const detailRunIdRef = useRef<number | null>(null)
  // adviceItems 的同步镜像：勾选走请求前取值不能依赖异步的 setState
  const adviceItemsRef = useRef<VisAdviceItem[]>([])
  // 保存请求串行链：并发 PUT 是整表覆盖，乱序到达会让后点的勾选被先点的旧表覆盖
  const saveChainRef = useRef<Promise<void>>(Promise.resolve())

  const syncAdviceItems = useCallback((items: VisAdviceItem[]) => {
    adviceItemsRef.current = items
    setAdviceItems(items)
  }, [])

  const loadCompare = useCallback(async (runId: number) => {
    setCompareLoading(true)
    try {
      const res = await compareGeoRun(runId)
      // 快速切换 run 详情时，慢的旧响应不得覆盖当前抽屉的对比数据
      if (detailRunIdRef.current !== runId) return
      if (res.has_previous && res.compare) {
        setCompare(res.compare)
        setComparePrev(res.previous || null)
      } else {
        setCompare(null)
        setComparePrev(null)
      }
    } catch {
      if (detailRunIdRef.current === runId) {
        setCompare(null)
        setComparePrev(null)
      }
    } finally {
      if (detailRunIdRef.current === runId) setCompareLoading(false)
    }
  }, [])

  const { polling } = usePollingTask({
    taskId: activeRunId,
    fetcher: getGeoRun,
    isDone: (d) => d.run.status !== 'running',
    onTick: (d) => {
      // 抽屉可能已切到别的 run：后台轮询不得覆盖当前展示
      if (detailRunIdRef.current === d.run.id) {
        setDetailRun(d.run)
        setResults(d.results || [])
      }
    },
    onFinish: (d) => {
      if (d.run.status === 'done') {
        message.success(t('检测完成：{{a}} 个问题中 {{b}} 个提到了你的品牌', { a: d.run.prompt_count, b: d.run.mention_count }))
        // 抽屉正开着这条 run 时，完成后自动带出历史对比
        if (detailRunIdRef.current === d.run.id) void loadCompare(d.run.id)
      } else {
        message.error(d.run.error || t('检测失败'))
      }
      void onRefreshList()
    },
    onError: () => {
      // 清掉 runId 才能对同一 run 重启轮询（后端 reused 返回相同 id）
      setActiveRunId(null)
      message.warning(t('进度查询连续失败已暂停，检测仍在后台进行，稍后点「检测记录」查看结果'))
    },
  })

  async function onRun(mode: 'stub' | 'llm', engine?: GeoRunEngine) {
    setSubmitting(true)
    try {
      const res = await createGeoRun({ mode, engine: mode === 'llm' ? engine : 'stub' })
      if (res.reused) {
        message.info(t('已有检测正在进行，为你打开进度'))
      }
      detailRunIdRef.current = res.run.id
      syncAdviceItems([])
      setCompare(null)
      setComparePrev(null)
      setDetailRun(res.run)
      setResults([])
      setActiveRunId(res.run.id)
      setDetailOpen(true)
      await onRefreshList()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('启动检测失败'))
    } finally {
      setSubmitting(false)
    }
  }

  async function onAdvice() {
    if (!detailRun) return
    const runId = detailRun.id
    setAdvising(true)
    try {
      const res = await generateGeoAdvice(runId)
      // 生成最长 3 分钟，期间用户可能切到别的 run 详情：只更新还是同一 run 的抽屉
      setDetailRun((prev) => (prev && prev.id === runId ? { ...prev, advice: res.advice } : prev))
      if (detailRunIdRef.current === runId) {
        syncAdviceItems(res.advice_items || [])
        message.success(t('优化建议与行动清单已生成'))
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('生成建议失败'))
    } finally {
      setAdvising(false)
    }
  }

  function toggleAdviceItem(id: string, done: boolean) {
    if (!detailRun) return
    const runId = detailRun.id
    // 用 ref 镜像取当前值：setState 的 updater 不保证同步执行
    adviceItemsRef.current = adviceItemsRef.current.map((it) => (it.id === id ? { ...it, done } : it))
    setAdviceItems(adviceItemsRef.current)
    // 串行发送，且发送时取 ref 最新值：连点两项时第二个请求必然包含第一个的状态，
    // 消除并发整表 PUT 乱序到达导致的静默覆盖丢失
    saveChainRef.current = saveChainRef.current.then(async () => {
      if (detailRunIdRef.current !== runId) return
      try {
        await saveGeoAdviceItems(runId, adviceItemsRef.current)
      } catch {
        // 已切到别的 run 时不回滚也不报错，避免把旧 run 的清单写进新抽屉
        if (detailRunIdRef.current !== runId) return
        message.error(t('保存勾选状态失败'))
        adviceItemsRef.current = adviceItemsRef.current.map((it) => (it.id === id ? { ...it, done: !done } : it))
        setAdviceItems(adviceItemsRef.current)
      }
    })
  }

  async function openRun(id: number) {
    detailRunIdRef.current = id
    try {
      const res = await getGeoRun(id)
      // 快速连点两条记录：慢的旧响应不得覆盖后打开的详情
      if (detailRunIdRef.current !== id) return
      setDetailRun(res.run)
      setResults(res.results || [])
      syncAdviceItems(res.advice_items || [])
      // 仍在跑的继续轮询；已结束的只在「当前轮询的就是它」时才停，
      // 不能误杀另一个正在后台轮询的 run
      setActiveRunId((prev) => (res.run.status === 'running' ? id : prev === id ? null : prev))
      setDetailOpen(true)
      if (res.run.status === 'done') {
        void loadCompare(id)
      } else {
        setCompare(null)
        setComparePrev(null)
      }
    } catch {
      if (detailRunIdRef.current === id) message.error(t('加载结果失败'))
    }
  }

  const closeDetail = useCallback(() => {
    setDetailOpen(false)
  }, [])

  return {
    submitting,
    polling,
    detailOpen,
    detailRun,
    results,
    advising,
    adviceItems,
    compare,
    comparePrev,
    compareLoading,
    onRun,
    openRun,
    onAdvice,
    loadCompare,
    toggleAdviceItem,
    closeDetail,
  }
}
