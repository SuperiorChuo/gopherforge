// 3D 办公室地图（管理员运维视图）：工位按绑定员工的 Hermes 设备状态着色。
// 渲染层在 Scene.tsx（按需懒加载），数据加载在 useOfficeData.ts，暂存层在 useSeatEditor.ts，
// 统计推导在 useOfficeMetrics.ts，本文件负责交互编排与保存。
//
// 视觉参照 Hive Office：场景四角浮 HUD（楼层信息卡 / 视图切换 / 风险工位栏），
// 底部三栏统计（实时概览 / 部门分布 / 优先关注）。部门用固定顺序的分类色板，
// 超过 8 个的部门统一折入中性灰——分类色循环复用会撞色，宁可折叠。
import { lazy, Suspense, useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Modal, Spin } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import ErrorBoundary from '@/components/common/ErrorBoundary'
import GlassEmpty from '@/components/common/GlassEmpty'
import {
  bulkUpsertOfficeSeats,
  createOfficeFloor,
  deleteOfficeFloor,
  updateOfficeFloor,
  type HermesOfficeFloor,
} from '@/api/ai/hermes-office'
import { message } from '@/utils/feedback'
import { usePermission } from '@/hooks/usePermission'
import { useThemeMode } from '@/theme/ThemeContext'
import type { FloorFormValues } from './constants'
import { FloorModal } from './FloorModal'
import { OfficeHud } from './OfficeHud'
import { OfficeMetrics } from './OfficeMetrics'
import { OfficeRail } from './OfficeRail'
import { SeatDetailDrawer } from './SeatDetailDrawer'
import { useOfficeData } from './useOfficeData'
import { useOfficeMetrics } from './useOfficeMetrics'
import { useSeatEditor } from './useSeatEditor'

const Scene = lazy(() => import('./Scene'))

const SCENE_HEIGHT = 'clamp(620px, calc(100vh - 156px), 880px)'
const SCENE_FALLBACK_STYLE = {
  width: '100%',
  height: '100%',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
} as const

export default function HermesOfficePage() {
  const { t } = useTranslation()
  const { hasPerm, isSuperAdmin } = usePermission()
  const { mode: themeMode } = useThemeMode()
  const canEdit = isSuperAdmin || hasPerm('ai:hermes:office:write')

  const [saving, setSaving] = useState(false)
  const [selectedSeatId, setSelectedSeatId] = useState<number | null>(null)
  const [editing, setEditing] = useState(false)
  const [selectedPodKey, setSelectedPodKey] = useState('all')
  const [resetKey, setResetKey] = useState(0)
  // 左上 HUD 面板默认收起（不挡 3D 场景），点小 chip 展开、卡片内收起
  const [hudCollapsed, setHudCollapsed] = useState(true)
  const [floorModalOpen, setFloorModalOpen] = useState(false)
  const [floorModalIsEdit, setFloorModalIsEdit] = useState(false)
  const [floorForm] = Form.useForm<FloorFormValues>()

  const { draft, dirty, reset, moveSeat, rotateSeat, setLabel, bindUser, addSeat, removeSeat, payload } =
    useSeatEditor([])
  const { loading, floors, floorId, setFloorId, floor, users, departments, loadFloors, loadSeats } =
    useOfficeData(reset)

  const selected = useMemo(() => draft.find((s) => s.seat.id === selectedSeatId) ?? null, [draft, selectedSeatId])

  const setActivePodKey = useCallback(
    (key: string) => {
      setSelectedPodKey(editing ? 'all' : key)
    },
    [editing],
  )

  const metrics = useOfficeMetrics({ draft, departments, users, themeMode, editing, selectedPodKey })

  const handleSelectPod = useCallback(
    (podKey: string) => {
      setActivePodKey(podKey)
      setSelectedSeatId(null)
    },
    [setActivePodKey],
  )

  const focusRiskSeat = useCallback(
    (seat: { id: number; podKey: string }) => {
      setActivePodKey(seat.podKey)
      setSelectedSeatId(seat.id)
    },
    [setActivePodKey],
  )

  // ---- 楼层 ----

  const openFloorModal = useCallback(
    (target?: HermesOfficeFloor) => {
      floorForm.setFieldsValue({
        name: target?.name ?? '',
        width: target?.width ?? 20,
        depth: target?.depth ?? 15,
        sort: target?.sort ?? 0,
      })
      setFloorModalOpen(true)
    },
    [floorForm],
  )

  const openCreateFloorModal = useCallback(() => {
    setFloorModalIsEdit(false)
    openFloorModal()
  }, [openFloorModal])

  const openEditFloorModal = useCallback(
    (target: HermesOfficeFloor) => {
      setFloorModalIsEdit(true)
      openFloorModal(target)
    },
    [openFloorModal],
  )

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

  return (
    <div className="page-list hermes-office-page">
      <div className="hermes-office-shell">
        {floor ? (
          <Spin spinning={loading}>
            <div className="hermes-office-stage" style={{ height: SCENE_HEIGHT, minHeight: 500 }}>
              <div className="hermes-office-canvas">
                <ErrorBoundary
                  inline
                  title={t('3D 场景渲染异常')}
                  description={t('WebGL 上下文异常或显卡资源不足，请点击重试')}
                >
                  <Suspense
                    fallback={
                      <div style={SCENE_FALLBACK_STYLE}>
                        <Spin />
                      </div>
                    }
                  >
                    <Scene
                      key={resetKey}
                      floor={floor}
                      seats={draft}
                      selectedSeatId={selectedSeatId}
                      editing={editing}
                      themeMode={themeMode}
                      accents={metrics.accents}
                      pods={editing ? metrics.departmentPods : metrics.visiblePods}
                      onSelectSeat={setSelectedSeatId}
                      onMoveSeat={(id, x, y) => moveSeat(id, x, y, floor.width, floor.depth)}
                      onAddSeatAt={(x, y) => setSelectedSeatId(addSeat(floor.id, x, y))}
                    />
                  </Suspense>
                </ErrorBoundary>

                <OfficeHud
                  floor={floor}
                  floors={floors}
                  floorId={floorId}
                  seatCount={draft.length}
                  onlineCount={metrics.counts.online}
                  onlineRate={metrics.onlineRate}
                  riskCount={metrics.riskCount}
                  collapsed={hudCollapsed}
                  canEdit={canEdit}
                  editing={editing}
                  loading={loading}
                  saving={saving}
                  dirty={dirty}
                  onToggleCollapsed={setHudCollapsed}
                  onSelectFloor={(value) => {
                    setFloorId(value)
                    setSelectedSeatId(null)
                    setEditing(false)
                    setActivePodKey('all')
                  }}
                  onCreateFloor={openCreateFloorModal}
                  onEditFloor={openEditFloorModal}
                  onRemoveFloor={() => void removeFloor()}
                  onRefresh={() => floorId != null && void loadSeats(floorId)}
                  onStartEditing={() => {
                    setActivePodKey('all')
                    setEditing(true)
                  }}
                  onSaveSeats={() => void saveSeats()}
                  onLeaveEditing={leaveEditing}
                />
              </div>

              <OfficeRail
                seatCount={draft.length}
                counts={metrics.counts}
                onlineRate={metrics.onlineRate}
                riskCount={metrics.riskCount}
                editing={editing}
                dirty={dirty}
                activePodKey={metrics.activePodKey}
                departmentPods={metrics.departmentPods}
                riskAreas={metrics.riskAreas}
                visibleRiskSeats={metrics.visibleRiskSeats}
                onResetCamera={() => setResetKey((key) => key + 1)}
                onSelectPod={handleSelectPod}
                onFocusRiskSeat={focusRiskSeat}
              />
            </div>
          </Spin>
        ) : (
          <div className="hermes-office-empty" style={{ height: SCENE_HEIGHT, minHeight: 500 }}>
            <GlassEmpty text={loading ? '加载中' : '还没有办公室楼层'} />
            {canEdit && !loading && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreateFloorModal}>
                {t('新建楼层')}
              </Button>
            )}
          </div>
        )}

        {floor && (
          <OfficeMetrics
            statTiles={metrics.statTiles}
            deptStats={metrics.deptStats}
            staffedCount={metrics.staffedCount}
            activePodKey={metrics.activePodKey}
            themeMode={themeMode}
            visibleRiskSeats={metrics.visibleRiskSeats}
            onSelectPod={handleSelectPod}
            onFocusRiskSeat={focusRiskSeat}
          />
        )}
      </div>

      <SeatDetailDrawer
        seat={selected}
        editing={editing}
        users={users}
        deptName={metrics.deptName}
        userDeptId={metrics.userDeptId}
        onClose={() => setSelectedSeatId(null)}
        onRemoveSeat={removeSeat}
        onSetLabel={setLabel}
        onRotateSeat={rotateSeat}
        onBindUser={bindUser}
      />

      <FloorModal
        open={floorModalOpen}
        isEdit={floorModalIsEdit}
        saving={saving}
        form={floorForm}
        onOk={() => void submitFloor()}
        onCancel={() => setFloorModalOpen(false)}
      />
    </div>
  )
}
