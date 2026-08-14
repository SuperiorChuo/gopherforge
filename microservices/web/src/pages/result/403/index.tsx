import { useTranslation } from 'react-i18next'
import { Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import SpaceResult from '@/components/common/SpaceResult'

export default function Page403() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <SpaceResult
      code="403"
      title={t('禁止访问')}
      description={t('您没有权限访问此页面。如需开通权限，请联系系统管理员。')}
      actions={
        <>
          <Button onClick={() => navigate(-1)}>{t('返回上页')}</Button>
          <Button type="primary" onClick={() => navigate('/dashboard')}>{t('回到首页')}</Button>
        </>
      }
    />
  )
}
