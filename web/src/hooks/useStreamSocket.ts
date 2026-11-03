import { useCallback, useEffect, useRef, useState } from 'react'
import { INITIAL_SOCKET_STATE, reduceSocketState, type SocketState } from '@/lib/socketReducer'

/**
 * useStreamSocket owns the realtime channel. A dropped connection never affects
 * transfers: it reconnects with exponential backoff (1s -> 15s) and receives a
 * fresh snapshot, after which metrics catch up automatically.
 */
export function useStreamSocket(onQueueChange?: () => void) {
  const [state, setState] = useState<SocketState>(INITIAL_SOCKET_STATE)
  const retry = useRef(0)
  const closed = useRef(false)
  const socket = useRef<WebSocket | null>(null)
  const queueCb = useRef(onQueueChange)
  queueCb.current = onQueueChange

  const connect = useCallback(() => {
    if (closed.current) return
    const token = document.querySelector('meta[name="streamdrop-token"]')?.getAttribute('content') ?? ''
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${window.location.host}/api/v1/ws?token=${encodeURIComponent(token)}`)
    socket.current = ws

    ws.onopen = () => {
      retry.current = 0
      setState((s) => ({ ...s, connected: true }))
    }

    ws.onmessage = (event) => {
      let parsed: { event: string; data: unknown }
      try {
        parsed = JSON.parse(event.data)
      } catch {
        return
      }
      const prev = stateRef.current
      const next = reduceSocketState(prev, parsed)
      stateRef.current = next
      setState(next)
      if (parsed.event === 'queue_update' && next.queue !== prev.queue) {
        queueCb.current?.()
      }
    }

    ws.onclose = () => {
      socket.current = null
      setState((s) => ({ ...s, connected: false }))
      if (closed.current) return
      retry.current = Math.min(retry.current + 1, 5)
      const delay = Math.min(15000, 1000 * 2 ** (retry.current - 1))
      window.setTimeout(connect, delay)
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [])

  const stateRef = useRef(state)
  stateRef.current = state

  useEffect(() => {
    closed.current = false
    connect()
    return () => {
      closed.current = true
      socket.current?.close()
    }
  }, [connect])

  const clearNotify = useCallback(() => setState((s) => ({ ...s, notify: null })), [])

  return { ...state, clearNotify }
}
