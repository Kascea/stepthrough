interface Props {
  onOpen?: () => void
  setupLogs: string[]
  isSettingUp: boolean
  dockerReady: boolean | null
  setupError: string | null
}

export default function SplashScreen({ onOpen, setupLogs, isSettingUp, dockerReady, setupError }: Props) {
  const dockerBlocked = dockerReady !== true

  return (
    <div className="app splash">
      <div className="splash-title">stepthrough</div>
      <div className="splash-sub">Azure Pipelines local runner with hot refresh</div>

      <div className="docker-status">
        {dockerReady === null && (
          <>
            <span className="docker-status-dot checking" />
            <span className="docker-status-label">Checking Docker...</span>
          </>
        )}
        {dockerReady === true && (
          <>
            <span className="docker-status-dot ready" />
            <span className="docker-status-label">Docker ready</span>
          </>
        )}
        {dockerReady === false && (
          <>
            <span className="docker-status-dot error" />
            <span className="docker-status-label docker-status-error">Docker Desktop is not running</span>
          </>
        )}
      </div>

      {dockerReady === false && (
        <div className="docker-hint">
          Start Docker Desktop — this screen will update automatically.
        </div>
      )}

      {setupError && !dockerBlocked && (
        <div className="docker-hint docker-hint-error">{setupError}</div>
      )}

      <button className="open-btn" onClick={onOpen} disabled={dockerBlocked || !onOpen}>
        Open pipeline file...
      </button>

      {isSettingUp && setupLogs.length > 0 && (
        <div className="setup-logs">
          <div className="setup-logs-header">
            <span className="setup-spinner" /> Preparing environment
          </div>
          {setupLogs.map((l, i) => <div key={i} className="log-line">{l}</div>)}
        </div>
      )}
    </div>
  )
}
