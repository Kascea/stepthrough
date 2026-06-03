import { StepStatus } from '../types'

interface Props {
  status: StepStatus
  size?: number
}

export default function StatusIcon({ status, size = 16 }: Props) {
  const r = size / 2

  if (status === 'pending') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#98a2b3" strokeWidth="1.5" />
        <circle cx={r} cy={r} r={r * 0.22} fill="#98a2b3" />
      </svg>
    )
  }

  if (status === 'running') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }} className="status-spin">
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#cdd6e4" strokeWidth="1.5" />
        <path
          d={`M ${r} ${1.5} A ${r - 1.5} ${r - 1.5} 0 0 1 ${size - 1.5} ${r}`}
          fill="none"
          stroke="#2563eb"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
      </svg>
    )
  }

  if (status === 'passed') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#059669" />
        <polyline points={`${size*0.28},${size*0.52} ${size*0.44},${size*0.68} ${size*0.72},${size*0.36}`}
          fill="none" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }

  if (status === 'failed') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#dc2626" />
        <line x1={size*0.3} y1={size*0.3} x2={size*0.7} y2={size*0.7} stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
        <line x1={size*0.7} y1={size*0.3} x2={size*0.3} y2={size*0.7} stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    )
  }

  if (status === 'skipped') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r - 1.5} fill="none" stroke="#98a2b3" strokeWidth="1.5" strokeDasharray="2 2" />
        <line x1={size*0.32} y1={r} x2={size*0.68} y2={r} stroke="#98a2b3" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    )
  }

  if (status === 'cached') {
    return (
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
        <circle cx={r} cy={r} r={r} fill="#0f766e" />
        <rect x={size * 0.24} y={size * 0.34} width={size * 0.5} height={size * 0.14} rx={size * 0.05} fill="none" stroke="#fff" strokeWidth="1.2" />
        <rect x={size * 0.3} y={size * 0.52} width={size * 0.5} height={size * 0.14} rx={size * 0.05} fill="none" stroke="#fff" strokeWidth="1.2" />
      </svg>
    )
  }

  return null
}
