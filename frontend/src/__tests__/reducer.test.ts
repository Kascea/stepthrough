import { describe, it, expect } from 'vitest'
import { TabState, SavedRun, PipelineState, PipelineFileEvent } from '../types'

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
    pipeline: null,
    missing: false,
    parseError: null,
    setupError: null,
    selectedStep: null,
    logs: {},
    setupLogs: [],
    isSettingUp: false,
  }
}

function tabFromSavedRun(file: string, run: SavedRun): TabState {
  const tab = emptyTab(file)
  tab.pipeline = {
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
  tab.logs = logs
  return tab
}

type SessionRestoredPayload = {
  tabOrder: string[] | null | undefined
  activeFile: string
  runs: Record<string, SavedRun>
}

// Mirrors the session:restored case in App.tsx reducer.
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

  it('restores tabs with saved run data', () => {
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
    expect(result.tabs[0].pipeline?.steps).toHaveLength(1)
    expect(result.tabs[0].pipeline?.steps[0].status).toBe('passed')
    expect(result.tabs[0].logs[0]).toEqual(['step output'])
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
    expect(result.tabs[0].pipeline).toBeNull()
    expect(result.tabs[0].logs).toEqual({})
  })
})

// ── step:done applies full PipelineState from event payload (no RPC round-trip) ─

describe('step:done', () => {
  function applyStepDone(state: AppState, event: PipelineFileEvent<PipelineState>): AppState {
    const { file, data: pipeline } = event
    return {
      ...state,
      tabs: state.tabs.map(t => t.file === file ? { ...t, pipeline: { ...pipeline } } : t),
    }
  }

  it('updates tab pipeline state directly from event payload', () => {
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{ ...emptyTab(file), pipeline: { file, valid: true, error: '', steps: [
        { index: 0, stageName: 'S', jobName: 'J', label: 'step', type: 'script', status: 'running', exitCode: 0, durationMs: 0 },
      ], variables: {}, safeMode: false, running: true } }],
      activeFile: file,
    }
    const updatedPipeline: PipelineState = {
      file, valid: true, error: '', running: false, safeMode: false, variables: {},
      steps: [
        { index: 0, stageName: 'S', jobName: 'J', label: 'step', type: 'script', status: 'passed', exitCode: 0, durationMs: 420 },
      ],
    }
    const result = applyStepDone(initial, { file, data: updatedPipeline })
    expect(result.tabs[0].pipeline?.steps[0].status).toBe('passed')
    expect(result.tabs[0].pipeline?.steps[0].durationMs).toBe(420)
    expect(result.tabs[0].pipeline?.running).toBe(false)
  })

  it('is a no-op for an unknown file', () => {
    const state: AppState = { ...initialState, tabs: [emptyTab('/other.yml')], activeFile: '/other.yml' }
    const result = applyStepDone(state, { file: '/unknown.yml', data: { file: '/unknown.yml', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: false } })
    expect(result.tabs[0].pipeline).toBeNull()
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
