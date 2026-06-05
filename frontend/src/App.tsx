import { useEffect, useReducer } from 'react'
import { Events, Call } from '@wailsio/runtime'
import './app.css'

import {
  PipelineState,
  LogLine,
  PipelineFileEvent,
  SessionData,
  SavedRun,
  TabState,
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
  | { type: 'step:done'; payload: { file: string; pipeline: PipelineState } }
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
  // Restore last run's step statuses so the user sees their previous results immediately.
  tab.pipeline = {
    file,
    valid: true,
    error: '',
    steps: run.steps ?? [],
    variables: {},
    safeMode: true,
    running: false,
  }
  // Restore logs keyed by numeric step index.
  const logs: Record<number, string[]> = {}
  for (const [k, v] of Object.entries(run.logs ?? {})) {
    logs[Number(k)] = v
  }
  tab.logs = logs
  return tab
}

function updateTab(tabs: TabState[], file: string, fn: (t: TabState) => TabState): TabState[] {
  return tabs.map(t => (t.file === file ? fn(t) : t))
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
        // Already exists (e.g. session-restored) — just switch to it.
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
          pipeline,
          missing: false,
          parseError: null,
          setupError: null,
          selectedStep: null,
          logs: {},
          setupLogs: [],
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:error': {
      const { file, data: err } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          parseError: err,
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:missing': {
      const { file } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({ ...t, missing: true })),
      }
    }

    case 'pipeline:setup-error': {
      const { file, data: err } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          setupError: err,
          isSettingUp: false,
        })),
      }
    }

    case 'pipeline:tab:relocated': {
      const { from, to } = action.payload.data
      return {
        ...state,
        tabs: state.tabs.map(t =>
          t.file === from ? { ...t, file: to, missing: false } : t
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
          if (!t.pipeline) return t
          const steps = [...t.pipeline.steps]
          steps[stepIndex] = { ...steps[stepIndex], status: 'running' }
          return {
            ...t,
            pipeline: { ...t.pipeline, steps, running: true },
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
          if (!t.pipeline) return t
          const steps = [...t.pipeline.steps]
          steps[stepIndex] = { ...steps[stepIndex], status: 'cached' }
          return { ...t, pipeline: { ...t.pipeline, steps } }
        }),
      }
    }

    case 'step:done': {
      const { file, pipeline } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          pipeline: { ...pipeline },
        })),
      }
    }

    case 'pipeline:done': {
      const { file, data: pipeline } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          pipeline,
          isSettingUp: false,
        })),
      }
    }

    case 'setup:log': {
      const { file, data: line } = action.payload
      return {
        ...state,
        tabs: updateTab(state.tabs, file, t => ({
          ...t,
          isSettingUp: true,
          selectedStep: null,
          setupLogs: [...t.setupLogs, line],
        })),
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

  // Poll Docker availability every 2s until confirmed ready.
  useEffect(() => {
    if (state.dockerReady === true) return
    const check = () => {
      Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.CheckDockerReady')
        .then((ready: boolean) => dispatch({ type: 'docker:status', payload: ready }))
        .catch(() => dispatch({ type: 'docker:status', payload: false }))
    }
    check()
    const id = setInterval(check, 2000)
    return () => clearInterval(id)
  }, [state.dockerReady])

  // Register all pipeline event listeners then restore session.
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
        dispatch({ type: 'step:started', payload: e })
      )),
      Events.On('step:log', unwrap<LogLine>(e =>
        dispatch({ type: 'step:log', payload: e })
      )),
      Events.On('step:cached', unwrap<number>(e =>
        dispatch({ type: 'step:cached', payload: e })
      )),
      Events.On('step:done', unwrap<number>(e => {
        Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.GetPipelineState', e.file)
          .then((pipeline: PipelineState) =>
            dispatch({ type: 'step:done', payload: { file: e.file, pipeline } })
          )
          .catch(console.error)
      })),
      Events.On('pipeline:done', unwrap<PipelineState>(e =>
        dispatch({ type: 'pipeline:done', payload: e })
      )),
      Events.On('setup:log', unwrap<string>(e =>
        dispatch({ type: 'setup:log', payload: e })
      )),
    ]

    // Restore previous session after listeners are registered.
    Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.GetSession')
      .then((data: SessionData) => dispatch({ type: 'session:restored', payload: data }))
      .catch(console.error)

    return () => unsubs.forEach(u => u())
  }, [])

  const activeTab = state.tabs.find(t => t.file === state.activeFile) ?? null

  const addPipeline = () => {
    Call.ByName('github.com/colecarlson/stepthrough/service.UIService.SelectAndAdd').catch(console.error)
  }

  const closeTab = (file: string) => {
    dispatch({ type: 'tab:removed', payload: file })
    Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.RemoveTab', file).catch(console.error)
  }

  const switchTab = (file: string) => {
    dispatch({ type: 'tab:activated', payload: file })
    Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.SetActiveTab', file).catch(console.error)
  }

  const runPipeline = (file: string) => {
    if (!activeTab?.pipeline || activeTab.missing) return
    Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.RunPipeline', file, 0).catch(console.error)
  }

  const cancelPipeline = (file: string) => {
    Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.CancelPipeline', file).catch(console.error)
  }

  const relocate = (file: string) => {
    Call.ByName('github.com/colecarlson/stepthrough/service.UIService.RelocateAndWatch', file).catch(console.error)
  }

  // Show splash when Docker is not running or there are no tabs.
  const showSplash = state.dockerReady !== true || state.tabs.length === 0

  if (showSplash) {
    return (
      <SplashScreen
        onOpen={state.dockerReady === true ? addPipeline : undefined}
        setupLogs={activeTab?.setupLogs ?? []}
        isSettingUp={activeTab?.isSettingUp ?? false}
        dockerReady={state.dockerReady}
        setupError={activeTab?.setupError ?? null}
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
        running={activeTab.pipeline?.running ?? false}
        missing={activeTab.missing}
      />
      {activeTab.missing && (
        <div className="missing-banner">
          <span className="missing-icon">!</span>
          File not found — pipeline was moved or deleted.
          <button className="missing-locate-btn" onClick={() => relocate(activeTab.file)}>
            Locate file...
          </button>
        </div>
      )}
      {activeTab.setupError && !activeTab.missing && (
        <div className="setup-error-banner">
          <span className="setup-error-icon">!</span>
          {activeTab.setupError}
        </div>
      )}
      {activeTab.parseError ? (
        <ErrorScreen error={activeTab.parseError} />
      ) : (
        <div className="layout">
          <Sidebar
            steps={activeTab.pipeline?.steps ?? []}
            selectedStep={activeTab.selectedStep}
            onSelectStep={index =>
              dispatch({ type: 'step:selected', payload: { file: activeTab.file, index } })
            }
          />
          <LogPanel
            selectedStep={activeTab.selectedStep}
            steps={activeTab.pipeline?.steps ?? []}
            logs={activeTab.logs}
            setupLogs={activeTab.setupLogs}
            variables={activeTab.pipeline?.variables ?? {}}
            isSettingUp={activeTab.isSettingUp}
          />
        </div>
      )}
    </div>
  )
}
