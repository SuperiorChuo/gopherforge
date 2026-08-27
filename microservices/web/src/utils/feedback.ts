import { App } from 'antd'
import { useEffect } from 'react'

type Feedback = ReturnType<typeof App.useApp>

// Live bindings assigned by <FeedbackBridge /> mounted inside <App> in App.tsx.
// Import { message } from here (instead of antd static) so toasts consume the
// ConfigProvider theme and antd v6 doesn't warn about static functions.
let message: Feedback['message']
let notification: Feedback['notification']
let modal: Feedback['modal']

export default function FeedbackBridge() {
  const app = App.useApp()
  // 赋值放 effect（StrictMode 下 render 期写外部变量可能残留弃用值）；
  // bridge 先于消费组件 mount，任何事件/子 effect 触发时已就绪。
  useEffect(() => {
    message = app.message
    notification = app.notification
    modal = app.modal
  })
  return null
}

export { message, notification, modal }
