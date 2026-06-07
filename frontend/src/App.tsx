import { useReducer } from 'react'
import './app.css'

import { api } from './api'
import { reducer, initialState, pipelineOf } from './reducer'
import { useDockerStatus } from './hooks/useDockerStatus'
import { usePipelineEvents } from './hooks/usePipelineEvents'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LogPanel from './components/LogPanel'
import SplashScreen from './components/SplashScreen'
import ErrorScreen from './components/ErrorScreen'

export default function App() {
  const [state, dispatch] = useReducer(reducer, initialState)
  const dockerReady = useDockerStatus()
  usePipelineEvents(dispatch)

  const activeTab = state.tabs.find(t => t.file === state.activeFile) ?? null
  const activePipeline = activeTab ? pipelineOf(activeTab.status) : null

  const addPipeline = () => api.selectAndAdd().catch(console.error)

  const closeTab = (file: string) => {
    dispatch({ type: 'tab:removed', payload: file })
    api.removeTab(file).catch(console.error)
  }

  const switchTab = (file: string) => {
    dispatch({ type: 'tab:activated', payload: file })
    api.setActiveTab(file).catch(console.error)
  }

  const runPipeline = (file: string) => {
    if (!activeTab || activeTab.status.kind !== 'loaded') return
    api.runPipeline(file, 0).catch(console.error)
  }

  const cancelPipeline = (file: string) => api.cancelPipeline(file).catch(console.error)

  const relocate = (file: string) => api.relocateAndWatch(file).catch(console.error)

  const showSplash = dockerReady !== true || state.tabs.length === 0

  if (showSplash) {
    return (
      <SplashScreen
        onOpen={dockerReady === true ? addPipeline : undefined}
        setupLogs={activeTab?.setupLogs ?? []}
        isSettingUp={activeTab?.isSettingUp ?? false}
        dockerReady={dockerReady}
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
