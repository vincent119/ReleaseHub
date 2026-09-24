import { Alert, Button, Space } from 'antd'
import { Component, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { isChunkLoadError, reloadOnceForBuild } from './routeRecovery'

type RouteFailure = 'chunk' | 'render'

interface Props {
  children: ReactNode
  buildIdentity?: string
  storage?: Pick<Storage, 'getItem' | 'setItem'>
  reload?: () => void
}

interface State {
  failure: RouteFailure | null
}

export class RouteErrorBoundary extends Component<Props, State> {
  state: State = { failure: null }

  static getDerivedStateFromError(error: unknown): State {
    return { failure: isChunkLoadError(error) ? 'chunk' : 'render' }
  }

  componentDidCatch(error: unknown) {
    if (!isChunkLoadError(error)) return

    // import.meta.url 指向目前執行中的 hashed bundle，舊頁面與新版的 guard 不共用。
    reloadOnceForBuild(
      this.props.buildIdentity ?? import.meta.url,
      this.props.storage ?? window.sessionStorage,
      this.props.reload ?? (() => window.location.reload()),
    )
  }

  render() {
    if (this.state.failure) {
      return <RouteFailureMessage failure={this.state.failure} />
    }

    return this.props.children
  }
}

function RouteFailureMessage({ failure }: { failure: RouteFailure }) {
  const { t } = useTranslation()
  const isChunkFailure = failure === 'chunk'

  return (
    <Space orientation="vertical" size="middle" role="alert">
      <Alert
        type={isChunkFailure ? 'warning' : 'error'}
        showIcon
        title={t(
          isChunkFailure ? 'app.routeError.updated' : 'app.routeError.title',
        )}
        description={t(
          isChunkFailure
            ? 'app.routeError.updatedDescription'
            : 'app.routeError.description',
        )}
      />
      <Button onClick={() => window.location.reload()}>
        {t('app.routeError.reload')}
      </Button>
    </Space>
  )
}
