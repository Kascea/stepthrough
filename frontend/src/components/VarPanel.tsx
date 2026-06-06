interface Props {
  variables: Record<string, string>
}

export default function VarPanel({ variables }: Props) {
  const entries = Object.entries(variables)
  if (entries.length === 0) return null

  return (
    <div className="var-panel">
      {entries.map(([k, v]) => (
        <div key={k} className="var-row">
          <span className="var-name">{k}</span>
          <span className="var-value">{v || '(empty)'}</span>
        </div>
      ))}
    </div>
  )
}
