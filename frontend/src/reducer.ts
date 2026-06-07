import {
  PipelineState,
  LogLine,
  PipelineFileEvent,
  SessionData,
  SavedRun,
  TabState,
  TabStatus,
} from './types'

export interface AppState {
  tabs: TabState[]
  activeFile: string | null
}

export type AppAction =
  | { type: 'session:restored'; payload: SessionData }
  | { type: 'tab:added'; payload: string }
  | { type: 'tab:removed'; payload: string }
  | { type: 'tab:activated'; payload: string }
  | { type: 'pipeline:loaded'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'pipeline:error'; payload: PipelineFileEvent<string> }
  | { type: 'pipeline:missing'; payload: PipelineFileEvent<null> }
  | { type: 'pipeline:setup-error'; payload: PipelineFileEvent<string> }
  | { type: 'pipeline:tab:relocated'; payload: PipelineFileEvent<{ from: string; to: string }> }
  | { type: 'step:started'; payload: PipelineFileEvent<number> }
  | { type: 'step:log'; payload: PipelineFileEvent<LogLine> }
  | { type: 'step:cached'; payload: PipelineFileEvent<number> }
  | { type: 'step:done'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'pipeline:done'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'setup:log'; payload: PipelineFileEvent<string> }
  | { type: 'step:selected'; payload: { file: string; index: number } }

export const initialState: AppState = {
  tabs: [],
  activeFile: null,
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
    repoRoot: '',
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

function updateTab(tabs: TabState[], file: string, fn: (t: TabState) => TabState): TabState[] {
  return tabs.map(t => (t.file === file ? fn(t) : t))
}

export function pipelineOf(status: TabStatus): PipelineState | null {
  return status.kind === 'loaded' || status.kind === 'running' ? status.pipeline : null
}

export function reducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case 'session:restored': {
      const { tabOrder, activeFile, runs } = action.payload
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

    case 'tab:added': {
      if (state.tabs.some(t => t.file === action.payload)) {
        return { ...state, activeFile: action.payload }
      }
      return {
        ...state,
        tabs: [...state.tabs, emptyTab(action.payload)],
        activeFile: action.payload,
      }
    }

    case 'tab:removed': {
      const remaining = state.tabs.filter(t => t.file !== action.payload)
      const newActive =
        state.activeFile === action.payload
          ? remaining[0]?.file ?? null
          : state.activeFile
      return { ...state, tabs: remaining, activeFile: newActive }
    }

    case 'tab:activated':
      return { ...state, activeFile: action.payload }

    case 'pipeline:loaded': {
      const { file, data: pipeline } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          status: { kind: 'loaded', pipeline },
          selectedStep: null,
          logs: {},
          setupLogs: [],
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:error': {
      const { file, data: message } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          status: { kind: 'error', message },
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:missing': {
      const { file } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          status: { kind: 'missing' },
        })),
      }
    }

    case 'pipeline:setup-error': {
      const { file, data: message } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          status: { kind: 'setup-error', message },
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:tab:relocated': {
      const { from, to } = action.payload.data
      return {
        ...state,
        tabs: state.tabs.map(t =>
          t.file === from ? { ...t, file: to, status: { kind: 'empty' } } : t
        ),
        activeFile: state.activeFile === from ? to : state.activeFile,
      }
    }

    case 'step:started': {
      const { file, data: stepIndex } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => {
          const pipeline = pipelineOf(t.status)
          if (!pipeline) return t
          const steps = [...pipeline.steps]
          steps[stepIndex] = { ...steps[stepIndex], status: 'running' }
          return {
            ...t,
            status: { kind: 'running', pipeline: { ...pipeline, steps, running: true } },
            selectedStep: stepIndex,
            isSettingUp: false,
          }
        }),
      }
    }

    case 'step:log': {
      const { file, data: ll } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          logs: {
            ...t.logs,
            [ll.stepIndex]: [...(t.logs[ll.stepIndex] ?? []), ll.line],
          },
        })),
      }
    }

    case 'step:cached': {
      const { file, data: stepIndex } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => {
          if (t.status.kind !== 'loaded' && t.status.kind !== 'running') return t
          const steps = [...t.status.pipeline.steps]
          steps[stepIndex] = { ...steps[stepIndex], status: 'cached' }
          return { ...t, status: { ...t.status, pipeline: { ...t.status.pipeline, steps } } }
        }),
      }
    }

    case 'step:done': {
      const { file, data: incoming } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => {
          // Preserve any step already marked 'running' in local state — the
          // step:done snapshot is taken before the next step:started fires on
          // the backend, so it can arrive after flushSync has already rendered
          // the next step as 'running'. Overwriting that with 'pending' would
          // kill the spinner.
          const current = pipelineOf(t.status)
          const steps = incoming.steps.map((s, i) =>
            current?.steps[i]?.status === 'running' && s.status === 'pending'
              ? current.steps[i]
              : s
          )
          return { ...t, status: { kind: 'running', pipeline: { ...incoming, steps } } }
        }),
      }
    }

    case 'pipeline:done': {
      const { file, data: pipeline } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          status: { kind: 'loaded', pipeline },
          isSettingUp: false,
        })),
      }
    }

    case 'setup:log': {
      const { file, data: line } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => {
          // Transition loaded → running when setup begins so the Cancel button appears.
          const newStatus: TabStatus =
            t.status.kind === 'loaded'
              ? { kind: 'running', pipeline: t.status.pipeline }
              : t.status
          return {
            ...t,
            status: newStatus,
            isSettingUp: true,
            selectedStep: null,
            setupLogs: [...t.setupLogs, line],
          }
        }),
      }
    }

    case 'step:selected': {
      const { file, index } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({ ...t, selectedStep: index })),
      }
    }

    default:
      return state
  }
}
