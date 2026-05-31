export type StepStatus = 'pending' | 'running' | 'passed' | 'failed' | 'skipped' | 'cached'

export interface StepState {
  index: number
  stageName: string
  jobName: string
  label: string
  type: string
  status: StepStatus
  exitCode: number
  durationMs: number
}

export interface PipelineState {
  file: string
  valid: boolean
  error: string
  steps: StepState[]
  variables: Record<string, string>
  safeMode: boolean
  running: boolean
}

export interface LogLine {
  stepIndex: number
  line: string
}

export const STATUS_ICON: Record<StepStatus, string> = {
  pending: '○',
  running: '◌',
  passed: '✔',
  failed: '✖',
  skipped: '⊝',
  cached: '⚡',
}

export function groupSteps(steps: StepState[]) {
  const stages: { name: string; jobs: { name: string; steps: StepState[] }[] }[] = []
  for (const step of steps) {
    let stage = stages.find(s => s.name === step.stageName)
    if (!stage) {
      stage = { name: step.stageName, jobs: [] }
      stages.push(stage)
    }
    let job = stage.jobs.find(j => j.name === step.jobName)
    if (!job) {
      job = { name: step.jobName, steps: [] }
      stage.jobs.push(job)
    }
    job.steps.push(step)
  }
  return stages
}

export function stageStatus(steps: StepState[]): StepStatus {
  if (steps.some(s => s.status === 'failed')) return 'failed'
  if (steps.some(s => s.status === 'running')) return 'running'
  if (steps.every(s => s.status === 'passed' || s.status === 'cached' || s.status === 'skipped')) return 'passed'
  return 'pending'
}

export function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}
