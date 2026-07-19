import { App } from 'antd'

type Feedback = ReturnType<typeof App.useApp>

// Live bindings assigned by <FeedbackBridge /> mounted inside <App> in App.tsx.
// Import { message } from here (instead of antd static) so toasts consume the
// ConfigProvider theme and antd v6 doesn't warn about static functions.
let message: Feedback['message']
let notification: Feedback['notification']
let modal: Feedback['modal']

export default function FeedbackBridge() {
  const app = App.useApp()
  message = app.message
  notification = app.notification
  modal = app.modal
  return null
}

export { message, notification, modal }
