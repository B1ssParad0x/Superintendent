import React from 'react'

export default class AppErrorBoundary extends React.Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, errorInfo) {
    // Keep details in console for local debugging.
    // eslint-disable-next-line no-console
    console.error('AppErrorBoundary caught:', error, errorInfo)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="mx-auto max-w-3xl p-6">
          <div className="panel rounded-xl border border-red-500/60 p-4">
            <h2 className="font-display text-lg text-red-300">UI runtime error</h2>
            <p className="mt-2 text-sm text-zinc-300">
              A component failed to render. Open browser devtools console for details.
            </p>
            <pre className="mt-3 overflow-auto rounded bg-black/60 p-3 text-xs text-zinc-400">
              {String(this.state.error?.message || this.state.error || 'Unknown error')}
            </pre>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
