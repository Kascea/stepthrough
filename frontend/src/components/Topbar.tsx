import { PipelineState } from '../types'

interface Props {
  state: PipelineState
  isSettingUp: boolean
}

export default function Topbar({ state, isSettingUp }: Props) {
  const filename = state.file.split('/').pop() ?? state.file

  return (
    <header className="topbar">
      <span className="topbar-title">stepthrough</span>
      <span className="topbar-sep">/</span>
      <span className="topbar-file">{filename}</span>
      <div className="topbar-badges">
        {isSettingUp ? (
          <span className="badge badge-running">
            <span className="topbar-spinner" /> setting up
          </span>
        ) : (
          <span className={`badge ${state.running ? 'badge-running' : 'badge-idle'}`}>
            {state.running ? '● running' : '○ idle'}
          </span>
        )}
        <span className={`badge ${state.safeMode ? 'badge-safe' : 'badge-live'}`}>
          {state.safeMode ? 'safe mode' : 'live mode'}
        </span>
      </div>
    </header>
  )
}
