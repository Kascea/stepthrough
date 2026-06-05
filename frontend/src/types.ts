export type StepStatus =
  | 'pending'
  | 'running'
  | 'passed'
  | 'failed'
  | 'skipped'
  | 'cached'
  | 'deployment'

export interface StepState {
  index: number
  stageName: string
  jobName: string
  label: string
  type: string
  status: StepStatus
  exitCode: number
  durationMs: number
  isDeploymentJob?: boolean
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

// Wraps every pipeline event with the file it originated from.
export interface PipelineFileEvent<T = unknown> {
  file: string
  data: T
}

// One saved run entry from the session file.
export interface SavedRun {
  steps: StepState[]
  logs: Record<string, string[]> // step index string → lines
  ranAt: string
}

// Returned by PipelineService.GetSession on startup.
export interface SessionData {
  tabOrder: string[]
  activeFile: string
  runs: Record<string, SavedRun>
}

// Explicit lifecycle state for a pipeline tab — mirrors the backend TabStatus enum.
// Using a discriminated union makes impossible combinations (e.g. missing + pipeline)
// unrepresentable and lets TypeScript narrow access to status-specific data.
export type TabStatus =
  | { kind: 'empty' }
  | { kind: 'loaded';      pipeline: PipelineState }
  | { kind: 'running';     pipeline: PipelineState }
  | { kind: 'missing' }
  | { kind: 'error';       message: string }
  | { kind: 'setup-error'; message: string }

// Frontend state for a single pipeline tab.
export interface TabState {
  file: string
  status: TabStatus
  selectedStep: number | null
  logs: Record<number, string[]>
  setupLogs: string[]
  isSettingUp: boolean
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

export function basename(filePath: string): string {
  return filePath.split('/').pop() ?? filePath
}

// Derive a display status for a tab (for the tab strip dot).
export function tabStatus(tab: TabState): StepStatus {
  if (tab.status.kind !== 'loaded' && tab.status.kind !== 'running') return 'pending'
  const { steps } = tab.status.pipeline
  if (!steps.length) return 'pending'
  return stageStatus(steps)
}
