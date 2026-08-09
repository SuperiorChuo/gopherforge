import { useTranslation } from 'react-i18next'
import { Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import SpaceResult from '@/components/SpaceResult'

export default function Page404() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <SpaceResult
      code="404"
      title={t('页面走丢了')}
      description={t('您访问的页面不存在或已被移除，请检查地址是否正确。')}
      detail={<span className="cell-mono">{window.location.pathname}</span>}
      actions={
        <>
          <Button onClick={() => navigate(-1)}>{t('返回上页')}</Button>
          <Button type="primary" onClick={() => navigate('/dashboard')}>{t('回到首页')}</Button>
        </>
      }
    />
  )
}
