import { Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import SpaceResult from '@/components/SpaceResult'

export default function Page500() {
  const navigate = useNavigate()
  return (
    <SpaceResult
      code="500"
      title="服务出了点问题"
      description="服务器开小差了，请稍后重试。如果问题持续出现，请联系管理员。"
      actions={
        <>
          <Button onClick={() => window.location.reload()}>刷新重试</Button>
          <Button type="primary" onClick={() => navigate('/dashboard')}>回到首页</Button>
        </>
      }
    />
  )
}
