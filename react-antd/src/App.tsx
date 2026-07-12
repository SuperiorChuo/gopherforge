import { Provider } from 'react-redux'
import { BrowserRouter, useRoutes } from 'react-router-dom'
import { ConfigProvider, App as AntApp } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { store } from '@/store'
import routes from '@/router'
import 'dayjs/locale/zh-cn'

function AppRoutes() {
  return useRoutes(routes)
}

export default function App() {
  return (
    <Provider store={store}>
      <ConfigProvider locale={zhCN} theme={{ token: { colorPrimary: '#1677ff' } }}>
        <AntApp>
          <BrowserRouter>
            <AppRoutes />
          </BrowserRouter>
        </AntApp>
      </ConfigProvider>
    </Provider>
  )
}
