import { StepStatus } from '../types'

interface Props {
  status: StepStatus
  size?: number
}

export default function StatusIcon({ status, size = 16 }: Props) {
  const r = size / 2

  if (status === 'pending') {
    // Grey outline circle with a small centre dot — "queued / not yet started"
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#c8c6c4" strokeWidth="1.5" />
        <circle cx={r} cy={r} r={r * 0.22} fill="#c8c6c4" />
      </svg>
    )
  }

  if (status === 'running') {
    // Blue arc spinner
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }} className="status-spin">
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#c7e0f4" strokeWidth="1.5" />
        <path
          d={`M ${r} ${1.5} A ${r - 1.5} ${r - 1.5} 0 0 1 ${size - 1.5} ${r}`}
          fill="none"
          stroke="#0078d4"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
      </svg>
    )
  }

  if (status === 'passed') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#107c10" />
        <polyline points={`${size*0.28},${size*0.52} ${size*0.44},${size*0.68} ${size*0.72},${size*0.36}`}
          fill="none" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }

  if (status === 'failed') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#c50f1f" />
        <line x1={size*0.3} y1={size*0.3} x2={size*0.7} y2={size*0.7} stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
        <line x1={size*0.7} y1={size*0.3} x2={size*0.3} y2={size*0.7} stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    )
  }

  if (status === 'skipped') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#a19f9d" strokeWidth="1.5" strokeDasharray="2 2" />
        <line x1={size*0.32} y1={r} x2={size*0.68} y2={r} stroke="#a19f9d" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    )
  }

  if (status === 'cached') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#0078d4" />
        <text x={r} y={r + size*0.2} textAnchor="middle" fontSize={size*0.65} fill="#fff" fontFamily="system-ui">⚡</text>
      </svg>
    )
  }

  return null
}
