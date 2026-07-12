import { useEffect, useState } from 'react'
import { Card, Row, Col, Statistic, Typography } from 'antd'
import { UserOutlined, TeamOutlined, SafetyOutlined, MenuOutlined } from '@ant-design/icons'
import { useAppSelector } from '@/hooks/store'
import { getUserList } from '@/api/system/user'
import { getRoleList } from '@/api/system/role'
import { getPermissionList } from '@/api/system/permission'
import { getMenuList } from '@/api/system/menu'

const { Title, Text } = Typography

export default function DashboardPage() {
  const { userInfo } = useAppSelector((s) => s.auth)
  const [stats, setStats] = useState({ users: 0, roles: 0, permissions: 0, menus: 0 })
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchStats = async () => {
      try {
        const [usersRes, rolesRes, permsRes, menusRes] = await Promise.all([
          getUserList({ page: 1, page_size: 1 }),
          getRoleList({ page: 1, page_size: 1 }),
          getPermissionList({ page: 1, page_size: 1 }),
          getMenuList({ page: 1, page_size: 1 }),
        ])
        setStats({
          users: usersRes.total,
          roles: rolesRes.total,
          permissions: permsRes.total,
          menus: menusRes.total,
        })
      } catch {
        // ignore
      } finally {
        setLoading(false)
      }
    }
    fetchStats()
  }, [])

  return (
    <div>
      <Card style={{ marginBottom: 24 }}>
        <Title level={4} style={{ marginBottom: 4 }}>
          欢迎回来，{userInfo?.nickname || userInfo?.username}
        </Title>
        <Text type="secondary">今天是个好日子，祝您工作顺利！</Text>
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="总用户数"
              value={loading ? '加载中' : stats.users}
              prefix={<UserOutlined />}
              valueStyle={{ color: '#1677ff' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="角色数量"
              value={loading ? '加载中' : stats.roles}
              prefix={<TeamOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="权限数量"
              value={loading ? '加载中' : stats.permissions}
              prefix={<SafetyOutlined />}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="菜单数量"
              value={loading ? '加载中' : stats.menus}
              prefix={<MenuOutlined />}
              valueStyle={{ color: '#f5222d' }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
