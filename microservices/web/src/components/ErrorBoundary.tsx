import { Component, type ReactNode } from 'react'
import { Button } from 'antd'
import SpaceResult from '@/components/SpaceResult'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
}

// 页面组件抛错时兜底，避免整页白屏
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  handleReset = () => {
    this.setState({ hasError: false })
  }

  render() {
    if (this.state.hasError) {
      return (
        <SpaceResult
          code="Oops"
          title="页面出错了"
          description="页面渲染发生异常，请刷新重试。如果反复出现请联系管理员。"
          actions={
            <Button type="primary" onClick={() => { this.handleReset(); window.location.reload() }}>
              刷新页面
            </Button>
          }
        />
      )
    }
    return this.props.children
  }
}
