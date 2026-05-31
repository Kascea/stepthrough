import { useState, useEffect } from 'react'
import { Events, Call } from '@wailsio/runtime'
import './app.css'

import { PipelineState, LogLine } from './types'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LogPanel from './components/LogPanel'
import SplashScreen from './components/SplashScreen'
import ErrorScreen from './components/ErrorScreen'

export default function App() {
  const [state, setState] = useState<PipelineState | null>(null)
  const [parseError, setParseError] = useState<string | null>(null)
  const [selectedStep, setSelectedStep] = useState<number | null>(null)
  const [logs, setLogs] = useState<Record<number, string[]>>({})
  const [setupLogs, setSetupLogs] = useState<string[]>([])
  const [isSettingUp, setIsSettingUp] = useState(false)

  useEffect(() => {
    Events.On('pipeline:loaded', (event: any) => {
      const s: PipelineState = event.data
      setState(s)
      setParseError(null)
      setLogs({})
      setSetupLogs([])
      setIsSettingUp(false)
    })

    Events.On('pipeline:error', (event: any) => {
      setParseError(event.data)
      setIsSettingUp(false)
    })

    Events.On('step:started', (event: any) => {
      const idx: number = event.data
      setIsSettingUp(false)
      setState(prev => {
        if (!prev) return prev
        const steps = [...prev.steps]
        steps[idx] = { ...steps[idx], status: 'running' }
        return { ...prev, steps, running: true }
      })
      setSelectedStep(idx)
    })

    Events.On('step:log', (event: any) => {
      const { stepIndex, line }: LogLine = event.data
      setLogs(prev => ({
        ...prev,
        [stepIndex]: [...(prev[stepIndex] ?? []), line],
      }))
    })

    Events.On('step:cached', (event: any) => {
      const idx: number = event.data
      setState(prev => {
        if (!prev) return prev
        const steps = [...prev.steps]
        steps[idx] = { ...steps[idx], status: 'cached' }
        return { ...prev, steps }
      })
    })

    Events.On('pipeline:done', (event: any) => {
      setState(event.data)
      setIsSettingUp(false)
    })

    Events.On('setup:log', (event: any) => {
      setIsSettingUp(true)
      setSetupLogs(prev => [...prev, event.data])
    })
  }, [])

  const openFile = () => {
    Call.ByName('github.com/colecarlson/stepthrough/service.UIService.SelectAndWatch').catch(console.error)
  }

  if (parseError) return <ErrorScreen error={parseError} />

  if (!state) return <SplashScreen onOpen={openFile} setupLogs={setupLogs} isSettingUp={isSettingUp} />

  return (
    <div className="app">
      <Topbar state={state} isSettingUp={isSettingUp} />
      <div className="layout">
        <Sidebar
          steps={state.steps ?? []}
          selectedStep={selectedStep}
          onSelectStep={setSelectedStep}
          isSettingUp={isSettingUp}
        />
        <LogPanel
          selectedStep={selectedStep}
          steps={state.steps ?? []}
          logs={logs}
          setupLogs={setupLogs}
          variables={state.variables ?? {}}
          isSettingUp={isSettingUp}
        />
      </div>
    </div>
  )
}
