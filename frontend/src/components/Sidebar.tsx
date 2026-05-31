import { useState, useEffect } from 'react'
import { StepState, groupSteps, stageStatus, formatDuration } from '../types'
import StatusIcon from './StatusIcon'

interface Props {
  steps: StepState[]
  selectedStep: number | null
  onSelectStep: (index: number) => void
  isSettingUp: boolean
}

export default function Sidebar({ steps, selectedStep, onSelectStep, isSettingUp }: Props) {
  const stages = groupSteps(steps)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  // Auto-expand any stage that has an active (running/failed) step
  useEffect(() => {
    const activeStageNames = new Set<string>()
    for (const step of steps) {
      if (step.status === 'running' || step.status === 'failed') {
        activeStageNames.add(step.stageName)
      }
    }
    if (activeStageNames.size === 0) return
    setCollapsed(prev => {
      const next = new Set(prev)
      for (const name of activeStageNames) next.delete(name)
      return next
    })
  }, [steps])

  const toggle = (name: string) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <aside className="sidebar">
      {isSettingUp && (
        <div className="sidebar-setup-banner">
          <span className="sidebar-setup-spinner" />
          Setting up container…
        </div>
      )}

      {stages.map(stage => {
        const allSteps = stage.jobs.flatMap(j => j.steps)
        const ss = stageStatus(allSteps)
        const isCollapsed = collapsed.has(stage.name)

        return (
          <div key={stage.name} className="stage-block">
            <div
              className={`stage-header s-${ss}`}
              onClick={() => toggle(stage.name)}
            >
              <span className={`stage-chevron${isCollapsed ? ' collapsed' : ''}`}>›</span>
              <StatusIcon status={ss} size={14} />
              <span className="stage-name">{stage.name}</span>
            </div>

            {!isCollapsed && stage.jobs.map(job => {
              const js = stageStatus(job.steps)
              return (
                <div key={job.name} className="job-block">
                  <div className={`job-header s-${js}`}>
                    <StatusIcon status={js} size={13} />
                    <span>{job.name}</span>
                  </div>
                  {job.steps.map(step => (
                    <div
                      key={step.index}
                      className={`step-row s-${step.status}${selectedStep === step.index ? ' selected' : ''}`}
                      onClick={e => { e.stopPropagation(); onSelectStep(step.index) }}
                    >
                      <StatusIcon status={step.status} size={13} />
                      <span className="step-label">{step.label}</span>
                      {step.durationMs > 0 && (
                        <span className="step-dur">{formatDuration(step.durationMs)}</span>
                      )}
                    </div>
                  ))}
                </div>
              )
            })}
          </div>
        )
      })}
    </aside>
  )
}
