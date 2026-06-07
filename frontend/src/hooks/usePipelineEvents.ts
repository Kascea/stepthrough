import { useEffect } from 'react'
import { flushSync } from 'react-dom'
import { Events } from '@wailsio/runtime'
import { AppAction } from '../reducer'
import { PipelineState, LogLine, PipelineFileEvent, SessionData } from '../types'
import { api } from '../api'

function unwrap<T>(handler: (e: PipelineFileEvent<T>) => void) {
  return (event: any) => handler(event.data as PipelineFileEvent<T>)
}

export function usePipelineEvents(dispatch: React.Dispatch<AppAction>) {
  useEffect(() => {
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

    api.getSession()
      .then((data: SessionData) => dispatch({ type: 'session:restored', payload: data }))
      .catch(console.error)

    return () => unsubs.forEach(u => u())
  }, [])
}
