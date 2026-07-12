import { Button, Result } from 'antd'
import { useNavigate } from 'react-router-dom'

export default function Page500() {
  const navigate = useNavigate()
  return (
    <Result
      status="500"
      title="500"
      subTitle="抱歉，服务器出现错误"
      extra={
        <Button type="primary" onClick={() => navigate(-1)}>
          返回
        </Button>
      }
    />
  )
}
