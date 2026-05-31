interface Props {
  onOpen: () => void
  setupLogs: string[]
  isSettingUp: boolean
}

export default function SplashScreen({ onOpen, setupLogs, isSettingUp }: Props) {
  return (
    <div className="app splash">
      <div className="splash-title">stepthrough</div>
      <div className="splash-sub">Azure Pipelines local runner with hot-refresh</div>
      <button className="open-btn" onClick={onOpen}>
        ⊕ Open pipeline file…
      </button>
      {isSettingUp && setupLogs.length > 0 && (
        <div className="setup-logs">
          <div className="setup-logs-header">
            <span className="setup-spinner" /> Setting up container…
          </div>
          {setupLogs.map((l, i) => <div key={i} className="log-line">{l}</div>)}
        </div>
      )}
    </div>
  )
}
