import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from '@/lib/api'
import type { InspectMetadata } from '@/types/api'

interface InspectorState {
  loading: boolean
  metadata: InspectMetadata | null
  error: ApiError | null
}

/**
 * useInspector debounces URL input by 400 ms and runs the pre-flight probe.
 * Re-inspection happens whenever the auth profile changes, because the probe
 * result depends on its headers.
 */
export function useInspector(url: string, authProfileId: string, enabled = true) {
  const [state, setState] = useState<InspectorState>({ loading: false, metadata: null, error: null })
  const seq = useRef(0)

  useEffect(() => {
    if (!enabled || !/^https?:\/\//i.test(url.trim())) {
      setState({ loading: false, metadata: null, error: null })
      return
    }
    const id = ++seq.current
    setState((s) => ({ ...s, loading: true, error: null }))
    const timer = window.setTimeout(() => {
      api
        .inspect(url.trim(), authProfileId)
        .then((metadata) => {
          if (seq.current === id) setState({ loading: false, metadata, error: null })
        })
        .catch((err: unknown) => {
          if (seq.current !== id) return
          const error = err instanceof ApiError ? err : null
          setState({ loading: false, metadata: null, error })
        })
    }, 400)
    return () => window.clearTimeout(timer)
  }, [url, authProfileId, enabled])

  return state
}
