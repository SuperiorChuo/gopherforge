import { Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import SpaceResult from '@/components/SpaceResult'

export default function Page404() {
  const navigate = useNavigate()
  return (
    <SpaceResult
      code="404"
      title="页面走丢了"
      description="您访问的页面不存在或已被移除，请检查地址是否正确。"
      actions={
        <>
          <Button onClick={() => navigate(-1)}>返回上页</Button>
          <Button type="primary" onClick={() => navigate('/dashboard')}>回到首页</Button>
        </>
      }
    />
  )
}
