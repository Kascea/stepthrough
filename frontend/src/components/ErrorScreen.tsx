interface Props {
  error: string
}

export default function ErrorScreen({ error }: Props) {
  return (
    <div className="app error-screen">
      <div className="error-header">YAML parse error — fix and save to retry</div>
      <pre className="error-body">{error}</pre>
    </div>
  )
}
