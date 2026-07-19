import { Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import SpaceResult from '@/components/SpaceResult'

export default function Page403() {
  const navigate = useNavigate()
  return (
    <SpaceResult
      code="403"
      title="禁止访问"
      description="您没有权限访问此页面。如需开通权限，请联系系统管理员。"
      actions={
        <>
          <Button onClick={() => navigate(-1)}>返回上页</Button>
          <Button type="primary" onClick={() => navigate('/dashboard')}>回到首页</Button>
        </>
      }
    />
  )
}
