import { describe, it, expect } from 'vitest'
import { reducer, initialState, pipelineOf, AppState } from '../reducer'
import { SavedRun, PipelineState, PipelineFileEvent, StepStatus } from '../types'

function emptyTabState(file: string): AppState['tabs'][0] {
  return {
    file,
    status: { kind: 'empty' },
    selectedStep: null,
    logs: {},
    setupLogs: [],
    isSettingUp: false,
  }
}

// ── Regression: null/empty tabOrder must never crash ─────────────────────────

describe('session:restored', () => {
  it('returns state unchanged when tabOrder is null (Go nil slice → JSON null)', () => {
    const result = reducer(initialState, {
      type: 'session:restored',
      payload: { tabOrder: null as any, activeFile: '', runs: {} },
    })
    expect(result).toBe(initialState)
  })

  it('returns state unchanged when tabOrder is undefined', () => {
    const result = reducer(initialState, {
      type: 'session:restored',
      payload: { tabOrder: undefined as any, activeFile: '', runs: {} },
    })
    expect(result).toBe(initialState)
  })

  it('returns state unchanged when tabOrder is empty', () => {
    const result = reducer(initialState, {
      type: 'session:restored',
      payload: { tabOrder: [], activeFile: '', runs: {} },
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

    const result = reducer(initialState, {
      type: 'session:restored',
      payload: {
        tabOrder: ['/path/to/pipeline.yml'],
        activeFile: '/path/to/pipeline.yml',
        runs: { '/path/to/pipeline.yml': savedRun },
      },
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
    const result = reducer(initialState, {
      type: 'session:restored',
      payload: { tabOrder: ['/a.yml', '/b.yml'], activeFile: '', runs: {} },
    })
    expect(result.activeFile).toBe('/a.yml')
  })

  it('creates empty tabs for files with no saved run', () => {
    const result = reducer(initialState, {
      type: 'session:restored',
      payload: { tabOrder: ['/unseen.yml'], activeFile: '/unseen.yml', runs: {} },
    })
    expect(result.tabs[0].status.kind).toBe('empty')
    expect(result.tabs[0].logs).toEqual({})
  })
})

// ── step:done applies full PipelineState from event payload (no RPC round-trip) ─

describe('step:done', () => {
  function makeStep(index: number, status: StepStatus, durationMs = 0) {
    return { index, stageName: 'S', jobName: 'J', label: `step-${index}`, type: 'script', status, exitCode: 0, durationMs } as const
  }

  function applyStepDone(state: AppState, event: PipelineFileEvent<PipelineState>): AppState {
    return reducer(state, { type: 'step:done', payload: event })
  }

  it('transitions the completed step from running to passed', () => {
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{
        ...emptyTabState(file),
        status: { kind: 'running', pipeline: {
          file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
          steps: [makeStep(0, 'running')],
        }},
      }],
      activeFile: file,
    }
    const result = applyStepDone(initial, {
      file,
      data: { file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
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
        ...emptyTabState(file),
        status: { kind: 'running', pipeline: {
          file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
          steps: [makeStep(0, 'passed'), makeStep(1, 'running')],
        }},
      }],
      activeFile: file,
    }
    const result = applyStepDone(initial, {
      file,
      data: { file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
        steps: [makeStep(0, 'passed', 100), makeStep(1, 'pending')] },
    })
    const steps = pipelineOf(result.tabs[0].status)?.steps
    expect(steps?.[0].status).toBe('passed')
    expect(steps?.[1].status).toBe('running') // must NOT be overwritten to 'pending'
  })

  it('does update a pending step to passed when the snapshot reflects completion', () => {
    const file = '/pipeline.yml'
    const initial: AppState = {
      ...initialState,
      tabs: [{
        ...emptyTabState(file),
        status: { kind: 'running', pipeline: {
          file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
          steps: [makeStep(0, 'running'), makeStep(1, 'pending')],
        }},
      }],
      activeFile: file,
    }
    const result = applyStepDone(initial, {
      file,
      data: { file, repoRoot: '', valid: true, error: '', running: true, safeMode: false, variables: {},
        steps: [makeStep(0, 'passed', 200), makeStep(1, 'pending')] },
    })
    const steps = pipelineOf(result.tabs[0].status)?.steps
    expect(steps?.[0].status).toBe('passed')
    expect(steps?.[1].status).toBe('pending')
  })

  it('is a no-op for tabs with a different file', () => {
    const state: AppState = { ...initialState, tabs: [emptyTabState('/other.yml')], activeFile: '/other.yml' }
    const result = applyStepDone(state, {
      file: '/unknown.yml',
      data: { file: '/unknown.yml', repoRoot: '', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: false },
    })
    expect(result.tabs[0].status.kind).toBe('empty')
  })
})

// ── tab:added deduplication ───────────────────────────────────────────────────

describe('tab:added', () => {
  it('adds a new tab', () => {
    const result = reducer(initialState, { type: 'tab:added', payload: '/new.yml' })
    expect(result.tabs).toHaveLength(1)
    expect(result.activeFile).toBe('/new.yml')
  })

  it('does not duplicate an existing tab', () => {
    const withTab: AppState = { ...initialState, tabs: [emptyTabState('/existing.yml')], activeFile: null }
    const result = reducer(withTab, { type: 'tab:added', payload: '/existing.yml' })
    expect(result.tabs).toHaveLength(1)
    expect(result.activeFile).toBe('/existing.yml')
  })
})

// ── TabStatus discriminated union — impossible states are unrepresentable ─────

describe('TabStatus', () => {
  it('empty tab has no pipeline', () => {
    const tab = emptyTabState('/a.yml')
    expect(tab.status.kind).toBe('empty')
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('loaded tab exposes pipeline', () => {
    const pipeline: PipelineState = {
      file: '/a.yml', repoRoot: '', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: false,
    }
    const tab = { ...emptyTabState('/a.yml'), status: { kind: 'loaded' as const, pipeline } }
    expect(pipelineOf(tab.status)).toBe(pipeline)
  })

  it('running tab exposes pipeline', () => {
    const pipeline: PipelineState = {
      file: '/a.yml', repoRoot: '', valid: true, error: '', steps: [], variables: {}, safeMode: false, running: true,
    }
    const tab = { ...emptyTabState('/a.yml'), status: { kind: 'running' as const, pipeline } }
    expect(pipelineOf(tab.status)).toBe(pipeline)
  })

  it('missing tab has no pipeline', () => {
    const tab = { ...emptyTabState('/a.yml'), status: { kind: 'missing' as const } }
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('error tab carries message, no pipeline', () => {
    const tab = { ...emptyTabState('/a.yml'), status: { kind: 'error' as const, message: 'bad yaml' } }
    expect(tab.status.kind).toBe('error')
    if (tab.status.kind === 'error') expect(tab.status.message).toBe('bad yaml')
    expect(pipelineOf(tab.status)).toBeNull()
  })

  it('setup-error tab carries message, no pipeline', () => {
    const tab = { ...emptyTabState('/a.yml'), status: { kind: 'setup-error' as const, message: 'docker died' } }
    expect(tab.status.kind).toBe('setup-error')
    if (tab.status.kind === 'setup-error') expect(tab.status.message).toBe('docker died')
    expect(pipelineOf(tab.status)).toBeNull()
  })
})
