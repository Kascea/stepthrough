import { useEffect, useState } from 'react'
import { api } from '../api'

export function useDockerStatus(): boolean | null {
  const [ready, setReady] = useState<boolean | null>(null)

  useEffect(() => {
    if (ready === true) return
    const check = () =>
      api.checkDockerReady()
        .then(setReady)
        .catch(() => setReady(false))
    check()
    const id = setInterval(check, 2000)
    return () => clearInterval(id)
  }, [ready])

  return ready
}
