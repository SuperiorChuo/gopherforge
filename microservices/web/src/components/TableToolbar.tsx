import type { ReactNode } from 'react'
import {
  UserOutlined, TeamOutlined, SafetyOutlined, MenuOutlined, ApartmentOutlined,
  DatabaseOutlined, FileOutlined, NotificationOutlined, LoginOutlined,
  FileTextOutlined, AuditOutlined, WifiOutlined, ScheduleOutlined, BarsOutlined,
  ClusterOutlined,
} from '@ant-design/icons'

interface ToolbarPreset {
  icon: ReactNode
  gradient: string
  glow: string
  description: string
}

// 标题 → 徽章预设。集中在这里,列表页无需逐个传参;
// 新页面标题不在表里时退回"渐变竖线"旧样式,不会坏。
const PRESETS: Record<string, ToolbarPreset> = {
  用户列表: {
    icon: <UserOutlined />,
    gradient: 'linear-gradient(135deg, #818cf8, #4f46e5)',
    glow: 'rgba(79, 70, 229, 0.4)',
    description: '账号、角色指派与启停管理',
  },
  角色列表: {
    icon: <TeamOutlined />,
    gradient: 'linear-gradient(135deg, #34d399, #059669)',
    glow: 'rgba(5, 150, 105, 0.4)',
    description: '角色定义与权限组合',
  },
  权限列表: {
    icon: <SafetyOutlined />,
    gradient: 'linear-gradient(135deg, #fbbf24, #d97706)',
    glow: 'rgba(217, 119, 6, 0.4)',
    description: '细粒度权限点与接口访问控制',
  },
  菜单结构: {
    icon: <MenuOutlined />,
    gradient: 'linear-gradient(135deg, #f472b6, #db2777)',
    glow: 'rgba(219, 39, 119, 0.4)',
    description: '导航菜单树与路由可见性',
  },
  部门架构: {
    icon: <ApartmentOutlined />,
    gradient: 'linear-gradient(135deg, #38bdf8, #0284c7)',
    glow: 'rgba(2, 132, 199, 0.4)',
    description: '组织层级与数据权限范围',
  },
  字典类型: {
    icon: <DatabaseOutlined />,
    gradient: 'linear-gradient(135deg, #a78bfa, #7c3aed)',
    glow: 'rgba(124, 58, 237, 0.4)',
    description: '枚举字典的类型定义',
  },
  字典项: {
    icon: <BarsOutlined />,
    gradient: 'linear-gradient(135deg, #a78bfa, #7c3aed)',
    glow: 'rgba(124, 58, 237, 0.4)',
    description: '各类型下的键值条目',
  },
  文件列表: {
    icon: <FileOutlined />,
    gradient: 'linear-gradient(135deg, #22d3ee, #0891b2)',
    glow: 'rgba(8, 145, 178, 0.4)',
    description: '上传文件的存储与预览,支持拖放上传',
  },
  通知公告: {
    icon: <NotificationOutlined />,
    gradient: 'linear-gradient(135deg, #fb923c, #ea580c)',
    glow: 'rgba(234, 88, 12, 0.4)',
    description: '面向全员的系统通知与公告',
  },
  登录日志: {
    icon: <LoginOutlined />,
    gradient: 'linear-gradient(135deg, #818cf8, #4f46e5)',
    glow: 'rgba(79, 70, 229, 0.4)',
    description: '登录行为审计与异常追踪',
  },
  操作日志: {
    icon: <FileTextOutlined />,
    gradient: 'linear-gradient(135deg, #2dd4bf, #0d9488)',
    glow: 'rgba(13, 148, 136, 0.4)',
    description: '接口调用记录与请求诊断',
  },
  审计日志: {
    icon: <AuditOutlined />,
    gradient: 'linear-gradient(135deg, #f87171, #dc2626)',
    glow: 'rgba(220, 38, 38, 0.4)',
    description: '敏感操作留痕,满足合规审计',
  },
  在线用户: {
    icon: <WifiOutlined />,
    gradient: 'linear-gradient(135deg, #38bdf8, #0284c7)',
    glow: 'rgba(2, 132, 199, 0.4)',
    description: '当前活跃会话,可强制下线',
  },
  定时任务: {
    icon: <ScheduleOutlined />,
    gradient: 'linear-gradient(135deg, #34d399, #059669)',
    glow: 'rgba(5, 150, 105, 0.4)',
    description: 'Cron 调度任务与执行日志',
  },
  租户管理: {
    icon: <ClusterOutlined />,
    gradient: 'linear-gradient(135deg, #fb7185, #e11d48)',
    glow: 'rgba(225, 29, 72, 0.4)',
    description: '多租户隔离、套餐与用户配额',
  },
}

interface TableToolbarProps {
  title: string
  total?: number
  extra?: ReactNode
  /** 覆盖预设徽章图标；标题不在预设表且不传时退回渐变竖线 */
  icon?: ReactNode
  gradient?: string
  glow?: string
  description?: string
}

export default function TableToolbar({
  title, total, extra, icon, gradient, glow, description,
}: TableToolbarProps) {
  const preset = PRESETS[title]
  const badgeIcon = icon ?? preset?.icon
  const badgeGradient = gradient ?? preset?.gradient
  const badgeGlow = glow ?? preset?.glow
  const desc = description ?? preset?.description

  return (
    <div className="table-toolbar">
      <div className={`table-toolbar-title ${badgeIcon ? 'table-toolbar-title-iconed' : ''}`}>
        {badgeIcon && (
          <span
            className="table-toolbar-badge"
            style={{ background: badgeGradient, '--badge-glow': badgeGlow } as React.CSSProperties}
          >
            {badgeIcon}
          </span>
        )}
        <span className="table-toolbar-text">
          <span className="table-toolbar-heading">
            {title}
            {typeof total === 'number' && <span className="table-count">{total}</span>}
          </span>
          {desc && <span className="table-toolbar-desc">{desc}</span>}
        </span>
      </div>
      {extra && <div className="table-toolbar-extra">{extra}</div>}
    </div>
  )
}
