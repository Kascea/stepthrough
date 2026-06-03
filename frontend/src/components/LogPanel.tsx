import { useRef, useEffect } from 'react'
import { StepState, formatDuration } from '../types'
import StatusIcon from './StatusIcon'
import VarPanel from './VarPanel'

interface Props {
  selectedStep: number | null
  steps: StepState[]
  logs: Record<number, string[]>
  setupLogs: string[]
  variables: Record<string, string>
  isSettingUp: boolean
}

export default function LogPanel({ selectedStep, steps, logs, setupLogs, variables, isSettingUp }: Props) {
  const logEndRef = useRef<HTMLDivElement>(null)
  const selectedLogs = selectedStep !== null ? (logs[selectedStep] ?? []) : []
  const step = selectedStep !== null ? steps[selectedStep] : null
  const logIconStatus = step?.status === 'running' ? 'pending' : step?.status

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs, setupLogs, selectedStep])

  const showSetup = isSettingUp && selectedStep === null

  return (
    <main className="log-panel">
      {isSettingUp ? (
        <div className="setup-global-banner">
          <span className="setup-spinner" />
          <span>Setting up container...</span>
        </div>
      ) : null}

      {step && !showSetup ? (
        <div className="log-header">
          <span className={`log-badge s-${step.status}`}>
            <StatusIcon status={logIconStatus ?? 'pending'} size={12} />
            {step.status}
          </span>
          <span className="log-step-name">{step.label}</span>
          {step.durationMs > 0 && (
            <span className="log-dur">{formatDuration(step.durationMs)}</span>
          )}
        </div>
      ) : null}

      <div className="log-body">
        {showSetup ? (
          setupLogs.length === 0 ? (
            <div className="log-empty">Waiting for environment output...</div>
          ) : (
            <>
              <div className="setup-stream-tag">Container setup logs</div>
              {setupLogs.map((line, i) => (
                <div key={i} className="log-line">{line}</div>
              ))}
            </>
          )
        ) : selectedLogs.length === 0 ? (
          <div className="log-empty">
            {selectedStep === null ? 'Select a step to view logs.' : 'No output yet.'}
          </div>
        ) : (
          selectedLogs.map((line, i) => (
            <div key={i} className="log-line">{line}</div>
          ))
        )}
        <div ref={logEndRef} />
      </div>

      <VarPanel variables={variables} />
    </main>
  )
}
