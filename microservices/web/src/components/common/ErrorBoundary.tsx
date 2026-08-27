import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Alert, Button, Card } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import SpaceResult from '@/components/common/SpaceResult'
import i18n from '@/i18n/init'

interface Props {
  children: ReactNode
  fallback?: ReactNode | ((error: Error | null, reset: () => void) => ReactNode)
  inline?: boolean
  title?: string
  description?: string
  onError?: (error: Error, info: ErrorInfo) => void
}

interface State {
  hasError: boolean
  error: Error | null
}

// 页面/组件抛错时兜底，避免整页白屏
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.props.onError?.(error, info)
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null })
  }

  render() {
    if (this.state.hasError) {
      if (typeof this.props.fallback === 'function') {
        return this.props.fallback(this.state.error, this.handleReset)
      }
      if (this.props.fallback) {
        return this.props.fallback
      }
      if (this.props.inline) {
        return (
          <Card className="error-boundary-inline-card" bordered={false} style={{ margin: '8px 0' }}>
            <Alert
              type="error"
              showIcon
              message={this.props.title || i18n.t('模块渲染异常')}
              description={this.props.description || this.state.error?.message || i18n.t('该区域加载发生错误')}
              action={
                <Button size="small" icon={<ReloadOutlined />} onClick={this.handleReset}>
                  {i18n.t('重试')}
                </Button>
              }
            />
          </Card>
        )
      }
      return (
        <SpaceResult
          code="Oops"
          title={this.props.title || i18n.t('页面出错了')}
          description={
            this.props.description ||
            i18n.t('页面渲染发生异常，请刷新重试。如果反复出现请联系管理员。')
          }
          actions={
            <Button
              type="primary"
              onClick={() => {
                this.handleReset()
                window.location.reload()
              }}
            >
              {i18n.t('刷新页面')}
            </Button>
          }
        />
      )
    }
    return this.props.children
  }
}
