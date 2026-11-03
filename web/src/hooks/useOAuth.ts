import { useCallback, useRef, useState } from 'react'
import { api, ApiError } from '@/lib/api'

interface OAuthState {
  status: 'idle' | 'pending' | 'done' | 'failed'
  error: ApiError | null
  sessionId: string | null
  /** authUrl allows a manual fallback when the popup is blocked. */
  authUrl: string | null
  blocked: boolean
}

/**
 * useOAuth starts a provider flow, opens the system browser and polls the
 * one-time session until the callback lands on the fixed loopback port.
 * The popup is opened synchronously with the user gesture so ad-blockers /
 * browser popup blockers cannot swallow it.
 */
export function useOAuth(onDone?: () => void) {
  const [state, setState] = useState<OAuthState>({
    status: 'idle',
    error: null,
    sessionId: null,
    authUrl: null,
    blocked: false,
  })
  const timer = useRef<number | null>(null)
  const winRef = useRef<Window | null>(null)

  const stop = useCallback(() => {
    if (timer.current) {
      window.clearInterval(timer.current)
      timer.current = null
    }
  }, [])

  const start = useCallback(
    async (provider: string, clientId = '', clientSecret = '') => {
      stop()
      // Open synchronously with the click; navigate after the session exists.
      let win: Window | null = null
      try {
        win = window.open('about:blank', '_blank', 'noopener,noreferrer')
      } catch {
        win = null
      }
      winRef.current = win

      try {
        const session = await api.oauthAuthorize(provider, clientId, clientSecret)
        setState({
          status: 'pending',
          error: null,
          sessionId: session.session_id,
          authUrl: session.auth_url,
          blocked: win === null,
        })
        if (win) {
          win.location.href = session.auth_url
        }
        timer.current = window.setInterval(async () => {
          try {
            const view = await api.oauthSession(session.session_id)
            if (view.status === 'done') {
              stop()
              setState({ status: 'done', error: null, sessionId: session.session_id, authUrl: null, blocked: false })
              onDone?.()
            } else if (view.status === 'failed') {
              stop()
              setState({
                status: 'failed',
                error: view.error_code
                  ? new ApiError({ code: view.error_code, kind: 'auth', http_status: 401 })
                  : null,
                sessionId: session.session_id,
                authUrl: session.auth_url,
                blocked: false,
              })
            }
          } catch {
            // Transient polling failure: keep waiting until the session expires.
          }
        }, 1500)
      } catch (err) {
        if (win) {
          try {
            win.close()
          } catch {
            /* ignore */
          }
        }
        winRef.current = null
        setState({ status: 'failed', error: err instanceof ApiError ? err : null, sessionId: null, authUrl: null, blocked: false })
      }
    },
    [onDone, stop],
  )

  const reset = useCallback(() => {
    stop()
    if (winRef.current) {
      try {
        winRef.current.close()
      } catch {
        /* ignore */
      }
    }
    winRef.current = null
    setState({ status: 'idle', error: null, sessionId: null, authUrl: null, blocked: false })
  }, [stop])

  return { ...state, start, reset }
}
