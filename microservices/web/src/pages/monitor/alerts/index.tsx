import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Form, Grid, Segmented } from 'antd'
import { BellOutlined, HistoryOutlined } from '@ant-design/icons'
import {
  deleteAlertRule,
  evaluateAlertRule,
  getAlertEvents,
  getAlertMetrics,
  getAlertRules,
  getAlertSummary,
  type AlertEventListParams,
  type AlertMetricDefinition,
  type AlertRuleListParams,
  type MonitorAlertEvent,
  type MonitorAlertRule,
  type MonitorAlertSummary,
} from '@/api/monitor'
import { message } from '@/utils/feedback'
import EventsTab from './EventsTab'
import {
  defaultEventParams,
  defaultRuleParams,
  type AlertEventFilterValues,
  type AlertRuleFilterValues,
  type ViewMode,
} from './helpers'
import OverviewStrip from './OverviewStrip'
import RuleFormModal from './RuleFormModal'
import RulesTab from './RulesTab'
import './style.css'

export default function AlertRulesPage() {
  const { t } = useTranslation()
  const [view, setView] = useState<ViewMode>('rules')
  const [metrics, setMetrics] = useState<AlertMetricDefinition[]>([])
  const [rules, setRules] = useState<MonitorAlertRule[]>([])
  const [summary, setSummary] = useState<MonitorAlertSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(true)
  const [summaryFailed, setSummaryFailed] = useState(false)
  const [ruleTotal, setRuleTotal] = useState(0)
  const [ruleParams, setRuleParams] = useState<AlertRuleListParams>(defaultRuleParams)
  const [ruleLoading, setRuleLoading] = useState(false)
  const [ruleLoaded, setRuleLoaded] = useState(false)
  const [ruleFailed, setRuleFailed] = useState(false)
  const [events, setEvents] = useState<MonitorAlertEvent[]>([])
  const [eventTotal, setEventTotal] = useState(0)
  const [eventParams, setEventParams] = useState<AlertEventListParams>(defaultEventParams)
  const [eventLoading, setEventLoading] = useState(false)
  const [eventLoaded, setEventLoaded] = useState(false)
  const [eventFailed, setEventFailed] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<MonitorAlertRule | null>(null)
  const [evaluatingID, setEvaluatingID] = useState<number | null>(null)
  const [ruleFilterForm] = Form.useForm<AlertRuleFilterValues>()
  const [eventFilterForm] = Form.useForm<AlertEventFilterValues>()
  const ruleRequestRef = useRef(false)
  const eventRequestRef = useRef(false)
  const summaryRequestRef = useRef(false)
  const screens = Grid.useBreakpoint()
  const compactTable = !screens.md

  const metricMap = useMemo(
    () => new Map(metrics.map((metric) => [metric.key, metric])),
    [metrics],
  )

  const fetchRules = useCallback(async (params: AlertRuleListParams, quiet = false) => {
    if (ruleRequestRef.current) return
    ruleRequestRef.current = true
    if (!quiet) setRuleLoading(true)
    try {
      const result = await getAlertRules(params)
      setRules(result.list ?? [])
      setRuleTotal(result.total ?? 0)
      setRuleLoaded(true)
      setRuleFailed(false)
    } catch {
      setRuleFailed(true)
      if (!quiet) message.error(t('获取告警规则失败'))
    } finally {
      ruleRequestRef.current = false
      if (!quiet) setRuleLoading(false)
    }
  }, [t])

  const fetchEvents = useCallback(async (params: AlertEventListParams, quiet = false) => {
    if (eventRequestRef.current) return
    eventRequestRef.current = true
    if (!quiet) setEventLoading(true)
    try {
      const result = await getAlertEvents(params)
      setEvents(result.list ?? [])
      setEventTotal(result.total ?? 0)
      setEventLoaded(true)
      setEventFailed(false)
    } catch {
      setEventFailed(true)
      if (!quiet) message.error(t('获取告警事件失败'))
    } finally {
      eventRequestRef.current = false
      if (!quiet) setEventLoading(false)
    }
  }, [t])

  const fetchSummary = useCallback(async (quiet = false) => {
    if (summaryRequestRef.current) return
    summaryRequestRef.current = true
    if (!quiet) setSummaryLoading(true)
    try {
      const result = await getAlertSummary()
      setSummary(result)
      setSummaryFailed(false)
    } catch {
      setSummaryFailed(true)
      if (!quiet) message.error(t('获取告警概览失败'))
    } finally {
      summaryRequestRef.current = false
      if (!quiet) setSummaryLoading(false)
    }
  }, [t])

  useEffect(() => {
    let active = true
    getAlertMetrics()
      .then((result) => {
        if (active) setMetrics(result.list ?? [])
      })
      .catch(() => {
        if (active) message.error(t('获取指标目录失败'))
      })
    return () => {
      active = false
    }
  }, [t])

  useEffect(() => {
    let active = true
    void Promise.resolve().then(() => {
      if (active) void fetchRules(ruleParams)
    })
    return () => {
      active = false
    }
  }, [fetchRules, ruleParams])

  useEffect(() => {
    let active = true
    void Promise.resolve().then(() => {
      if (active) void fetchSummary()
    })
    return () => {
      active = false
    }
  }, [fetchSummary])

  useEffect(() => {
    let active = true
    void Promise.resolve().then(() => {
      if (active) void fetchEvents(eventParams)
    })
    return () => {
      active = false
    }
  }, [eventParams, fetchEvents])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchRules(ruleParams, true)
      void fetchSummary(true)
      if (view === 'events') void fetchEvents(eventParams, true)
    }, 30_000)
    return () => window.clearInterval(timer)
  }, [eventParams, fetchEvents, fetchRules, fetchSummary, ruleParams, view])

  const openCreate = () => {
    setEditingRule(null)
    setModalOpen(true)
  }

  const openEdit = (rule: MonitorAlertRule) => {
    setEditingRule(rule)
    setModalOpen(true)
  }

  const handleRuleSaved = async () => {
    setModalOpen(false)
    await Promise.all([fetchRules(ruleParams), fetchSummary(true)])
  }

  const removeRule = async (rule: MonitorAlertRule) => {
    try {
      await deleteAlertRule(rule.id)
      message.success(t('告警规则已删除'))
      const nextPage = rules.length === 1 && ruleParams.page > 1 ? ruleParams.page - 1 : ruleParams.page
      setRuleParams({ ...ruleParams, page: nextPage })
      void Promise.all([fetchSummary(true), fetchEvents(eventParams, true)])
    } catch {
      message.error(t('删除告警规则失败'))
    }
  }

  const evaluateRule = async (rule: MonitorAlertRule) => {
    setEvaluatingID(rule.id)
    try {
      const result = await evaluateAlertRule(rule.id)
      if (result.event?.status === 'firing') {
        message.warning(t('评估完成，规则进入告警状态'))
      } else if (result.event?.status === 'resolved') {
        message.success(t('评估完成，告警已恢复'))
      } else {
        message.success(t('评估完成'))
      }
      await Promise.all([fetchRules(ruleParams, true), fetchSummary(true), fetchEvents(eventParams, true)])
    } catch {
      message.error(t('评估失败，规则状态已记录采集错误'))
      void fetchRules(ruleParams, true)
    } finally {
      setEvaluatingID(null)
    }
  }

  return (
    <div className="page-list alert-rules-page">
      <OverviewStrip
        summary={summary}
        loading={summaryLoading}
        failed={summaryFailed}
        onRefresh={() => { void fetchSummary() }}
      />

      <div className="alert-view-switch">
        <Segmented
          value={view}
          onChange={(value) => setView(value as ViewMode)}
          options={[
            { value: 'rules', label: t('规则'), icon: <BellOutlined /> },
            { value: 'events', label: t('事件'), icon: <HistoryOutlined /> },
          ]}
        />
      </div>

      {view === 'rules' ? (
        <RulesTab
          filterForm={ruleFilterForm}
          rules={rules}
          total={ruleTotal}
          params={ruleParams}
          loading={ruleLoading}
          loaded={ruleLoaded}
          failed={ruleFailed}
          metrics={metrics}
          metricMap={metricMap}
          compactTable={compactTable}
          fixActionsRight={Boolean(screens.lg)}
          evaluatingID={evaluatingID}
          onParamsChange={setRuleParams}
          onRefresh={() => { void fetchRules(ruleParams) }}
          onCreate={openCreate}
          onEdit={openEdit}
          onDelete={removeRule}
          onEvaluate={evaluateRule}
        />
      ) : (
        <EventsTab
          filterForm={eventFilterForm}
          events={events}
          total={eventTotal}
          params={eventParams}
          loading={eventLoading}
          loaded={eventLoaded}
          failed={eventFailed}
          metricMap={metricMap}
          compactTable={compactTable}
          onParamsChange={setEventParams}
          onRefresh={() => { void fetchEvents(eventParams) }}
        />
      )}

      <RuleFormModal
        open={modalOpen}
        editingRule={editingRule}
        metrics={metrics}
        metricMap={metricMap}
        onCancel={() => setModalOpen(false)}
        onSaved={handleRuleSaved}
      />
    </div>
  )
}
