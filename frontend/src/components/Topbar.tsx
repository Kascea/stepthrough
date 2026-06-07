import { TabState, tabLabel, tabStatus } from '../types'

interface Props {
  tabs: TabState[]
  activeFile: string | null
  onAdd: () => void
  onClose: (file: string) => void
  onSwitch: (file: string) => void
  onRun: () => void
  onCancel: () => void
  running?: boolean
  missing?: boolean
}

function StatusDot({ tab }: { tab: TabState }) {
  if (tab.status.kind === 'missing') return <span className="tab-dot tab-dot-missing" title="File not found" />
  if (tab.status.kind === 'error') return <span className="tab-dot tab-dot-error" title="Parse error" />
  const s = tabStatus(tab)
  return <span className={`tab-dot tab-dot-${s}`} />
}

export default function Topbar({ tabs, activeFile, onAdd, onClose, onSwitch, onRun, onCancel, running, missing }: Props) {
  return (
    <header className="topbar">
      <span className="topbar-brand">stepthrough</span>

      <div className="tab-strip">
        {tabs.map(tab => {
          const active = tab.file === activeFile
          return (
            <button
              key={tab.file}
              className={`tab ${active ? 'tab-active' : ''}`}
              onClick={() => onSwitch(tab.file)}
              title={tab.file}
            >
              <StatusDot tab={tab} />
              <span className="tab-label">{tabLabel(tab.file, (tab.status.kind === 'loaded' || tab.status.kind === 'running') ? tab.status.pipeline.repoRoot : undefined)}</span>
              <span
                className="tab-close"
                role="button"
                onClick={e => { e.stopPropagation(); onClose(tab.file) }}
                title="Close"
              >
                ×
              </span>
            </button>
          )
        })}
        <button className="tab-add" onClick={onAdd} title="Open pipeline file">
          +
        </button>
      </div>

      <div className="topbar-actions">
        {running ? (
          <button className="run-btn run-btn-cancel" onClick={onCancel}>
            <span className="run-spinner" /> Cancel
          </button>
        ) : (
          <button className="run-btn" onClick={onRun} disabled={missing}>
            ▶ Run
          </button>
        )}
      </div>
    </header>
  )
}
