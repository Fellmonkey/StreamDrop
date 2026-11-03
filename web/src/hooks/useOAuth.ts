import { useCallback, useRef, useState } from 'react'
import { api, ApiError } from '@/lib/api'

interface OAuthState {
  status: 'idle' | 'pending' | 'done' | 'failed'
  error: ApiError | null
  sessionId: string | null
}

/**
 * useOAuth starts a provider flow, opens the system browser and polls the
 * one-time session until the callback lands on the fixed loopback port.
 */
export function useOAuth(onDone?: () => void) {
  const [state, setState] = useState<OAuthState>({ status: 'idle', error: null, sessionId: null })
  const timer = useRef<number | null>(null)

  const stop = useCallback(() => {
    if (timer.current) {
      window.clearInterval(timer.current)
      timer.current = null
    }
  }, [])

  const start = useCallback(
    async (provider: string, clientId = '', clientSecret = '') => {
      stop()
      try {
        const session = await api.oauthAuthorize(provider, clientId, clientSecret)
        setState({ status: 'pending', error: null, sessionId: session.session_id })
        window.open(session.auth_url, '_blank', 'noopener,noreferrer')
        timer.current = window.setInterval(async () => {
          try {
            const view = await api.oauthSession(session.session_id)
            if (view.status === 'done') {
              stop()
              setState({ status: 'done', error: null, sessionId: session.session_id })
              onDone?.()
            } else if (view.status === 'failed') {
              stop()
              setState({
                status: 'failed',
                error: view.error_code
                  ? new ApiError({ code: view.error_code, kind: 'auth', http_status: 401 })
                  : null,
                sessionId: session.session_id,
              })
            }
          } catch {
            // Transient polling failure: keep waiting until the session expires.
          }
        }, 1500)
      } catch (err) {
        setState({ status: 'failed', error: err instanceof ApiError ? err : null, sessionId: null })
      }
    },
    [onDone, stop],
  )

  const reset = useCallback(() => {
    stop()
    setState({ status: 'idle', error: null, sessionId: null })
  }, [stop])

  return { ...state, start, reset }
}
