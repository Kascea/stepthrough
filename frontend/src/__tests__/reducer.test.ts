import { describe, it, expect } from 'vitest'
import { TabState, TabStatus, SavedRun, PipelineState, PipelineFileEvent, StepStatus } from '../types'

// Inline the reducer types and logic so tests have no dependency on
// @wailsio/runtime (which is only available in a Wails webview).

interface AppState {
  tabs: TabState[]
  activeFile: string | null
  dockerReady: boolean | null
}

const initialState: AppState = {
  tabs: [],
  activeFile: null,
  dockerReady: null,
}

function emptyTab(file: string): TabState {
  return {
    file,
    status: { kind: 'empty' },
    selectedStep: null,
    logs: {},
    setupLogs: [],
    isSettingUp: false,
  }
}

function tabFromSavedRun(file: string, run: SavedRun): TabState {
  const pipeline: PipelineState = {
    file,
    valid: true,
    error: '',
    steps: run.steps ?? [],
    variables: {},
    safeMode: true,
    running: false,
  }
  const logs: Record<number, string[]> = {}
  for (const [k, v] of Object.entries(run.logs ?? {})) {
    logs[Number(k)] = v
  }
  return {
    file,
    status: { kind: 'loaded', pipeline },
    selectedStep: null,
    logs,
    setupLogs: [],
    isSettingUp: false,
  }
}

function pipelineOf(status: TabStatus): PipelineState | null {
  return status.kind === 'loaded' || status.kind === 'running' ? status.pipeline : null
}

type SessionRestoredPayload = {
  tabOrder: string[] | null | undefined
  activeFile: string
  runs: Record<string, SavedRun>
}

function applySessionRestored(state: AppState, payload: SessionRestoredPayload): AppState {
  const { tabOrder, activeFile, runs } = payload
  if (!tabOrder?.length) return state
  const tabs = tabOrder.map(file =>
    runs[file] ? tabFromSavedRun(file, runs[file]) : emptyTab(file)
  )
  return {
    ...state,
    tabs,
    activeFile: activeFile || tabOrder[0] || null,
  }
}

// ── Regression: null/empty tabOrder must never crash ─────────────────────────

describe('session:restored', () => {
  it('returns state unchanged when tabOrder is null (Go nil slice → JSON null)', () => {
    const result = applySessionRestored(initialState, {
      tabOrder: null,
      activeFile: '',
      runs: {},
    })
    expect(result).toBe(initialState)
  })

  it('returns state unchanged when tabOrder is undefined', () => {
    const result = applySessionRestored(initialState, {
      tabOrder: undefined,
      activeFile: '',
      runs: {},
    })
    expect(result).toBe(initialState)
  })

  it('returns state unchanged when tabOrder is empty', () => {
    const result = applySessionRestored(initialState, {
      tabOrder: [],
      activeFile: '',
      runs: {},
    })
    expect(result).toBe(initialState)
  })

  it('restores tabs with saved run data using loaded status', () => {
    const savedRun: SavedRun = {
      steps: [
        {
          index: 0, stageName: 'Stage1', jobName: 'Job1', label: 'npm install',
          type: 'script', status: 'passed', exitCode: 0, durationMs: 1200,
        },
      ],
      logs: { '0': ['step output'] },
      ranAt: '2026-06-05T10:00:00Z',
    }

    const result = applySessionRestored(initialState, {
      tabOrder: ['/path/to/pipeline.yml'],
      activeFile: '/path/to/pipeline.yml',
      runs: { '/path/to/pipeline.yml': savedRun },
    })

    expect(result.tabs).toHaveLength(1)
    expect(result.activeFile).toBe('/path/to/pipeline.yml')
    const tab = result.tabs[0]
    expect(tab.status.kind).toBe('loaded')
    const pipeline = pipelineOf(tab.status)
    expect(pipeline?.steps).toHaveLength(1)
    expect(pipeline?.steps[0].status).toBe('passed')
    expect(tab.logs[0]).toEqual(['step output'])
  })

  it('uses first tab as active when activeFile is empty', () => {
    const result = applySessionRestored(initialState, {
      tabOrder: ['/a.yml', '/b.yml'],
      activeFile: '',
      runs: {},
    })
    expect(result.activeFile).toBe('/a.yml')
  })

  it('creates empty tabs for files with no saved run', () => {
    const result = applySessionRestored(initialState, {
      tabOrder: ['/unseen.yml'],
      activeFile: '/unseen.yml',
      runs: {},
    })
    expect(result.tabs[0].status.kind).toBe('empty')
    expect(result.tabs[0].logs).toEqual({})
  })
})

// ── step:done applies full PipelineState from event payload (no RPC round-trip) ─

describe('step:done', () => {
  function applyStepDone(state: AppState, event: PipelineFileEvent<PipelineState>): AppState {
    const { file, data: incoming } = event
    return {
      ...state,
      tabs: state.tabs.map(t => {
        if (t.file !== file) return t
        const current = pipelineOf(t.status)
        // Preserve any step already marked 'running' in local state when the
        // incoming snapshot shows it as 'pending' — the snapshot is taken before
        // the next step:started fires on the backend, so it can arrive after
        // flushSync has already rendered the next step as 'running'.
        const steps = incoming.steps.map((s, i) =>
          current?.steps[i]?.status === 'running' && s.status === 'pending'
            ? current.steps[i]
            : s
        )
        return { ...t, status: { kind: 'running', pipeline: { ...incoming, steps } } }
      }),
    }
  }

  function makeStep(index: number, status: StepStatus, durationMs = 0) {
    return { index, stageName: 'S', jobName: 'J', label: `step-${index}`, type: 'script', status, exitCode: 0, durationMs } as const
  }

  it('transitions the completed step from running to passed', () => {
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{
        ...emptyTab(file),
        status: { kind: 'running', pipeline: {
          file, valid: true, error: '', running: true, safeMode: false, variables: {},
          steps: [makeStep(0, 'running')],
        }},
      }],
      activeFile: file,
    }
    const result = applyStepDone(initial, {
      file,
      data: { file, valid: true, error: '', running: true, safeMode: false, variables: {},
        steps: [makeStep(0, 'passed', 420)] },
    })
    expect(pipelineOf(result.tabs[0].status)?.steps[0].status).toBe('passed')
    expect(pipelineOf(result.tabs[0].status)?.steps[0].durationMs).toBe(420)
  })

  it('does not downgrade a running step to pending from a stale snapshot', () => {
    // Regression: step:done snapshot is taken before step:started fires for the
    // next step. If it arrives after flushSync already rendered step 1 as
    // 'running', we must not overwrite it with 'pending'.
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{
        ...emptyTab(file),
        status: { kind: 'running', pipeline: {
          file, valid: true, error: '', running: true, safeMode: false, variables: {},
          // step 0 passed, step 1 already set to running by step:started + flushSync
          steps: [makeStep(0, 'passed'), makeStep(1, 'running')],
        }},
      }],
      activeFile: file,
    }
    // step:done snapshot: captured before step 1 started — shows step 1 as pending
    const result = applyStepDone(initial, {
      file,
      data: { file, valid: true, error: '', running: true, safeMode: false, variables: {},
        steps: [makeStep(0, 'passed', 100), makeStep(1, 'pending')] },
    })
    const steps = pipelineOf(result.tabs[0].status)?.steps
    expect(steps?.[0].status).toBe('passed')
    expect(steps?.[1].status).toBe('running') // must NOT be overwritten to 'pending'
  })

  it('does update a pending step to passed when the snapshot reflects completion', () => {
    // Two steps, step 0 completes, step 1 was never started (still pending in both
    // local state and snapshot) — snapshot should win and step 1 stays pending.
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{
        ...emptyTab(file),
        status: { kind: 'running', pipeline: {
          file, valid: true, error: '', running: true, safeMode: false, variables: {},
          steps: [makeStep(0, 'running'), makeStep(1, 'pending')],
        }},
      }],
      activeFile: file,
    }
    const result = applyStepDone(initial, {
      file,
      data: { file, valid: true, error: '', running: true, safeMode: false, variables: {},
        steps: [makeStep(0, 'passed', 200), makeStep(1, 'pending')] },
    })
    const steps = pipelineOf(result.tabs[0].status)?.steps
    expect(steps?.[0].status).toBe('passed')
    expect(steps?.[1].status).toBe('pending')
  })

  it('is a no-op for tabs with a different file', () => {
    const state: AppState = { ...initialState, tabs: [emptyTab('/other.yml')], activeFile: '/other.yml' }
    const result = applyStepDone(state, {
      file: '/unknown.yml',
      data: { file: '/unknown.yml', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: false },
    })
    expect(result.tabs[0].status.kind).toBe('empty')
  })
})

// ── tab:added deduplication ───────────────────────────────────────────────────

describe('tab:added', () => {
  function applyTabAdded(state: AppState, file: string): AppState {
    if (state.tabs.some(t => t.file === file)) {
      return { ...state, activeFile: file }
    }
    return {
      ...state,
      tabs: [...state.tabs, emptyTab(file)],
      activeFile: file,
    }
  }

  it('adds a new tab', () => {
    const result = applyTabAdded(initialState, '/new.yml')
    expect(result.tabs).toHaveLength(1)
    expect(result.activeFile).toBe('/new.yml')
  })

  it('does not duplicate an existing tab', () => {
    const withTab = { ...initialState, tabs: [emptyTab('/existing.yml')], activeFile: null }
    const result = applyTabAdded(withTab, '/existing.yml')
    expect(result.tabs).toHaveLength(1)
    expect(result.activeFile).toBe('/existing.yml')
  })
})

// ── TabStatus discriminated union — impossible states are unrepresentable ─────

describe('TabStatus', () => {
  it('empty tab has no pipeline', () => {
    const tab = emptyTab('/a.yml')
    expect(tab.status.kind).toBe('empty')
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('loaded tab exposes pipeline', () => {
    const pipeline: PipelineState = {
      file: '/a.yml', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: false,
    }
    const tab: TabState = { ...emptyTab('/a.yml'), status: { kind: 'loaded', pipeline } }
    expect(pipelineOf(tab.status)).toBe(pipeline)
  })

  it('running tab exposes pipeline', () => {
    const pipeline: PipelineState = {
      file: '/a.yml', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: true,
    }
    const tab: TabState = { ...emptyTab('/a.yml'), status: { kind: 'running', pipeline } }
    expect(pipelineOf(tab.status)).toBe(pipeline)
  })

  it('missing tab has no pipeline', () => {
    const tab: TabState = { ...emptyTab('/a.yml'), status: { kind: 'missing' } }
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('error tab carries message, no pipeline', () => {
    const tab: TabState = { ...emptyTab('/a.yml'), status: { kind: 'error', message: 'bad yaml' } }
    expect(tab.status.kind).toBe('error')
    if (tab.status.kind === 'error') expect(tab.status.message).toBe('bad yaml')
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('setup-error tab carries message, no pipeline', () => {
    const tab: TabState = { ...emptyTab('/a.yml'), status: { kind: 'setup-error', message: 'docker died' } }
    expect(tab.status.kind).toBe('setup-error')
    if (tab.status.kind === 'setup-error') expect(tab.status.message).toBe('docker died')
    expect(pipelineOf(tab.status)).toBeNull()
  })
})
