import { PipelineState } from '../types'

interface Props {
  state: PipelineState
}

export default function Topbar({ state }: Props) {
  const filename = state.file.split('/').pop() ?? state.file

  return (
    <header className="topbar">
      <span className="topbar-title">stepthrough</span>
      <span className="topbar-sep">/</span>
      <span className="topbar-file">{filename}</span>
    </header>
  )
}
