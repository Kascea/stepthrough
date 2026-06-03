import { useEffect, useReducer } from 'react'
import { Events, Call } from '@wailsio/runtime'
import './app.css'

import { PipelineState, LogLine } from './types'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LogPanel from './components/LogPanel'
import SplashScreen from './components/SplashScreen'
import ErrorScreen from './components/ErrorScreen'

interface AppState {
  pipeline: PipelineState | null
  parseError: string | null
  selectedStep: number | null
  logs: Record<number, string[]>
  setupLogs: string[]
  isSettingUp: boolean
}

type AppAction =
  | { type: 'pipeline:loaded'; payload: PipelineState }
  | { type: 'pipeline:synced'; payload: PipelineState }
  | { type: 'pipeline:error'; payload: string }
  | { type: 'step:started'; payload: number }
  | { type: 'step:log'; payload: LogLine }
  | { type: 'step:cached'; payload: number }
  | { type: 'pipeline:done'; payload: PipelineState }
  | { type: 'setup:log'; payload: string }
  | { type: 'step:selected'; payload: number }

const initialState: AppState = {
  pipeline: null,
  parseError: null,
  selectedStep: null,
  logs: {},
  setupLogs: [],
  isSettingUp: false,
}

function reducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case 'pipeline:loaded':
      return {
        pipeline: action.payload,
        parseError: null,
        selectedStep: null,
        logs: {},
        setupLogs: [],
        isSettingUp: false,
      }
    case 'pipeline:synced':
      return {
        ...state,
        pipeline: {
          ...action.payload,
        },
      }
    case 'pipeline:error':
      return {
        ...state,
        parseError: action.payload,
        isSettingUp: false,
      }
    case 'step:started': {
      if (!state.pipeline) return state
      const steps = [...state.pipeline.steps]
      steps[action.payload] = { ...steps[action.payload], status: 'running' }
      return {
        ...state,
        pipeline: { ...state.pipeline, steps, running: true },
        selectedStep: action.payload,
        isSettingUp: false,
      }
    }
    case 'step:log': {
      const { stepIndex, line } = action.payload
      return {
        ...state,
        logs: {
          ...state.logs,
          [stepIndex]: [...(state.logs[stepIndex] ?? []), line],
        },
      }
    }
    case 'step:cached': {
      if (!state.pipeline) return state
      const steps = [...state.pipeline.steps]
      steps[action.payload] = { ...steps[action.payload], status: 'cached' }
      return {
        ...state,
        pipeline: { ...state.pipeline, steps },
      }
    }
    case 'pipeline:done':
      return {
        ...state,
        pipeline: action.payload,
        isSettingUp: false,
      }
    case 'setup:log':
      return {
        ...state,
        isSettingUp: true,
        selectedStep: null,
        setupLogs: [...state.setupLogs, action.payload],
      }
    case 'step:selected':
      return {
        ...state,
        selectedStep: action.payload,
      }
    default:
      return state
  }
}

export default function App() {
  const [state, dispatch] = useReducer(reducer, initialState)

  useEffect(() => {
    const unsubs = [
      Events.On('pipeline:loaded', (event: any) => {
        dispatch({ type: 'pipeline:loaded', payload: event.data as PipelineState })
      }),
      Events.On('pipeline:error', (event: any) => {
        dispatch({ type: 'pipeline:error', payload: event.data as string })
      }),
      Events.On('step:started', (event: any) => {
        dispatch({ type: 'step:started', payload: event.data as number })
      }),
      Events.On('step:log', (event: any) => {
        dispatch({ type: 'step:log', payload: event.data as LogLine })
      }),
      Events.On('step:cached', (event: any) => {
        dispatch({ type: 'step:cached', payload: event.data as number })
      }),
      Events.On('step:done', () => {
        Call.ByName('github.com/colecarlson/stepthrough/service.PipelineService.GetState')
          .then((pipeline: PipelineState) => {
            dispatch({ type: 'pipeline:synced', payload: pipeline })
          })
          .catch(console.error)
      }),
      Events.On('pipeline:done', (event: any) => {
        dispatch({ type: 'pipeline:done', payload: event.data as PipelineState })
      }),
      Events.On('setup:log', (event: any) => {
        dispatch({ type: 'setup:log', payload: event.data as string })
      }),
    ]

    return () => {
      unsubs.forEach(unsub => unsub())
    }
  }, [])

  const openFile = () => {
    Call.ByName('github.com/colecarlson/stepthrough/service.UIService.SelectAndWatch').catch(console.error)
  }

  if (state.parseError) return <ErrorScreen error={state.parseError} />

  if (!state.pipeline) {
    return <SplashScreen onOpen={openFile} setupLogs={state.setupLogs} isSettingUp={state.isSettingUp} />
  }

  return (
    <div className="app">
      <Topbar state={state.pipeline} />
      <div className="layout">
        <Sidebar
          steps={state.pipeline.steps ?? []}
          selectedStep={state.selectedStep}
          onSelectStep={index => dispatch({ type: 'step:selected', payload: index })}
        />
        <LogPanel
          selectedStep={state.selectedStep}
          steps={state.pipeline.steps ?? []}
          logs={state.logs}
          setupLogs={state.setupLogs}
          variables={state.pipeline.variables ?? {}}
          isSettingUp={state.isSettingUp}
        />
      </div>
    </div>
  )
}
