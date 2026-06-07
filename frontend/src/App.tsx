import { useEffect, useReducer } from 'react'
import { flushSync } from 'react-dom'
import { Events, Call } from '@wailsio/runtime'
import './app.css'

import {
  PipelineState,
  LogLine,
  PipelineFileEvent,
  SessionData,
  SavedRun,
  TabState,
  TabStatus,
} from './types'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LogPanel from './components/LogPanel'
import SplashScreen from './components/SplashScreen'
import ErrorScreen from './components/ErrorScreen'

interface AppState {
  tabs: TabState[]
  activeFile: string | null
  dockerReady: boolean | null
}

type AppAction =
  | { type: 'session:restored'; payload: SessionData }
  | { type: 'tab:added'; payload: string }
  | { type: 'tab:removed'; payload: string }
  | { type: 'tab:activated'; payload: string }
  | { type: 'pipeline:loaded'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'pipeline:error'; payload: PipelineFileEvent<string> }
  | { type: 'pipeline:missing'; payload: PipelineFileEvent<null> }
  | { type: 'pipeline:setup-error'; payload: PipelineFileEvent<string> }
  | { type: 'pipeline:tab:relocated'; payload: PipelineFileEvent<{ from: string; to: string }> }
  | { type: 'docker:status'; payload: boolean }
  | { type: 'step:started'; payload: PipelineFileEvent<number> }
  | { type: 'step:log'; payload: PipelineFileEvent<LogLine> }
  | { type: 'step:cached'; payload: PipelineFileEvent<number> }
  | { type: 'step:done'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'pipeline:done'; payload: PipelineFileEvent<PipelineState> }
  | { type: 'setup:log'; payload: PipelineFileEvent<string> }
  | { type: 'step:selected'; payload: { file: string; index: number } }

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

function updateTab(tabs: TabState[], file: string, fn: (t: TabState) => TabState): TabState[] {
  return tabs.map(t => (t.file === file ? fn(t) : t))
}

// Returns the pipeline from a tab status if one is available.
function pipelineOf(status: TabStatus): PipelineState | null {
  return status.kind === 'loaded' || status.kind === 'running' ? status.pipeline : null
}

function reducer(state: AppState, action: AppAction): AppState {
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

    case 'docker:status':
      return { ...state, dockerReady: action.payload }

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

export default function App() {
  const [state, dispatch] = useReducer(reducer, initialState)

  useEffect(() => {
    if (state.dockerReady === true) return
    const check = () => {
      Call.ByName('github.com/kascea/stepthrough/service.PipelineService.CheckDockerReady')
        .then((ready: boolean) => dispatch({ type: 'docker:status', payload: ready }))
        .catch(() => dispatch({ type: 'docker:status', payload: false }))
    }
    check()
    const id = setInterval(check, 2000)
    return () => clearInterval(id)
  }, [state.dockerReady])

  useEffect(() => {
    const unwrap = <T,>(handler: (e: PipelineFileEvent<T>) => void) =>
      (event: any) => handler(event.data as PipelineFileEvent<T>)

    const unsubs = [
      Events.On('pipeline:loaded', unwrap<PipelineState>(e =>
        dispatch({ type: 'pipeline:loaded', payload: e })
      )),
      Events.On('pipeline:error', unwrap<string>(e =>
        dispatch({ type: 'pipeline:error', payload: e })
      )),
      Events.On('pipeline:missing', unwrap<null>(e =>
        dispatch({ type: 'pipeline:missing', payload: e })
      )),
      Events.On('pipeline:setup-error', unwrap<string>(e =>
        dispatch({ type: 'pipeline:setup-error', payload: e })
      )),
      Events.On('pipeline:tab:added', (event: any) =>
        dispatch({ type: 'tab:added', payload: event.data as string })
      ),
      Events.On('pipeline:tab:relocated', unwrap<{ from: string; to: string }>(e =>
        dispatch({ type: 'pipeline:tab:relocated', payload: e })
      )),
      Events.On('step:started', unwrap<number>(e =>
        flushSync(() => dispatch({ type: 'step:started', payload: e }))
      )),
      Events.On('step:log', unwrap<LogLine>(e =>
        dispatch({ type: 'step:log', payload: e })
      )),
      Events.On('step:cached', unwrap<number>(e =>
        dispatch({ type: 'step:cached', payload: e })
      )),
      Events.On('step:done', unwrap<PipelineState>(e =>
        dispatch({ type: 'step:done', payload: e })
      )),
      Events.On('pipeline:done', unwrap<PipelineState>(e =>
        dispatch({ type: 'pipeline:done', payload: e })
      )),
      Events.On('setup:log', unwrap<string>(e =>
        dispatch({ type: 'setup:log', payload: e })
      )),
    ]

    Call.ByName('github.com/kascea/stepthrough/service.PipelineService.GetSession')
      .then((data: SessionData) => dispatch({ type: 'session:restored', payload: data }))
      .catch(console.error)

    return () => unsubs.forEach(u => u())
  }, [])

  const activeTab = state.tabs.find(t => t.file === state.activeFile) ?? null
  const activePipeline = activeTab ? pipelineOf(activeTab.status) : null

  const addPipeline = () => {
    Call.ByName('github.com/kascea/stepthrough/service.UIService.SelectAndAdd').catch(console.error)
  }

  const closeTab = (file: string) => {
    dispatch({ type: 'tab:removed', payload: file })
    Call.ByName('github.com/kascea/stepthrough/service.PipelineService.RemoveTab', file).catch(console.error)
  }

  const switchTab = (file: string) => {
    dispatch({ type: 'tab:activated', payload: file })
    Call.ByName('github.com/kascea/stepthrough/service.PipelineService.SetActiveTab', file).catch(console.error)
  }

  const runPipeline = (file: string) => {
    if (!activeTab || activeTab.status.kind !== 'loaded') return
    Call.ByName('github.com/kascea/stepthrough/service.PipelineService.RunPipeline', file, 0).catch(console.error)
  }

  const cancelPipeline = (file: string) => {
    Call.ByName('github.com/kascea/stepthrough/service.PipelineService.CancelPipeline', file).catch(console.error)
  }

  const relocate = (file: string) => {
    Call.ByName('github.com/kascea/stepthrough/service.UIService.RelocateAndWatch', file).catch(console.error)
  }

  const showSplash = state.dockerReady !== true || state.tabs.length === 0

  if (showSplash) {
    return (
      <SplashScreen
        onOpen={state.dockerReady === true ? addPipeline : undefined}
        setupLogs={activeTab?.setupLogs ?? []}
        isSettingUp={activeTab?.isSettingUp ?? false}
        dockerReady={state.dockerReady}
        setupError={activeTab && activeTab.status.kind === 'setup-error' ? activeTab.status.message : null}
      />
    )
  }

  if (!activeTab) {
    return (
      <div className="app">
        <Topbar
          tabs={state.tabs}
          activeFile={state.activeFile}
          onAdd={addPipeline}
          onClose={closeTab}
          onSwitch={switchTab}
          onRun={() => {}}
          onCancel={() => {}}
        />
      </div>
    )
  }

  const isMissing = activeTab.status.kind === 'missing'
  const isRunning = activeTab.status.kind === 'running'

  return (
    <div className="app">
      <Topbar
        tabs={state.tabs}
        activeFile={state.activeFile}
        onAdd={addPipeline}
        onClose={closeTab}
        onSwitch={switchTab}
        onRun={() => activeTab && runPipeline(activeTab.file)}
        onCancel={() => activeTab && cancelPipeline(activeTab.file)}
        running={isRunning}
        missing={isMissing}
      />
      {isMissing && (
        <div className="missing-banner">
          <span className="missing-icon">!</span>
          File not found — pipeline was moved or deleted.
          <button className="missing-locate-btn" onClick={() => relocate(activeTab.file)}>
            Locate file...
          </button>
        </div>
      )}
      {activeTab.status.kind === 'setup-error' && (
        <div className="setup-error-banner">
          <span className="setup-error-icon">!</span>
          {activeTab.status.message}
        </div>
      )}
      {activeTab.status.kind === 'error' ? (
        <ErrorScreen error={activeTab.status.message} />
      ) : (
        <div className="layout">
          <Sidebar
            steps={activePipeline?.steps ?? []}
            selectedStep={activeTab.selectedStep}
            onSelectStep={index =>
              dispatch({ type: 'step:selected', payload: { file: activeTab.file, index } })
            }
          />
          <LogPanel
            selectedStep={activeTab.selectedStep}
            steps={activePipeline?.steps ?? []}
            logs={activeTab.logs}
            setupLogs={activeTab.setupLogs}
            variables={activePipeline?.variables ?? {}}
            isSettingUp={activeTab.isSettingUp}
          />
        </div>
      )}
    </div>
  )
}
