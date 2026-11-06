import { useCallback, useRef, useState } from 'react'
import { api, ApiError } from '@/lib/api'
import { readBrowser } from '@/lib/browserPref'

interface OAuthState {
  status: 'idle' | 'pending' | 'done' | 'failed'
  error: ApiError | null
  sessionId: string | null
  /** authUrl is always available while pending for manual fallback. */
  authUrl: string | null
  /** Whether the last request to open the system browser failed. */
  openFailed: boolean
  /** Whether the browser-open attempt is currently running. */
  opening: boolean
}

/**
 * useOAuth starts a provider flow, opens the selected system browser via the
 * backend and polls the one-time session until the callback lands on the
 * fixed loopback port.
 */
export function useOAuth(onDone?: () => void) {
  const [state, setState] = useState<OAuthState>({
    status: 'idle',
    error: null,
    sessionId: null,
    authUrl: null,
    openFailed: false,
    opening: false,
  })
  const timer = useRef<number | null>(null)

  const stop = useCallback(() => {
    if (timer.current) {
      window.clearInterval(timer.current)
      timer.current = null
    }
  }, [])

  /** openBrowser asks the backend to open the auth URL in a browser. */
  const openBrowser = useCallback(async (sessionAuthUrl = state.authUrl, browser: string = readBrowser()) => {
    if (!sessionAuthUrl) return false
    setState((s) => ({ ...s, opening: true }))
    try {
      await api.openURL(sessionAuthUrl, browser)
      setState((s) => ({ ...s, opening: false, openFailed: false }))
      return true
    } catch {
      setState((s) => ({ ...s, opening: false, openFailed: true }))
      return false
    }
  }, [state.authUrl])

  const start = useCallback(
    async (provider: string, clientId = '', clientSecret = '') => {
      stop()
      try {
        const session = await api.oauthAuthorize(provider, clientId, clientSecret)
        setState({
          status: 'pending',
          error: null,
          sessionId: session.session_id,
          authUrl: session.auth_url,
          openFailed: false,
          opening: false,
        })
        // The backend opens the system browser as part of authorize; no
        // client-side popup is needed. The manual link stays visible as a
        // fallback and the retry button can re-open it.
        timer.current = window.setInterval(async () => {
          try {
            const view = await api.oauthSession(session.session_id)
            if (view.status === 'done') {
              stop()
              setState({ status: 'done', error: null, sessionId: session.session_id, authUrl: null, openFailed: false, opening: false })
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
                openFailed: false,
                opening: false,
              })
            }
          } catch {
            // Transient polling failure: keep waiting until the session expires.
          }
        }, 1500)
      } catch (err) {
        setState({
          status: 'failed',
          error: err instanceof ApiError ? err : null,
          sessionId: null,
          authUrl: null,
          openFailed: false,
          opening: false,
        })
      }
    },
    [onDone, stop],
  )

  /** submitCode completes a provider flow that shows the code on a
   * verification page (Yandex): the user pastes the code here. */
  const submitCode = useCallback(
    async (code: string) => {
      if (!state.sessionId || !code.trim()) return false
      stop()
      try {
        const view = await api.oauthSubmitCode(state.sessionId, code.trim())
        if (view.status === 'done') {
          setState({ status: 'done', error: null, sessionId: state.sessionId, authUrl: null, openFailed: false, opening: false })
          onDone?.()
          return true
        }
        if (view.status === 'failed') {
          setState({
            status: 'failed',
            error: view.error_code ? new ApiError({ code: view.error_code, kind: 'auth', http_status: 401 }) : null,
            sessionId: state.sessionId,
            authUrl: state.authUrl,
            openFailed: false,
            opening: false,
          })
        }
        return false
      } catch (err) {
        setState((s) => ({
          ...s,
          status: 'failed',
          error: err instanceof ApiError ? err : null,
          openFailed: false,
          opening: false,
        }))
        return false
      }
    },
    [state.sessionId, state.authUrl, stop, onDone],
  )

  const reset = useCallback(() => {
    stop()
    setState({ status: 'idle', error: null, sessionId: null, authUrl: null, openFailed: false, opening: false })
  }, [stop])

  return { ...state, start, reset, openBrowser, submitCode }
}